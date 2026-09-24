// Package hub runs the machine outside the filtered network: the database,
// the admin console, the API gates sync with, and the egress that carries
// customers' connections on to the real services.
package hub

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amor7/gozarDNS/internal/auth"
	"github.com/amor7/gozarDNS/internal/catalog"
	"github.com/amor7/gozarDNS/internal/certs"
	"github.com/amor7/gozarDNS/internal/config"
	"github.com/amor7/gozarDNS/internal/netx"
	"github.com/amor7/gozarDNS/internal/resolver"
	"github.com/amor7/gozarDNS/internal/store"
	"github.com/amor7/gozarDNS/internal/sysmetrics"
)

type Hub struct {
	cfg     *config.Config
	h       *config.Hub
	log     *slog.Logger
	store   *store.Store
	cat     *catalog.Catalog
	sampler *sysmetrics.Sampler

	controlCert tls.Certificate
	adminCert   *certs.Server
	adminL      *netx.ChanListener

	resolver *resolver.Resolver
	ipv6     atomic.Bool

	allowMu sync.RWMutex
	allowed map[string]bool // gate addresses the egress answers

	loginThrottle  *auth.Throttle
	signupThrottle *auth.Throttle
	openConns      atomic.Int64
	tunnelStreams  atomic.Int64

	// ctrlL receives gates' tunnel connections, handed over from port 443.
	ctrlL *netx.ChanListener

	restart chan struct{}

	// dialHook replaces dialOut in tests, which cannot reach the internet.
	dialHook func(ctx context.Context, name, port string) (net.Conn, error)
}

func New(cfg *config.Config, st *store.Store, logger *slog.Logger) (*Hub, error) {
	h := cfg.Hub
	tlsDir := filepath.Join(cfg.DataDir, "tls")
	control, err := certs.SelfSigned(tlsDir, "control", []string{h.PublicIP})
	if err != nil {
		return nil, fmt.Errorf("control certificate: %w", err)
	}
	adminFallback, err := certs.SelfSigned(tlsDir, "admin", []string{h.PublicIP, h.AdminDomain})
	if err != nil {
		return nil, fmt.Errorf("admin certificate: %w", err)
	}
	s := &Hub{
		cfg:            cfg,
		h:              h,
		log:            logger,
		store:          st,
		cat:            catalog.Builtin(),
		sampler:        sysmetrics.New(cfg.DataDir),
		controlCert:    control,
		adminCert:      certs.NewServer(h.AdminDomain, h.ACMEEmail, filepath.Join(cfg.DataDir, "acme"), adminFallback),
		resolver:       resolver.New([]string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}),
		allowed:        map[string]bool{},
		loginThrottle:  auth.NewThrottle(8, 15*time.Minute),
		signupThrottle: auth.NewThrottle(5, time.Hour),
		restart:        make(chan struct{}),
	}
	return s, nil
}

// ControlFingerprint is what gates pin.
func (s *Hub) ControlFingerprint() string { return certs.Fingerprint(s.controlCert) }

// ErrRestart asks the caller to exit so the service manager starts a fresh
// process - after a restore, most importantly.
var ErrRestart = errors.New("restart requested")

