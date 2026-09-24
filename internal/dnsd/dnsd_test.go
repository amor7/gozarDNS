package dnsd

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/amor7/gozarDNS/internal/resolver"
	"github.com/amor7/gozarDNS/internal/rules"
)

// fakeUpstream answers every A query with 93.184.216.34 and every AAAA with
// 2606:2800::1.
func fakeUpstream(t *testing.T) string {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{PacketConn: pc, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		q := r.Question[0]
		switch q.Qtype {
		case dns.TypeA:
			m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300}, A: net.ParseIP("93.184.216.34")})
		case dns.TypeAAAA:
			m.Answer = append(m.Answer, &dns.AAAA{Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 300}, AAAA: net.ParseIP("2606:2800::1")})
		}
		_ = w.WriteMsg(m)
	})}
	go srv.ActivateAndServe()
	t.Cleanup(func() { srv.Shutdown() })
	return pc.LocalAddr().String()
}

func startServer(t *testing.T, decide func(netip.Addr) Decision) string {
	up := fakeUpstream(t)
	s := &Server{
		Decide:   decide,
		SelfIP:   func() netip.Addr { return netip.MustParseAddr("203.0.113.10") },
		Upstream: resolver.New([]string{up}),
		Clean:    resolver.New([]string{up}),
		Watch:    NewBus(),
		Probes:   NewProbes(),
	}
	l, _ := net.ListenPacket("udp", "127.0.0.1:0")
	addr := l.LocalAddr().String()
	l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.ListenAndServe(ctx, addr)
	time.Sleep(150 * time.Millisecond)
	return addr
}

func ask(t *testing.T, addr, name string, qt uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qt)
	r, err := dns.Exchange(m, addr)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRoutingDecisions(t *testing.T) {
	set := rules.Compile(rules.Spec{Route: []string{"playstation.net"}, Direct: []string{"np.playstation.net"}})
	addr := startServer(t, func(netip.Addr) Decision { return Decision{Allowed: true, Set: set} })

	r := ask(t, addr, "gst.prod.dl.playstation.net", dns.TypeA)
	if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "203.0.113.10" {
		t.Fatalf("routed name: %v", r.Answer)
	}
	if r := ask(t, addr, "gst.prod.dl.playstation.net", dns.TypeAAAA); len(r.Answer) != 0 || r.Rcode != dns.RcodeSuccess {
		t.Fatalf("routed AAAA must be empty: %v", r)
	}
	if r := ask(t, addr, "ps5.np.playstation.net", dns.TypeA); r.Answer[0].(*dns.A).A.String() != "93.184.216.34" {
		t.Fatalf("excepted name: %v", r.Answer)
	}
	if r := ask(t, addr, "example.org", dns.TypeAAAA); len(r.Answer) != 1 {
		t.Fatalf("ordinary AAAA should pass through: %v", r.Answer)
	}
}

func TestRefusesUnregistered(t *testing.T) {
	addr := startServer(t, func(netip.Addr) Decision { return Decision{Allowed: false} })
	if r := ask(t, addr, "example.org", dns.TypeA); r.Rcode != dns.RcodeRefused {
		t.Fatalf("rcode %d", r.Rcode)
	}
}

func TestProbeIsAnsweredEvenWhenRefused(t *testing.T) {
	addr := startServer(t, func(netip.Addr) Decision { return Decision{Allowed: false} })
	r := ask(t, addr, "tok12345678.check.gozar.internal", dns.TypeA)
	if len(r.Answer) != 1 {
		t.Fatalf("probe not answered: %v", r)
	}
}
