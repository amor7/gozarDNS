// Package dnsd is the gate's DNS server: the one place a customer's device is
// told whether to reach a service directly or through the hub.
package dnsd

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/time/rate"

	"github.com/amor7/gozar/internal/resolver"
	"github.com/amor7/gozar/internal/rules"
)

// Decision is what the gate knows about whoever is asking.
type Decision struct {
	Allowed bool
	Set     *rules.Set
	UserID  int64
	Name    string // account name, for the watch stream
}

// ProbeSuffix: a query for <token>.check.gozar.internal proves the asking
// device uses this gate for DNS. The customer page generates the token.
const ProbeSuffix = ".check.gozar.internal."

type Server struct {
	Decide   func(netip.Addr) Decision
	SelfIP   func() netip.Addr
	Upstream *resolver.Resolver // ordinary names
	Clean    *resolver.Resolver // names excepted from routing
	Log      *slog.Logger
	Watch    *Bus
	Probes   *Probes
	pins     pinner

	Queries atomic.Int64
	Routed  atomic.Int64
	Refused atomic.Int64

	limMu sync.Mutex
	lims  map[netip.Addr]*limEntry
}

type limEntry struct {
	l    *rate.Limiter
	seen time.Time
}

// ListenAndServe runs UDP and TCP listeners on addr until ctx ends.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	s.lims = map[netip.Addr]*limEntry{}
	s.pins.init()
	udp := &dns.Server{Addr: addr, Net: "udp", Handler: s, UDPSize: 1232, ReusePort: false}
	tcp := &dns.Server{Addr: addr, Net: "tcp", Handler: s, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	errc := make(chan error, 2)
	go func() { errc <- udp.ListenAndServe() }()
	go func() { errc <- tcp.ListenAndServe() }()
	go s.janitor(ctx)
	select {
	case <-ctx.Done():
		_ = udp.Shutdown()
		_ = tcp.Shutdown()
		return nil
	case err := <-errc:
		_ = udp.Shutdown()
		_ = tcp.Shutdown()
		return err
	}
}

func (s *Server) janitor(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cut := time.Now().Add(-5 * time.Minute)
			s.limMu.Lock()
			for k, v := range s.lims {
				if v.seen.Before(cut) {
					delete(s.lims, k)
				}
			}
			s.limMu.Unlock()
			s.Probes.prune()
		}
	}
}

// allow is the amplification defence: an open resolver is worth roughly its
// bandwidth to whoever finds it. Generous enough for a console booting.
func (s *Server) allow(ip netip.Addr) bool {
	s.limMu.Lock()
	defer s.limMu.Unlock()
	e, ok := s.lims[ip]
	if !ok {
		if len(s.lims) > 200000 {
			return false
		}
		e = &limEntry{l: rate.NewLimiter(60, 240)}
		s.lims[ip] = e
	}
	e.seen = time.Now()
	return e.l.Allow()
}

