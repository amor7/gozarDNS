package gate

import (
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"time"

	"github.com/amor7/gozar/internal/buildinfo"
	"github.com/amor7/gozar/internal/ctl"
	"github.com/amor7/gozar/internal/rules"
)

// Status is what `gozar status` prints on a gate.
type Status struct {
	Role      string   `json:"role"`
	Version   string   `json:"version"`
	PublicIP  string   `json:"public_ip"`
	Hub       string   `json:"hub"`
	Portal    string   `json:"portal"`
	Revision  string   `json:"revision"`
	Enforce   bool     `json:"enforce"`
	Clients   int      `json:"clients"`
	Plans     int      `json:"plans"`
	OpenConns int64    `json:"open_conns"`
	LastSync  int64    `json:"last_sync"`
	SyncError string   `json:"sync_error,omitempty"`
	Upstreams []string `json:"upstreams"`
}

type CheckResult struct {
	Name    string `json:"name"`
	User    string `json:"user,omitempty"`
	Action  string `json:"action"`
	Matched string `json:"matched,omitempty"`
	Allowed bool   `json:"allowed"`
}

// Snapshot reports the gate's current state.
func (gt *Gate) Snapshot() Status {
	rev, enforce, clients, plans := gt.state.info()
	errStr, _ := gt.syncErr.Load().(string)
	return Status{
		Role: "gate", Version: buildinfo.Version, PublicIP: gt.selfIP.String(),
		Hub: gt.hubAddr, Portal: gt.portalAddress(), Revision: rev, Enforce: enforce,
		Clients: clients, Plans: plans, OpenConns: gt.openConns.Load(),
		LastSync: gt.lastSync.Load(), SyncError: errStr, Upstreams: gt.dns.Upstream.Servers(),
	}
}

func (gt *Gate) controlSocket() (io.Closer, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, gt.Snapshot())
	})
	// /check?name=example.com&user=ali (or &ip=1.2.3.4)
	mux.HandleFunc("GET /check", func(w http.ResponseWriter, r *http.Request) {
		name := rules.Normalize(r.URL.Query().Get("name"))
		ip, _ := netip.ParseAddr(r.URL.Query().Get("ip"))
		if u := r.URL.Query().Get("user"); u != "" {
			if _, ips := gt.state.userByName(u); len(ips) > 0 {
				ip = ips[0]
			} else {
				http.Error(w, "no registered address for "+u+" on this gate", 404)
				return
			}
		}
		c, set, ok := gt.state.who(ip)
		res := CheckResult{Name: name, Allowed: ok}
		if c != nil {
			res.User = c.name
		}
		a, m := set.Match(name)
		res.Action, res.Matched = a.String(), m
		writeJSON(w, 200, res)
	})
	// /watch?user=ali or ?ip=1.2.3.4 streams one JSON object per line.
	mux.HandleFunc("GET /watch", func(w http.ResponseWriter, r *http.Request) {
		var want map[netip.Addr]bool
		if u := r.URL.Query().Get("user"); u != "" {
			_, ips := gt.state.userByName(u)
			if len(ips) == 0 {
				http.Error(w, "no registered address for "+u+" on this gate", 404)
				return
			}
			want = map[netip.Addr]bool{}
			for _, ip := range ips {
				want[ip] = true
			}
		}
		if s := r.URL.Query().Get("ip"); s != "" {
			ip, err := netip.ParseAddr(s)
			if err != nil {
				http.Error(w, "bad ip", 400)
				return
			}
			want = map[netip.Addr]bool{ip: true}
		}
		ch, stop := gt.watch.Subscribe()
		defer stop()
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(200)
		fl, _ := w.(http.Flusher)
		enc := json.NewEncoder(w)
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
				_, _ = w.Write([]byte("\n")) // keepalive
			case e := <-ch:
				if want != nil && !want[e.IP] {
					continue
				}
				if enc.Encode(e) != nil {
					return
				}
			}
			if fl != nil {
				fl.Flush()
			}
		}
	})
	return ctl.Listen(mux)
}
