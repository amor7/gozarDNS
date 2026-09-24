// Package resolver asks upstream DNS servers and remembers the answers for as
// long as they say they are good for.
package resolver

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type Resolver struct {
	servers []string
	udp     *dns.Client
	tcp     *dns.Client

	mu    sync.Mutex
	cache map[key]entry
	max   int
}

type key struct {
	name  string
	qtype uint16
}

type entry struct {
	msg     *dns.Msg
	expires time.Time
}

// New builds a resolver over the given servers ("1.1.1.1" or "1.1.1.1:53").
func New(servers []string) *Resolver {
	r := &Resolver{
		udp:   &dns.Client{Net: "udp", Timeout: 2 * time.Second, UDPSize: 1232},
		tcp:   &dns.Client{Net: "tcp", Timeout: 3 * time.Second},
		cache: map[key]entry{},
		max:   20000,
	}
	for _, s := range servers {
		if _, _, err := net.SplitHostPort(s); err != nil {
			s = net.JoinHostPort(s, "53")
		}
		r.servers = append(r.servers, s)
	}
	return r
}

func (r *Resolver) Servers() []string { return r.servers }

var ErrNoServers = errors.New("no upstream DNS servers")

// Exchange forwards q upstream, trying each server in turn, and serves from
// cache when it can. The returned message is a copy the caller may modify.
func (r *Resolver) Exchange(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	if len(q.Question) != 1 {
		return nil, errors.New("exactly one question expected")
	}
	k := key{strings.ToLower(q.Question[0].Name), q.Question[0].Qtype}
	if m := r.cached(k); m != nil {
		m.Id = q.Id
		return m, nil
	}
	if len(r.servers) == 0 {
		return nil, ErrNoServers
	}
	out := q.Copy()
	out.RecursionDesired = true
	out.SetEdns0(1232, false)
	var lastErr error
	for _, s := range r.servers {
		m, _, err := r.udp.ExchangeContext(ctx, out, s)
		if err == nil && m.Truncated {
			m, _, err = r.tcp.ExchangeContext(ctx, out, s)
		}
		if err != nil {
			lastErr = err
			continue
		}
		if m.Rcode == dns.RcodeServerFailure {
			lastErr = errors.New("SERVFAIL from " + s)
			continue
		}
		r.store(k, m)
		res := m.Copy()
		res.Id = q.Id
		return res, nil
	}
	return nil, lastErr
}

func (r *Resolver) cached(k key) *dns.Msg {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[k]
	if !ok {
		return nil
	}
	left := time.Until(e.expires)
	if left <= 0 {
		delete(r.cache, k)
		return nil
	}
	m := e.msg.Copy()
	ttl := uint32(left.Seconds()) + 1
	for _, sec := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for _, rr := range sec {
			if rr.Header().Rrtype != dns.TypeOPT && rr.Header().Ttl > ttl {
				rr.Header().Ttl = ttl
			}
		}
	}
	return m
}

func (r *Resolver) store(k key, m *dns.Msg) {
	ttl := minTTL(m)
	if ttl == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.cache) >= r.max {
		now := time.Now()
		for kk, e := range r.cache {
			if e.expires.Before(now) || len(r.cache) >= r.max {
				delete(r.cache, kk)
			}
		}
	}
	r.cache[k] = entry{msg: m.Copy(), expires: time.Now().Add(time.Duration(ttl) * time.Second)}
}

func minTTL(m *dns.Msg) uint32 {
	var ttl uint32 = 3600
	found := false
	for _, sec := range [][]dns.RR{m.Answer, m.Ns} {
		for _, rr := range sec {
			found = true
			ttl = min(ttl, rr.Header().Ttl)
		}
	}
	if !found {
		if m.Rcode == dns.RcodeNameError || m.Rcode == dns.RcodeSuccess {
			return 30 // negative answers, briefly
		}
		return 0
	}
	return max(min(ttl, 3600), 5)
}

// LookupIP returns the addresses for name of one family ("ip4" or "ip6").
func (r *Resolver) LookupIP(ctx context.Context, name, family string) ([]netip.Addr, error) {
	qt := dns.TypeA
	if family == "ip6" {
		qt = dns.TypeAAAA
	}
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(name), qt)
	m, err := r.Exchange(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, rr := range m.Answer {
		switch v := rr.(type) {
		case *dns.A:
			if a, ok := netip.AddrFromSlice(v.A.To4()); ok {
				out = append(out, a)
			}
		case *dns.AAAA:
			if a, ok := netip.AddrFromSlice(v.AAAA); ok {
				out = append(out, a)
			}
		}
	}
	if len(out) == 0 {
		return nil, &net.DNSError{Err: "no addresses", Name: name, IsNotFound: true}
	}
	return out, nil
}
