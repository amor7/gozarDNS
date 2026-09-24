package dnsd

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// ---------------------------------------------------------------- watch

const (
	VerdictRouted   = "routed"
	VerdictDirect   = "direct"
	VerdictPinned   = "pinned"
	VerdictRefused  = "refused"
	VerdictFiltered = "filtered"
	VerdictNXDomain = "nxdomain"
	VerdictFailed   = "failed"
)

type Event struct {
	At      time.Time  `json:"at"`
	IP      netip.Addr `json:"ip"`
	User    string     `json:"user,omitempty"`
	Name    string     `json:"name"`
	Qtype   uint16     `json:"qtype"`
	Type    string     `json:"type"`
	Verdict string     `json:"verdict"`
	Answer  string     `json:"answer,omitempty"`
}

// Bus fans query events out to whoever is watching. Nothing is written to
// disk: what a watcher sees while connected is all there ever is.
type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

func NewBus() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 256)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

func (b *Bus) publish(e Event) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.subs) == 0 {
		return
	}
	e.At = time.Now()
	e.Type = dns.TypeToString[e.Qtype]
	for ch := range b.subs {
		select {
		case ch <- e:
		default: // a slow watcher misses lines rather than slowing DNS
		}
	}
}

// ---------------------------------------------------------------- probes

// Probes remembers which address asked for which check token, briefly.
type Probes struct {
	mu   sync.Mutex
	seen map[string]probe
}

type probe struct {
	ip netip.Addr
	at time.Time
}

func NewProbes() *Probes { return &Probes{seen: map[string]probe{}} }

func (p *Probes) record(token string, ip netip.Addr) {
	if len(token) < 8 || len(token) > 64 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.seen) > 50000 {
		return
	}
	p.seen[token] = probe{ip, time.Now()}
}

// Seen reports the address that asked for token, if anyone did.
func (p *Probes) Seen(token string) (netip.Addr, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.seen[token]
	return v.ip, ok
}

func (p *Probes) prune() {
	p.mu.Lock()
	defer p.mu.Unlock()
	cut := time.Now().Add(-10 * time.Minute)
	for k, v := range p.seen {
		if v.at.Before(cut) {
			delete(p.seen, k)
		}
	}
}

// ---------------------------------------------------------------- pinning

// pinner keeps only addresses that accept a connection on 443. Some services
// round-robin across addresses a few of which are unreachable from inside the
// country; a device that draws a dead one stalls. Results are remembered so
// each address is probed at most every few minutes.
type pinner struct {
	mu   sync.Mutex
	good map[netip.Addr]pinResult
}

type pinResult struct {
	ok bool
	at time.Time
}

func (p *pinner) init() { p.good = map[netip.Addr]pinResult{} }

func (p *pinner) filter(ctx context.Context, answer []dns.RR) []dns.RR {
	var addrs []netip.Addr
	for _, rr := range answer {
		if a, ok := rr.(*dns.A); ok {
			if ip, ok := netip.AddrFromSlice(a.A.To4()); ok {
				addrs = append(addrs, ip)
			}
		}
	}
	if len(addrs) < 2 {
		return answer
	}
	ok := p.check(ctx, addrs)
	var out []dns.RR
	kept := 0
	for _, rr := range answer {
		if a, isA := rr.(*dns.A); isA {
			ip, _ := netip.AddrFromSlice(a.A.To4())
			if !ok[ip] {
				continue
			}
			kept++
		}
		out = append(out, rr)
	}
	if kept == 0 {
		return answer // nothing answered; better some chance than none
	}
	return out
}

func (p *pinner) check(ctx context.Context, addrs []netip.Addr) map[netip.Addr]bool {
	out := map[netip.Addr]bool{}
	var todo []netip.Addr
	p.mu.Lock()
	for _, a := range addrs {
		if r, ok := p.good[a]; ok && time.Since(r.at) < 5*time.Minute {
			out[a] = r.ok
		} else {
			todo = append(todo, a)
		}
	}
	p.mu.Unlock()
	if len(todo) == 0 {
		return out
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, a := range todo {
		wg.Add(1)
		go func(a netip.Addr) {
			defer wg.Done()
			d := net.Dialer{Timeout: 1500 * time.Millisecond}
			c, err := d.DialContext(ctx, "tcp4", netip.AddrPortFrom(a, 443).String())
			if err == nil {
				c.Close()
			}
			mu.Lock()
			out[a] = err == nil
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	p.mu.Lock()
	if len(p.good) > 5000 {
		p.good = map[netip.Addr]pinResult{}
	}
	for _, a := range todo {
		p.good[a] = pinResult{out[a], time.Now()}
	}
	p.mu.Unlock()
	return out
}
