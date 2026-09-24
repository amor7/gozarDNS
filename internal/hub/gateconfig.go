package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"

	"github.com/amor7/gozarDNS/internal/catalog"
	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/rules"
	"github.com/amor7/gozarDNS/internal/store"
)

// CustomGroup is the pseudo-group that holds the operator's own domains. A
// plan routes them when it lists this key among its groups.
const CustomGroup = "custom/main"

// exceptions returns the names excepted from routing for every plan: the
// built-in rules that are switched on, plus the operator's own "direct" names.
func (s *Hub) exceptions(custom []store.CustomDomain) (direct, pin []string) {
	var off []string
	_ = json.Unmarshal([]byte(s.store.Setting(store.SetDisabledRules)), &off)
	for _, r := range s.cat.Rules {
		if slices.Contains(off, r.ID) {
			continue
		}
		switch r.Action {
		case catalog.ActionDirect:
			direct = append(direct, r.Domains...)
		case catalog.ActionPin:
			pin = append(pin, r.Domains...)
		}
	}
	for _, d := range custom {
		if d.Action == "direct" {
			direct = append(direct, d.Domain)
		}
	}
	sort.Strings(direct)
	sort.Strings(pin)
	return
}

// planSpec is the full routing table of one plan.
func (s *Hub) planSpec(p *store.Plan, custom []store.CustomDomain, direct, pin []string) rules.Spec {
	route := s.cat.Domains(p.Groups, p.Excluded)
	if slices.Contains(p.Groups, CustomGroup) {
		for _, d := range custom {
			if d.Action == "route" && !slices.Contains(p.Excluded, d.Domain) {
				route = append(route, d.Domain)
			}
		}
		sort.Strings(route)
	}
	return rules.Spec{Route: route, Direct: direct, Pin: pin}
}

// gateConfig builds what one gate should be doing right now, and a revision
// that changes exactly when that does.
func (s *Hub) gateConfig(g *store.Gate) (*proto.GateConfig, string, error) {
	plans, err := s.store.Plans()
	if err != nil {
		return nil, "", err
	}
	custom, err := s.store.CustomDomains()
	if err != nil {
		return nil, "", err
	}
	direct, pin := s.exceptions(custom)
	cfg := &proto.GateConfig{
		Plans:   map[int64]rules.Spec{},
		Clients: []proto.Client{},
		Brand:   s.store.Setting(store.SetBrand),
	}
	for _, p := range plans {
		cfg.Plans[p.ID] = s.planSpec(p, custom, direct, pin)
	}
	if def, err := s.store.DefaultPlan(); err == nil {
		cfg.DefaultPlan = def.ID
	}

	users, _, err := s.store.Users(store.UserFilter{})
	if err != nil {
		return nil, "", err
	}
	for _, u := range users {
		if u.State != store.StateActive || u.Devices == 0 {
			continue
		}
		devs, err := s.store.Devices(u.ID)
		if err != nil {
			return nil, "", err
		}
		for _, d := range devs {
			cfg.Clients = append(cfg.Clients, proto.Client{IP: d.IP, User: u.ID, Name: u.Username, Plan: *u.PlanID, Kbps: u.SpeedKbps})
		}
	}
	sort.Slice(cfg.Clients, func(i, j int) bool { return cfg.Clients[i].IP < cfg.Clients[j].IP })

	switch g.Enforce {
	case "on":
		cfg.Enforce = true
	case "off":
		cfg.Enforce = false
	default:
		// Open until the first customer registers an address: locking a
		// fresh gate against an empty list would lock out its own operator
		// while they test it.
		cfg.Enforce = s.store.DeviceCount() > 0
	}

	b, _ := json.Marshal(cfg)
	sum := sha256.Sum256(b)
	return cfg, hex.EncodeToString(sum[:8]), nil
}
