// Package rules decides what the gate does with a name: send it through the
// hub, resolve it normally, or resolve it normally but only to addresses
// known to answer.
package rules

import (
	"strings"
)

type Action uint8

const (
	None Action = iota
	Route
	Direct
	Pin
)

func (a Action) String() string {
	switch a {
	case Route:
		return "route"
	case Direct:
		return "direct"
	case Pin:
		return "pin"
	}
	return "none"
}

func Parse(s string) Action {
	switch s {
	case "route":
		return Route
	case "direct":
		return Direct
	case "pin":
		return Pin
	}
	return None
}

// Set is the compiled routing table for one plan. Matching is by suffix on
// label boundaries and the longest suffix wins, so "np.playstation.net" can be
// direct while "playstation.net" is routed.
type Set struct {
	m map[string]Action
}

// Spec is the wire form of a Set, as the hub sends it to gates.
type Spec struct {
	Route  []string `json:"route"`
	Direct []string `json:"direct,omitempty"`
	Pin    []string `json:"pin,omitempty"`
}

func Compile(s Spec) *Set {
	set := &Set{m: make(map[string]Action, len(s.Route)+len(s.Direct)+len(s.Pin))}
	add := func(list []string, a Action) {
		for _, d := range list {
			if d = Normalize(d); d != "" {
				set.m[d] = a
			}
		}
	}
	// Later wins on an exact duplicate: an exception beats a route.
	add(s.Route, Route)
	add(s.Pin, Pin)
	add(s.Direct, Direct)
	return set
}

// Match returns the action for name and the suffix that decided it.
func (s *Set) Match(name string) (Action, string) {
	if s == nil {
		return None, ""
	}
	name = Normalize(name)
	for name != "" {
		if a, ok := s.m[name]; ok {
			return a, name
		}
		i := strings.IndexByte(name, '.')
		if i < 0 {
			break
		}
		name = name[i+1:]
	}
	return None, ""
}

func (s *Set) Len() int {
	if s == nil {
		return 0
	}
	return len(s.m)
}

// Normalize lowercases a name and strips a trailing dot and any "*." prefix.
func Normalize(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimSuffix(d, ".")
	d = strings.TrimPrefix(d, "*.")
	return d
}

// ValidDomain accepts plain hostnames only: letters, digits, hyphens, dots,
// at least two labels, nothing that could smuggle syntax anywhere.
func ValidDomain(d string) bool {
	d = Normalize(d)
	if len(d) < 3 || len(d) > 253 || !strings.Contains(d, ".") {
		return false
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	// A bare IPv4 address is not a domain.
	last := d[strings.LastIndexByte(d, '.')+1:]
	allDigits := true
	for _, r := range last {
		if r < '0' || r > '9' {
			allDigits = false
		}
	}
	return !allDigits
}
