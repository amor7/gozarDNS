package hub

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"strings"
	"time"

	"github.com/amor7/gozar/internal/netx"
	"github.com/amor7/gozar/internal/rules"
)

// googleSuffixes leave over IPv6 when the hub has it. Google refuses Gemini,
// AI Studio and friends to some IPv4 ranges it places in sanctioned
// countries, and serves the same machine fine over IPv6.
var googleSuffixes = []string{
	"google.com", "googleapis.com", "gstatic.com", "googleusercontent.com", "google",
	"withgoogle.com", "googlevideo.com", "ggpht.com", "gvt1.com",
}

// consoleSuffixes are served over plain HTTP by their CDNs. Their port 443
// presents a certificate that names no console host at all, so the only way
// to carry a console download is to forward the HTTP itself.
var consoleSuffixes = []string{"playstation.net", "playstation.com", "xboxlive.com", "gamepass.com"}

func hasSuffix(name string, list []string) bool {
	for _, s := range list {
		if name == s || strings.HasSuffix(name, "."+s) {
			return true
		}
	}
	return false
}

// handleTLS: the admin console when its domain is asked for; otherwise a
// gate's customer connection, carried to the name in the ClientHello.
func (s *Hub) handleTLS(c net.Conn) {
	name, pc, err := netx.PeekSNI(c, 10*time.Second)
	if err != nil {
		c.Close()
		return
	}
	if s.h.AdminDomain != "" && name == s.h.AdminDomain {
		s.adminL.Push(pc)
		return
	}
	if !s.isGate(netx.RemoteAddr(c).String()) {
		c.Close() // not an open proxy
		return
	}
	up, err := s.dialOut(context.Background(), name, "443")
	if err != nil {
		c.Close()
		return
	}
	s.openConns.Add(1)
	defer s.openConns.Add(-1)
	netx.Pipe(pc, up, netx.Counter{}, nil, 10*time.Minute)
}

var errRefusedTarget = errors.New("refusing to dial that destination")

// dialOut connects to name:port. The name must be a real hostname and must
// resolve to public addresses: an empty or bare-IP SNI, or one resolving to
// this machine, would otherwise loop straight back through the gate.
func (s *Hub) dialOut(ctx context.Context, name, port string) (net.Conn, error) {
	if !rules.ValidDomain(name) {
		return nil, errRefusedTarget
	}
	if s.dialHook != nil {
		return s.dialHook(ctx, name, port)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var addrs []netip.Addr
	if s.ipv6.Load() && hasSuffix(name, googleSuffixes) {
		addrs, _ = s.resolver.LookupIP(ctx, name, "ip6")
	}
	if v4, err := s.resolver.LookupIP(ctx, name, "ip4"); err == nil {
		addrs = append(addrs, v4...)
	}
	d := net.Dialer{Timeout: 6 * time.Second, KeepAlive: 30 * time.Second}
	tried := 0
	var lastErr error = errRefusedTarget
	for _, a := range addrs {
		if !netx.IsPublic(a) || a.String() == s.h.PublicIP {
			continue
		}
		if tried++; tried > 3 {
			break
		}
		c, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(a, portNum(port)).String())
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func portNum(p string) uint16 {
	if p == "80" {
		return 80
	}
	return 443
}

// plainHandler serves the hub's port 80: console downloads forwarded as HTTP,
// everything else redirected to HTTPS.
func (s *Hub) plainHandler() http.Handler {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			host, port, _ := net.SplitHostPort(addr)
			return s.dialOut(ctx, host, port)
		},
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    true,
	}
	toHTTPS := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+hostOnly(r.Host)+r.URL.RequestURI(), http.StatusMovedPermanently)
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = hostOnly(pr.In.Host)
			pr.Out.Host = pr.In.Host
		},
		Transport:     transport,
		FlushInterval: -1, // game packages stream; never buffer them here
		// Not every host under these names actually speaks HTTP. Where the
		// upstream cannot, fall back to the redirect: this path can only add
		// to what works, never take anything away.
		ModifyResponse: func(resp *http.Response) error {
			if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout {
				return errors.New("upstream gateway error")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) { toHTTPS(w, r) },
		ErrorLog:     nil,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := hostOnly(r.Host)
		if s.h.AdminDomain != "" && host == s.h.AdminDomain {
			toHTTPS(w, r)
			return
		}
		if !s.isGate(remoteIP(r)) {
			http.Error(w, "", http.StatusForbidden)
			return
		}
		if hasSuffix(host, consoleSuffixes) {
			proxy.ServeHTTP(w, r)
			return
		}
		toHTTPS(w, r)
	})
}

func hostOnly(h string) string {
	h = strings.ToLower(h)
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}
