// Package gate runs the machine customers point their DNS at: the resolver
// that decides what goes through the hub, the proxy that carries it there,
// and the customer portal.
package gate

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/amor7/gozarDNS/internal/certs"
	"github.com/amor7/gozarDNS/internal/config"
	"github.com/amor7/gozarDNS/internal/dnsd"
	"github.com/amor7/gozarDNS/internal/netx"
	"github.com/amor7/gozarDNS/internal/resolver"
	"github.com/amor7/gozarDNS/internal/stun"
	"github.com/amor7/gozarDNS/internal/sysmetrics"
)

type Gate struct {
	cfg     *config.Config
	g       *config.Gate
	log     *slog.Logger
	state   *state
	dns     *dnsd.Server
	watch   *dnsd.Bus
	probes  *dnsd.Probes
	sampler *sysmetrics.Sampler
	selfIP  netip.Addr
	hubTLS  *tls.Config
	hubHTTP *http.Client

	portal     *portal
	portalCert *certs.Server
	portalL    *netx.ChanListener

	openConns atomic.Int64
	lastSync  atomic.Int64 // unix time of the last successful sync
	syncErr   atomic.Value // string
	hubAddr   string
	kick      chan struct{}
}

func New(cfg *config.Config, log *slog.Logger) (*Gate, error) {
	g := cfg.Gate
	self, err := netip.ParseAddr(g.PublicIP)
	if err != nil || !self.Is4() {
		return nil, fmt.Errorf("gate public_ip %q must be an IPv4 address", g.PublicIP)
	}
	upstreams := g.Upstreams
	if len(upstreams) == 0 {
		upstreams = systemResolvers()
	}
	gt := &Gate{
		cfg:     cfg,
		g:       g,
		log:     log,
		state:   newState(),
		watch:   dnsd.NewBus(),
		probes:  dnsd.NewProbes(),
		sampler: sysmetrics.New(cfg.DataDir),
		selfIP:  self,
		hubTLS:  certs.PinnedConfig(g.Fingerprint),
		hubAddr: g.HubAddr,
		kick:    make(chan struct{}, 1),
	}
	gt.hubHTTP = &http.Client{
		Timeout: 25 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:     gt.hubTLS,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     90 * time.Second,
			ForceAttemptHTTP2:   true,
		},
	}
	gt.dns = &dnsd.Server{
		Decide:   gt.state.decide,
		SelfIP:   func() netip.Addr { return gt.selfIP },
		Upstream: resolver.New(upstreams),
		Clean:    resolver.New(g.CleanUpstreams),
		Log:      log,
		Watch:    gt.watch,
		Probes:   gt.probes,
	}
	if c, err := loadCache(gt.cachePath()); err == nil {
		gt.state.apply(c.Revision, c.Config)
		log.Info("serving the last known config until the hub answers", "revision", c.Revision, "saved", c.Saved.Format(time.RFC3339))
	}
	fallback, err := certs.SelfSigned(filepath.Join(cfg.DataDir, "tls"), "portal", []string{g.PublicIP, g.PortalDomain})
	if err != nil {
		return nil, err
	}
	gt.portalCert = certs.NewServer(g.PortalDomain, g.ACMEEmail, filepath.Join(cfg.DataDir, "acme"), fallback)
	gt.portal = newPortal(gt)
	return gt, nil
}

func (gt *Gate) cachePath() string { return filepath.Join(gt.cfg.DataDir, "gate-config.json") }

func (gt *Gate) hubURL(path string) string {
	return "https://" + net.JoinHostPort(gt.hubAddr, strconv.Itoa(gt.g.HubControlPort)) + path
}

// Run serves until ctx ends. A listener that cannot start is fatal: a gate
// half-running is harder to diagnose than one that refuses to start.
func (gt *Gate) Run(ctx context.Context) error {
	errc := make(chan error, 8)
	fail := func(what string, err error) {
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !netx.IsClosed(err) {
			errc <- fmt.Errorf("%s: %w", what, err)
		}
	}

	go func() { fail("dns "+gt.g.DNSListen, gt.dns.ListenAndServe(ctx, gt.g.DNSListen)) }()

	httpsL, err := net.Listen("tcp", gt.g.HTTPSListen)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", gt.g.HTTPSListen, err)
	}
	httpL, err := net.Listen("tcp", gt.g.HTTPListen)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", gt.g.HTTPListen, err)
	}
	go gt.acceptLoop(httpsL, gt.handleTLS)
	go gt.acceptLoop(httpL, gt.handleHTTP)

	// The portal: through 443 when its domain is asked for, and always on
	// its own port for anyone who reaches it by address.
	gt.portalL = netx.NewChanListener(httpsL.Addr())
	srv := gt.portal.server(gt.portalCert.TLSConfig())
	go func() { fail("portal", srv.ServeTLS(gt.portalL, "", "")) }()
	portalL, err := net.Listen("tcp", ":"+strconv.Itoa(gt.g.PortalPort))
	if err != nil {
		return fmt.Errorf("listening on portal port %d: %w", gt.g.PortalPort, err)
	}
	srv2 := gt.portal.server(gt.portalCert.TLSConfig())
	go func() { fail("portal", srv2.ServeTLS(portalL, "", "")) }()
	gt.portalCert.Warm()

	if gt.g.STUNListen != "" {
		pc, err := net.ListenPacket("udp", gt.g.STUNListen)
		if err != nil {
			gt.log.Warn("STUN is off: cannot listen", "addr", gt.g.STUNListen, "err", err)
		} else {
			go stun.Serve(pc, gt.log)
			defer pc.Close()
		}
	}

	ctl, err := gt.controlSocket()
	if err != nil {
		gt.log.Warn("control socket unavailable - `gozar status` and `gozar watch` will not work", "err", err)
	} else {
		defer ctl.Close()
	}

	go gt.syncLoop(ctx)

	gt.log.Info("gate is up", "ip", gt.selfIP, "dns", gt.g.DNSListen, "hub", gt.hubAddr, "portal", gt.portalAddress())

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errc:
	}
	httpsL.Close()
	httpL.Close()
	portalL.Close()
	gt.portalL.Close()
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	_ = srv2.Shutdown(sctx)
	return runErr
}

func (gt *Gate) portalAddress() string {
	if gt.g.PortalDomain != "" {
		return "https://" + gt.g.PortalDomain
	}
	return "https://" + net.JoinHostPort(gt.selfIP.String(), strconv.Itoa(gt.g.PortalPort))
}

func (gt *Gate) acceptLoop(l net.Listener, handle func(net.Conn)) {
	for {
		c, err := l.Accept()
		if err != nil {
			if netx.IsClosed(err) {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			gt.log.Warn("accept failed", "err", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go handle(c)
	}
}
