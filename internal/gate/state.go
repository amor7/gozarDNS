package gate

import (
	"encoding/json"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/amor7/gozarDNS/internal/dnsd"
	"github.com/amor7/gozarDNS/internal/netx"
	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/rules"
)

// state is what the gate currently believes, rebuilt whole from each new
// config the hub sends. Lookups take a read lock; a rebuild swaps pointers.
type state struct {
	mu       sync.RWMutex
	revision string
	enforce  bool
	brand    string
	clients  map[netip.Addr]*client
	plans    map[int64]*rules.Set
	fallback *rules.Set // the default plan, for anybody while not enforcing

	// Counters survive rebuilds: they belong to the account, not the config.
	umu    sync.Mutex
	usage  map[int64]*meter
	limits map[int64]*limit
	seenMu sync.Mutex
	seen   map[string]int64
}

type client struct {
	user int64
	name string
	plan int64
	kbps int
}

type meter struct {
	up, down atomic.Int64
}

type limit struct {
	kbps int
	l    *rate.Limiter
}

func newState() *state {
	return &state{
		clients: map[netip.Addr]*client{},
		plans:   map[int64]*rules.Set{},
		usage:   map[int64]*meter{},
		limits:  map[int64]*limit{},
		seen:    map[string]int64{},
	}
}

func (s *state) apply(rev string, c *proto.GateConfig) {
	clients := make(map[netip.Addr]*client, len(c.Clients))
	for _, cl := range c.Clients {
		ip, err := netip.ParseAddr(cl.IP)
		if err != nil {
			continue
		}
		clients[ip.Unmap()] = &client{user: cl.User, name: cl.Name, plan: cl.Plan, kbps: cl.Kbps}
	}
	plans := make(map[int64]*rules.Set, len(c.Plans))
	for id, spec := range c.Plans {
		plans[id] = rules.Compile(spec)
	}
	s.mu.Lock()
	s.revision = rev
	s.enforce = c.Enforce
	s.brand = c.Brand
	s.clients = clients
	s.plans = plans
	s.fallback = plans[c.DefaultPlan]
	s.mu.Unlock()

	// Speed limits: keep a user's limiter while their speed is unchanged, so
	// a config refresh does not reset anybody's pacing mid-download.
	s.umu.Lock()
	want := map[int64]int{}
	for _, cl := range clients {
		want[cl.user] = cl.kbps
	}
	for uid, l := range s.limits {
		if want[uid] != l.kbps {
			delete(s.limits, uid)
		}
	}
	for uid, kbps := range want {
		if _, ok := s.limits[uid]; !ok && kbps > 0 {
			s.limits[uid] = &limit{kbps: kbps, l: netx.NewLimiter(kbps)}
		}
	}
	s.umu.Unlock()
}

// who resolves a source address to what the gate should do for it.
func (s *state) who(ip netip.Addr) (c *client, set *rules.Set, allowed bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if cl, ok := s.clients[ip]; ok {
		return cl, s.plans[cl.plan], true
	}
	if !s.enforce {
		return nil, s.fallback, true
	}
	return nil, nil, false
}

func (s *state) decide(ip netip.Addr) dnsd.Decision {
	c, set, ok := s.who(ip)
	d := dnsd.Decision{Allowed: ok, Set: set}
	if c != nil {
		d.UserID, d.Name = c.user, c.name
		s.touch(ip)
	}
	return d
}

func (s *state) touch(ip netip.Addr) {
	s.seenMu.Lock()
	s.seen[ip.String()] = time.Now().Unix()
	s.seenMu.Unlock()
}

func (s *state) meter(uid int64) *meter {
	s.umu.Lock()
	defer s.umu.Unlock()
	m, ok := s.usage[uid]
	if !ok {
		m = &meter{}
		s.usage[uid] = m
	}
	return m
}

func (s *state) limiter(uid int64) *rate.Limiter {
	s.umu.Lock()
	defer s.umu.Unlock()
	if l, ok := s.limits[uid]; ok {
		return l.l
	}
	return nil
}

// drain takes everything counted since the last drain.
func (s *state) drain() (map[int64]proto.Usage, map[string]int64) {
	out := map[int64]proto.Usage{}
	s.umu.Lock()
	for uid, m := range s.usage {
		u := proto.Usage{Up: m.up.Swap(0), Down: m.down.Swap(0)}
		if u.Up+u.Down > 0 {
			out[uid] = u
		}
	}
	s.umu.Unlock()
	s.seenMu.Lock()
	seen := s.seen
	s.seen = map[string]int64{}
	s.seenMu.Unlock()
	return out, seen
}

func (s *state) info() (rev string, enforce bool, clients, plans int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revision, s.enforce, len(s.clients), len(s.plans)
}

func (s *state) userByName(name string) (int64, []netip.Addr) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var uid int64
	var ips []netip.Addr
	for ip, c := range s.clients {
		if c.name == name {
			uid = c.user
			ips = append(ips, ip)
		}
	}
	return uid, ips
}

// ---------------------------------------------------------------- cache

// The last config is kept on disk so a gate that restarts while the hub is
// unreachable keeps serving the customers it already knew about.
type cached struct {
	Revision string            `json:"revision"`
	Config   *proto.GateConfig `json:"config"`
	Saved    time.Time         `json:"saved"`
}

func saveCache(path, rev string, c *proto.GateConfig) error {
	b, err := json.Marshal(cached{Revision: rev, Config: c, Saved: time.Now()})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadCache(path string) (*cached, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c cached
	if err := json.Unmarshal(b, &c); err != nil || c.Config == nil {
		return nil, err
	}
	return &c, nil
}