func (s *Hub) Run(ctx context.Context) error {
	errc := make(chan error, 8)
	fail := func(what string, err error) {
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !netx.IsClosed(err) {
			errc <- fmt.Errorf("%s: %w", what, err)
		}
	}
	s.refreshAllowed()
	s.detectIPv6()

	// Control API: gates only.
	ctrl := &http.Server{
		Handler:           s.controlHandler(),
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{s.controlCert}, MinVersion: tls.VersionTLS12},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          log.New(discard{}, "", 0),
	}
	ctrlL, err := net.Listen("tcp", ":"+strconv.Itoa(s.h.ControlPort))
	if err != nil {
		return fmt.Errorf("control port %d: %w", s.h.ControlPort, err)
	}
	go func() { fail("control", ctrl.ServeTLS(ctrlL, "", "")) }()

	// The same API on port 443, for gates that tunnel. No read or write
	// timeouts here: a tunnel stream lasts as long as a download does.
	ctrlTunnel := &http.Server{
		Handler:           ctrl.Handler,
		TLSConfig:         ctrl.TLSConfig.Clone(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       5 * time.Minute,
		HTTP2:             &http.HTTP2Config{MaxConcurrentStreams: 4096, SendPingTimeout: 30 * time.Second, PingTimeout: 15 * time.Second},
		ErrorLog:          log.New(discard{}, "", 0),
	}
	s.ctrlL = netx.NewChanListener(ctrlL.Addr())
	go func() { fail("control", ctrlTunnel.ServeTLS(s.ctrlL, "", "")) }()

	// Admin console: its own port, and 443 when its domain is asked for.
	adminH := s.adminHandler()
	newAdmin := func() *http.Server {
		return &http.Server{
			Handler:           adminH,
			TLSConfig:         s.adminCert.TLSConfig(),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       5 * time.Minute, // backup uploads
			WriteTimeout:      5 * time.Minute,
			IdleTimeout:       120 * time.Second,
			ErrorLog:          log.New(discard{}, "", 0),
		}
	}
	adminSrv, adminSrv2 := newAdmin(), newAdmin()
	adminL, err := net.Listen("tcp", ":"+strconv.Itoa(s.h.AdminPort))
	if err != nil {
		return fmt.Errorf("admin port %d: %w", s.h.AdminPort, err)
	}
	go func() { fail("admin", adminSrv.ServeTLS(adminL, "", "")) }()

	// Egress.
	httpsL, err := net.Listen("tcp", ":"+strconv.Itoa(s.h.HTTPSPort))
	if err != nil {
		return fmt.Errorf("port %d: %w", s.h.HTTPSPort, err)
	}
	s.adminL = netx.NewChanListener(httpsL.Addr())
	go func() { fail("admin", adminSrv2.ServeTLS(s.adminL, "", "")) }()
	go s.acceptLoop(httpsL, s.handleTLS)

	plain := &http.Server{
		Handler:           s.plainHandler(),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          log.New(discard{}, "", 0),
	}
	httpL, err := net.Listen("tcp", ":"+strconv.Itoa(s.h.HTTPPort))
	if err != nil {
		return fmt.Errorf("port %d: %w", s.h.HTTPPort, err)
	}
	go func() { fail("http", plain.Serve(httpL)) }()
	s.adminCert.Warm()

	ctl, err := s.controlSocket()
	if err != nil {
		s.log.Warn("control socket unavailable", "err", err)
	} else {
		defer ctl.Close()
	}

	go s.maintain(ctx)

	s.log.Info("hub is up", "admin", s.AdminURL(), "control_port", s.h.ControlPort, "ipv6_egress", s.ipv6.Load())

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
	case <-s.restart:
		runErr = ErrRestart
	}
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpsL.Close()
	s.adminL.Close()
	s.ctrlL.Close()
	for _, srv := range []*http.Server{ctrl, ctrlTunnel, adminSrv, adminSrv2, plain} {
		_ = srv.Shutdown(sctx)
	}
	return runErr
}

// AdminURL is where the console can be opened.
func (s *Hub) AdminURL() string {
	if s.h.AdminDomain != "" {
		if s.h.HTTPSPort == 443 {
			return "https://" + s.h.AdminDomain
		}
		return "https://" + net.JoinHostPort(s.h.AdminDomain, strconv.Itoa(s.h.HTTPSPort))
	}
	return "https://" + net.JoinHostPort(s.h.PublicIP, strconv.Itoa(s.h.AdminPort))
}

func (s *Hub) acceptLoop(l net.Listener, handle func(net.Conn)) {
	for {
		c, err := l.Accept()
		if err != nil {
			if netx.IsClosed(err) {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		go handle(c)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// maintain runs the periodic chores.
func (s *Hub) maintain(ctx context.Context) {
	minute := time.NewTicker(time.Minute)
	metrics := time.NewTicker(30 * time.Second)
	defer minute.Stop()
	defer metrics.Stop()
	s.store.Prune()
	n := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-metrics.C:
			m := s.sampler.Take()
			m.Conns = s.openConns.Load()
			s.store.RecordMetrics("hub", m)
			s.refreshAllowed()
		case <-minute.C:
			n++
			if rolled, err := s.store.RollPeriods(); err == nil && rolled > 0 {
				s.log.Info("started a new period", "accounts", rolled)
			}
			if n%60 == 0 {
				s.store.Prune()
				s.store.PruneReceipts()
				s.store.PruneOnline()
				s.detectIPv6()
			}
		}
	}
}

func (s *Hub) refreshAllowed() {
	m := map[string]bool{}
	for _, ip := range s.store.GateIPs() {
		m[ip] = true
	}
	s.allowMu.Lock()
	s.allowed = m
	s.allowMu.Unlock()
}

func (s *Hub) isGate(ip string) bool {
	if ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	s.allowMu.RLock()
	defer s.allowMu.RUnlock()
	return s.allowed[ip]
}

// detectIPv6 decides whether Google's names may leave over IPv6.
func (s *Hub) detectIPv6() {
	switch s.h.IPv6Egress {
	case "off":
		s.ipv6.Store(false)
		return
	case "on":
		s.ipv6.Store(true)
		return
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	c, err := d.Dial("tcp6", "[2001:4860:4860::8888]:443")
	if err == nil {
		c.Close()
	}
	s.ipv6.Store(err == nil)
}
