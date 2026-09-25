package proto

import "testing"

func TestServicePort(t *testing.T) {
	for p, want := range map[int]bool{80: true, 443: true, 2053: true, 2083: true, 2087: true, 2096: true, 22: false, 25: false, 8443: false, 7443: false, 0: false} {
		if ServicePort(p) != want {
			t.Errorf("port %d: got %v", p, !want)
		}
	}
}
