// Package catalog is the built-in list of services a plan can route, grouped
// the way a customer thinks about them: by brand, then by what part of the
// brand (the store, or the multi-gigabyte downloads).
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed catalog.json
var raw []byte

type Text struct {
	FA string `json:"fa"`
	EN string `json:"en"`
}

type Category struct {
	ID   string `json:"id"`
	Name Text   `json:"name"`
}

type Group struct {
	ID      string   `json:"id"`
	Name    Text     `json:"name"`
	Heavy   bool     `json:"heavy"`
	Domains []string `json:"domains"`
}

type Service struct {
	ID       string  `json:"id"`
	Name     Text    `json:"name"`
	Category string  `json:"category"`
	Color    string  `json:"color"`
	Groups   []Group `json:"groups"`
}

// Rule action values. "direct" resolves a name normally even under a routed
// parent; "pin" does the same but only hands out addresses that answer.
const (
	ActionRoute  = "route"
	ActionDirect = "direct"
	ActionPin    = "pin"
)

type Rule struct {
	ID      string   `json:"id"`
	Action  string   `json:"action"`
	Title   Text     `json:"title"`
	Why     Text     `json:"why"`
	Domains []string `json:"domains"`
}

type Catalog struct {
	Version    int        `json:"version"`
	Categories []Category `json:"categories"`
	Services   []Service  `json:"services"`
	Rules      []Rule     `json:"rules"`
}

var builtin *Catalog

func init() {
	c := &Catalog{}
	if err := json.Unmarshal(raw, c); err != nil {
		panic(fmt.Sprintf("catalog.json: %v", err))
	}
	builtin = c
}

// Builtin returns the catalogue compiled into this binary.
func Builtin() *Catalog { return builtin }

// GroupKey names a group across the whole catalogue: "playstation/download".
func GroupKey(service, group string) string { return service + "/" + group }

// AllGroupKeys lists every group, in catalogue order.
func (c *Catalog) AllGroupKeys() []string {
	var out []string
	for _, s := range c.Services {
		for _, g := range s.Groups {
			out = append(out, GroupKey(s.ID, g.ID))
		}
	}
	return out
}

// Domains returns the domains of the given groups minus the excluded ones,
// sorted and de-duplicated.
func (c *Catalog) Domains(groups []string, excluded []string) []string {
	want := make(map[string]bool, len(groups))
	for _, k := range groups {
		want[k] = true
	}
	skip := make(map[string]bool, len(excluded))
	for _, d := range excluded {
		skip[strings.ToLower(d)] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range c.Services {
		for _, g := range s.Groups {
			if !want[GroupKey(s.ID, g.ID)] {
				continue
			}
			for _, d := range g.Domains {
				if !skip[d] && !seen[d] {
					seen[d] = true
					out = append(out, d)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// Owner reports which service and group a domain belongs to, if any.
func (c *Catalog) Owner(domain string) (svc *Service, grp *Group) {
	for i := range c.Services {
		s := &c.Services[i]
		for j := range s.Groups {
			for _, d := range s.Groups[j].Domains {
				if d == domain {
					return s, &s.Groups[j]
				}
			}
		}
	}
	return nil, nil
}
