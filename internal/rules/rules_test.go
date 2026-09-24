package rules

import "testing"

func TestLongestSuffixWins(t *testing.T) {
	s := Compile(Spec{
		Route:  []string{"playstation.net", "epicgames.com"},
		Direct: []string{"np.playstation.net"},
		Pin:    []string{"datarouter.ol.epicgames.com"},
	})
	cases := map[string]Action{
		"playstation.net":             Route,
		"gst.prod.dl.playstation.net": Route,
		"PS5.NP.PlayStation.net.":     Direct,
		"np.playstation.net":          Direct,
		"datarouter.ol.epicgames.com": Pin,
		"store.epicgames.com":         Route,
		"notplaystation.net":          None,
		"example.org":                 None,
	}
	for name, want := range cases {
		if got, _ := s.Match(name); got != want {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
}

func TestExceptionBeatsRouteOnSameName(t *testing.T) {
	s := Compile(Spec{Route: []string{"core.windows.net"}, Direct: []string{"core.windows.net"}})
	if a, _ := s.Match("x.core.windows.net"); a != Direct {
		t.Fatalf("got %v", a)
	}
}

func TestValidDomain(t *testing.T) {
	good := []string{"example.com", "a-b.example.co", "*.github.io", "xn--mgbai9azgqp6j.ir"}
	bad := []string{"", "com", "-a.com", "a..com", "1.2.3.4", "a b.com", "a/b.com", "ex;ample.com"}
	for _, d := range good {
		if !ValidDomain(d) {
			t.Errorf("%q should be valid", d)
		}
	}
	for _, d := range bad {
		if ValidDomain(d) {
			t.Errorf("%q should be invalid", d)
		}
	}
}
