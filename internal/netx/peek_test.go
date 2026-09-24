package netx

import (
	"crypto/tls"
	"io"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestPeekSNIFromRealClientHello(t *testing.T) {
	a, b := net.Pipe()
	go func() {
		c := tls.Client(a, &tls.Config{ServerName: "Store.PlayStation.com", InsecureSkipVerify: true})
		_ = c.Handshake()
	}()
	name, conn, err := PeekSNI(b, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if name != "store.playstation.com" {
		t.Fatalf("got %q", name)
	}
	hdr := make([]byte, 5)
	if _, err := io.ReadFull(conn, hdr); err != nil || hdr[0] != 0x16 {
		t.Fatalf("replay broken: %v %x", err, hdr)
	}
}

func TestPeekSNIRejectsPlaintext(t *testing.T) {
	a, b := net.Pipe()
	go a.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
	if _, _, err := PeekSNI(b, time.Second); err != ErrNotTLS {
		t.Fatalf("want ErrNotTLS, got %v", err)
	}
}

func TestPeekHost(t *testing.T) {
	a, b := net.Pipe()
	req := "GET /pkg HTTP/1.1\r\nUser-Agent: x\r\nHOST: gst.prod.dl.playstation.net:80\r\n\r\nBODY"
	go a.Write([]byte(req))
	host, conn, err := PeekHost(b, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if host != "gst.prod.dl.playstation.net" {
		t.Fatalf("got %q", host)
	}
	got := make([]byte, len(req))
	if _, err := io.ReadFull(conn, got); err != nil || !strings.HasPrefix(string(got), "GET /pkg") || !strings.HasSuffix(string(got), "BODY") {
		t.Fatalf("replay broken: %v %q", err, got)
	}
}

func TestIsPublic(t *testing.T) {
	for s, want := range map[string]bool{
		"1.1.1.1": true, "10.0.0.1": false, "127.0.0.1": false, "100.64.1.1": false,
		"192.168.1.1": false, "::1": false, "2606:4700::1111": true, "::ffff:10.1.1.1": false,
	} {
		if got := IsPublic(netip.MustParseAddr(s)); got != want {
			t.Errorf("%s: got %v", s, got)
		}
	}
}