func (s *Server) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	src := addrOf(w.RemoteAddr())
	_, isUDP := w.RemoteAddr().(*net.UDPAddr)
	if isUDP && !s.allow(src) {
		return // silence, not an answer: answering is what gets abused
	}
	s.Queries.Add(1)
	if len(req.Question) != 1 || req.Opcode != dns.OpcodeQuery {
		s.reply(w, req, rcode(req, dns.RcodeNotImplemented), isUDP)
		return
	}
	q := req.Question[0]
	name := strings.ToLower(q.Name)

	if q.Qtype == dns.TypeANY {
		s.reply(w, req, rcode(req, dns.RcodeNotImplemented), isUDP)
		return
	}

	if strings.HasSuffix(name, ProbeSuffix) {
		token := strings.TrimSuffix(name, ProbeSuffix)
		s.Probes.record(token, src)
		m := new(dns.Msg)
		m.SetReply(req)
		m.Authoritative = true
		if q.Qtype == dns.TypeA {
			m.Answer = append(m.Answer, s.selfA(q.Name))
		}
		s.reply(w, req, m, isUDP)
		return
	}

	d := s.Decide(src)
	if !d.Allowed {
		s.Refused.Add(1)
		s.Watch.publish(Event{IP: src, Name: name, Qtype: q.Qtype, Verdict: VerdictRefused})
		s.reply(w, req, rcode(req, dns.RcodeRefused), isUDP)
		return
	}

	action, _ := d.Set.Match(name)
	ev := Event{IP: src, User: d.Name, Name: name, Qtype: q.Qtype}
	switch action {
	case rules.Route:
		s.Routed.Add(1)
		m := new(dns.Msg)
		m.SetReply(req)
		m.RecursionAvailable = true
		if q.Qtype == dns.TypeA {
			m.Answer = append(m.Answer, s.selfA(q.Name))
		}
		// AAAA, HTTPS and SVCB get an empty answer. Any IPv6 address or
		// address hint handed out for a routed name is a way around the hub.
		ev.Verdict = VerdictRouted
		ev.Answer = s.SelfIP().String()
		s.Watch.publish(ev)
		s.reply(w, req, m, isUDP)
		return
	case rules.Direct:
		ev.Verdict = VerdictDirect
		s.forward(w, req, s.Clean, ev, isUDP, false)
	case rules.Pin:
		ev.Verdict = VerdictPinned
		s.forward(w, req, s.Clean, ev, isUDP, true)
	default:
		ev.Verdict = VerdictDirect
		s.forward(w, req, s.Upstream, ev, isUDP, false)
	}
}

func (s *Server) selfA(name string) dns.RR {
	return &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   s.SelfIP().AsSlice(),
	}
}

func (s *Server) forward(w dns.ResponseWriter, req *dns.Msg, r *resolver.Resolver, ev Event, isUDP, pin bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	m, err := r.Exchange(ctx, req)
	if err != nil {
		ev.Verdict = VerdictFailed
		s.Watch.publish(ev)
		s.reply(w, req, rcode(req, dns.RcodeServerFailure), isUDP)
		return
	}
	if pin && req.Question[0].Qtype == dns.TypeA {
		m.Answer = s.pins.filter(ctx, m.Answer)
	}
	ev.Answer = firstAddr(m)
	if filteredLocally(m) {
		ev.Verdict = VerdictFiltered
	} else if m.Rcode == dns.RcodeNameError {
		ev.Verdict = VerdictNXDomain
	}
	s.Watch.publish(ev)
	s.reply(w, req, m, isUDP)
}

func (s *Server) reply(w dns.ResponseWriter, req, m *dns.Msg, isUDP bool) {
	m.Id = req.Id
	if isUDP {
		size := dns.MinMsgSize
		if o := req.IsEdns0(); o != nil {
			size = int(max(min(o.UDPSize(), 1232), dns.MinMsgSize))
		}
		m.Truncate(size)
	}
	_ = w.WriteMsg(m)
}

func rcode(req *dns.Msg, code int) *dns.Msg {
	m := new(dns.Msg)
	m.SetRcode(req, code)
	m.RecursionAvailable = true
	return m
}

func addrOf(a net.Addr) netip.Addr {
	switch v := a.(type) {
	case *net.UDPAddr:
		return v.AddrPort().Addr().Unmap()
	case *net.TCPAddr:
		return v.AddrPort().Addr().Unmap()
	}
	return netip.Addr{}
}

func firstAddr(m *dns.Msg) string {
	for _, rr := range m.Answer {
		switch v := rr.(type) {
		case *dns.A:
			return v.A.String()
		case *dns.AAAA:
			return v.AAAA.String()
		}
	}
	return ""
}

// The national filter answers blocked names with an address in 10.10.34.0/24.
var nationalFilter = netip.MustParsePrefix("10.10.34.0/24")

func filteredLocally(m *dns.Msg) bool {
	for _, rr := range m.Answer {
		if a, ok := rr.(*dns.A); ok {
			if ip, ok := netip.AddrFromSlice(a.A.To4()); ok && nationalFilter.Contains(ip) {
				return true
			}
		}
	}
	return false
}
