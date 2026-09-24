package gate

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/web"
)

// portal serves the customer's web app. The pages come from this binary;
// everything to do with accounts is answered by the hub, which holds the
// database. The gate adds the one thing only it knows for sure: the address
// the customer is connecting from.
type portal struct {
	gt    *Gate
	app   http.Handler
	proxy *httputil.ReverseProxy
}

func newPortal(gt *Gate) *portal {
	hub, _ := url.Parse(gt.hubURL(""))
	p := &portal{gt: gt, app: web.App("portal")}
	p.proxy = &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(hub)
			r.Out.URL.Path = proto.PathPortal + strings.TrimPrefix(r.In.URL.Path, "/api/")
			r.Out.URL.RawPath = ""
			r.Out.Host = hub.Host
			for k := range r.Out.Header {
				if strings.HasPrefix(strings.ToLower(k), "x-gozar-") || strings.HasPrefix(strings.ToLower(k), "x-forwarded") {
					r.Out.Header.Del(k)
				}
			}
			r.Out.Header.Set("Authorization", "Bearer "+gt.g.GateID+"."+gt.g.Secret)
			r.Out.Header.Set(proto.HeaderClientIP, clientIP(r.In).String())
			r.Out.Header.Set(proto.HeaderGateHost, r.In.Host)
			r.Out.Header.Set(proto.HeaderGateIP, gt.selfIP.String())
		},
		Transport: gt.hubHTTP.Transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			gt.log.Warn("portal: hub unreachable", "err", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "hub_unreachable"})
		},
		ErrorLog: log.New(discard{}, "", 0),
	}
	return p
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

// clientIP is the connecting address. Headers are never consulted: the
// gate is the edge, and nothing in front of it could vouch for one.
func clientIP(r *http.Request) netip.Addr {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	return ap.Addr().Unmap()
}

func (p *portal) server(tlsCfg *tls.Config) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/probe/{token}", p.probe)
	mux.HandleFunc("/api/", p.api)
	mux.Handle("/", p.app)
	return &http.Server{
		Handler:           mux,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 << 10,
		ErrorLog:          log.New(discard{}, "", 0),
	}
}

func (p *portal) api(w http.ResponseWriter, r *http.Request) {
	web.SecurityHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	p.proxy.ServeHTTP(w, r)
}

// probe answers the setup check: did this browser's device resolve the
// token through this gate, and from the same address it is browsing from?
func (p *portal) probe(w http.ResponseWriter, r *http.Request) {
	web.SecurityHeaders(w)
	w.Header().Set("Cache-Control", "no-store")
	ip, seen := p.gt.probes.Seen(strings.ToLower(r.PathValue("token")))
	me := clientIP(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"seen":    seen,
		"same_ip": seen && ip == me,
		"from":    ipString(ip, seen),
		"you":     me.String(),
	})
}

func ipString(ip interface{ String() string }, ok bool) string {
	if !ok {
		return ""
	}
	return ip.String()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
