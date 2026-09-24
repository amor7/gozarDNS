package hub

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"github.com/amor7/gozarDNS/internal/buildinfo"
	"github.com/amor7/gozarDNS/internal/ctl"
	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/store"
)

type gateKey struct{}

// gateAuth admits requests carrying a gate's credential.
func (s *Hub) gateAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cred := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		g, ok := s.store.AuthGate(cred)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, &apiError{Code: "unauthorized"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), gateKey{}, g)))
	})
}

func gateOf(r *http.Request) *store.Gate {
	g, _ := r.Context().Value(gateKey{}).(*store.Gate)
	return g
}

func (s *Hub) controlHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+proto.PathSync, s.wrap(s.sync))
	mux.HandleFunc("GET "+proto.PathHello, s.wrap(func(w http.ResponseWriter, r *http.Request) error {
		g := gateOf(r)
		return ok(w, map[string]string{"gate": g.ID, "name": g.Name, "hub_version": buildinfo.Version})
	}))
	mux.Handle(proto.PathPortal, http.StripPrefix(strings.TrimSuffix(proto.PathPortal, "/"), s.portalHandler()))
	return s.gateAuth(mux)
}

func (s *Hub) sync(w http.ResponseWriter, r *http.Request) error {
	g := gateOf(r)
	var req proto.SyncRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<20)).Decode(&req); err != nil {
		return errBadRequest
	}
	if a, err := netip.ParseAddr(req.PublicIP); err != nil || !a.Is4() {
		req.PublicIP = ""
	}
	seen := remoteIP(r)
	if _, err := s.store.RecordSync(g, seen, &req); err != nil {
		return err
	}
	req.Host.Conns = req.Stats.OpenConns
	s.store.RecordMetrics(g.ID, req.Host)
	if !s.isGate(seen) {
		s.refreshAllowed()
	}
	if _, err := s.store.RollPeriods(); err != nil {
		s.log.Warn("rolling periods failed", "err", err)
	}
	cfg, rev, err := s.gateConfig(g)
	if err != nil {
		return err
	}
	out := proto.SyncResponse{Revision: rev, Interval: 30}
	if rev != req.Have {
		out.Config = cfg
	}
	return ok(w, out)
}

// ---------------------------------------------------------------- local socket

type HubStatus struct {
	Role      string        `json:"role"`
	Version   string        `json:"version"`
	AdminURL  string        `json:"admin_url"`
	PublicIP  string        `json:"public_ip"`
	IPv6      bool          `json:"ipv6_egress"`
	OpenConns int64         `json:"open_conns"`
	Gates     []*store.Gate `json:"gates"`
	Totals    store.Totals  `json:"totals"`
}

func (s *Hub) controlSocket() (io.Closer, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		gates, _ := s.store.Gates()
		writeJSON(w, 200, HubStatus{
			Role: "hub", Version: buildinfo.Version, AdminURL: s.AdminURL(), PublicIP: s.h.PublicIP,
			IPv6: s.ipv6.Load(), OpenConns: s.openConns.Load(), Gates: gates, Totals: s.store.Totals(),
		})
	})
	return ctl.Listen(mux)
}

// JoinToken builds the token a new gate is set up with.
func JoinToken(h *HubInfo, gateID, secret string) string {
	return proto.JoinToken{
		Hub: h.PublicIP, ControlPort: h.ControlPort, HTTPSPort: h.HTTPSPort, HTTPPort: h.HTTPPort,
		Fingerprint: h.Fingerprint, SNI: h.SNI, GateID: gateID, Secret: secret,
	}.Encode()
}

// HubInfo is the part of the hub's identity that goes into a join token.
type HubInfo struct {
	PublicIP    string
	ControlPort int
	HTTPSPort   int
	HTTPPort    int
	Fingerprint string
	SNI         string // the hub's admin domain, when it has one
}

func (s *Hub) info() *HubInfo {
	return &HubInfo{PublicIP: s.h.PublicIP, ControlPort: s.h.ControlPort, HTTPSPort: s.h.HTTPSPort,
		HTTPPort: s.h.HTTPPort, Fingerprint: s.ControlFingerprint(), SNI: s.h.AdminDomain}
}
