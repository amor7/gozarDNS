package hub

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"

	"github.com/amor7/gozarDNS/internal/certs"
	"github.com/amor7/gozarDNS/internal/config"
	"github.com/amor7/gozarDNS/internal/ctl"
	"github.com/amor7/gozarDNS/internal/gate"
	"github.com/amor7/gozarDNS/internal/store"
)

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// TestEndToEnd runs a hub and a gate in one process and walks a customer
// through: sign-up on the portal, DNS through the gate, a TLS connection
// carried to the "service", and usage arriving back at the hub.
func TestEndToEnd(t *testing.T) {
	dir, err := os.MkdirTemp("", "gz")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ctl.SocketPath = filepath.Join(dir, "c.sock")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// The "real service" the customer wants: a TLS server on loopback that
	// the hub's dial hook sends every routed name to.
	svcCert, _ := certs.SelfSigned(filepath.Join(dir, "svc"), "svc", []string{"127.0.0.1"})
	svcL, _ := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{svcCert}})
	defer svcL.Close()
	payload := bytes.Repeat([]byte("x"), 200_000)
	go func() {
		for {
			c, err := svcL.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 5)
				if _, err := io.ReadFull(c, buf); err != nil {
					return
				}
				c.Write(payload)
			}(c)
		}
	}()

	// ---- hub
	hubCfg := &config.Config{Role: config.RoleHub, DataDir: filepath.Join(dir, "hub"), Hub: &config.Hub{
		PublicIP: "127.0.0.1", AdminPort: freePort(t), ControlPort: freePort(t),
		HTTPSPort: freePort(t), HTTPPort: freePort(t), IPv6Egress: "off",
	}}
	st, err := store.Open(filepath.Join(hubCfg.DataDir, "gozar.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	plan := &store.Plan{Name: "Games", Price: 150000, MaxDevices: 2, QuotaBytes: 10 << 30, DurationDays: 30,
		Groups: []string{"playstation/online", "playstation/download"}, Public: true, IsDefault: true}
	if err := st.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	_ = st.SetSetting(store.SetSignupPlan, strconv.FormatInt(plan.ID, 10))
	g, secret, _ := st.CreateGate("test-gate")

	h, err := New(hubCfg, st, log)
	if err != nil {
		t.Fatal(err)
	}
	h.dialHook = func(ctx context.Context, name, port string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", svcL.Addr().String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Run(ctx)

	// ---- gate
	dnsPort := freePort(t)
	gateCfg := &config.Config{Role: config.RoleGate, DataDir: filepath.Join(dir, "gate"), Gate: &config.Gate{
		HubAddr: "127.0.0.1", HubControlPort: hubCfg.Hub.ControlPort, HubHTTPSPort: hubCfg.Hub.HTTPSPort,
		HubHTTPPort: hubCfg.Hub.HTTPPort, Fingerprint: h.ControlFingerprint(), GateID: g.ID, Secret: secret,
		PublicIP: "127.0.0.1", PortalPort: freePort(t), DNSListen: "127.0.0.1:" + strconv.Itoa(dnsPort),
		HTTPSListen: "127.0.0.1:" + strconv.Itoa(freePort(t)), HTTPListen: "127.0.0.1:" + strconv.Itoa(freePort(t)),
		Upstreams: []string{"127.0.0.1:1"}, CleanUpstreams: []string{"127.0.0.1:1"},
	}}
	os.MkdirAll(gateCfg.DataDir, 0o700)
	gw, err := gate.New(gateCfg, log)
	if err != nil {
		t.Fatal(err)
	}
	go gw.Run(ctx)
	time.Sleep(700 * time.Millisecond)

	// ---- customer signs up through the gate's portal
	jar, _ := cookiejar.New(nil)
	web := &http.Client{Jar: jar, Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	portal := "https://127.0.0.1:" + strconv.Itoa(gateCfg.Gate.PortalPort)
	call := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, portal+path, rd)
		req.Header.Set("X-Requested-With", "gozar")
		req.Header.Set("Content-Type", "application/json")
		resp, err := web.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if code, out := call("GET", "/api/info", nil); code != 200 || out["dns"] != "127.0.0.1" {
		t.Fatalf("info: %d %v", code, out)
	}
	if code, out := call("POST", "/api/signup", map[string]string{"username": "Sara", "password": "secret123"}); code != 200 {
		t.Fatalf("signup: %d %v", code, out)
	}
	code, me := call("GET", "/api/me", nil)
	if code != 200 {
		t.Fatalf("me: %d %v", code, me)
	}
	user := me["user"].(map[string]any)
	if user["username"] != "sara" || user["state"] != "active" || me["this_device"] == nil {
		t.Fatalf("after sign-up the device should be registered and the plan active: %v", me)
	}
	if code, _ := call("POST", "/api/signup", map[string]string{"username": "sara", "password": "secret123"}); code != 409 {
		t.Fatalf("duplicate sign-up: %d", code)
	}

	// Push a sync so the gate learns about the new device right away.
	waitFor(t, func() bool {
		st, _ := statusOf(gw)
		return st
	})

	// ---- DNS through the gate
	q := new(dns.Msg)
	q.SetQuestion("gst.prod.dl.playstation.net.", dns.TypeA)
	r, err := dns.Exchange(q, "127.0.0.1:"+strconv.Itoa(dnsPort))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Answer) != 1 || r.Answer[0].(*dns.A).A.String() != "127.0.0.1" {
		t.Fatalf("routed name should resolve to the gate: %v", r)
	}

	// ---- a TLS connection, carried gate -> hub -> service
	conn, err := tls.Dial("tcp", gateCfg.Gate.HTTPSListen, &tls.Config{ServerName: "store.playstation.com", InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("through the gate: %v", err)
	}
	conn.Write([]byte("hello"))
	got, _ := io.ReadAll(conn)
	conn.Close()
	if len(got) != len(payload) {
		t.Fatalf("received %d bytes, want %d", len(got), len(payload))
	}

	// A name outside the plan must not be carried.
	if c, err := tls.Dial("tcp", gateCfg.Gate.HTTPSListen, &tls.Config{ServerName: "example.org", InsecureSkipVerify: true}); err == nil {
		c.Close()
		t.Fatal("a name outside the plan was proxied")
	}

	// ---- usage reaches the hub on the next sync
	u, _ := st.UserByName("sara")
	waitFor(t, func() bool {
		gw.SyncNow()
		u, _ = st.UserByName("sara")
		return u.UsedBytes >= int64(len(payload))
	})

	// ---- online payment through a fake IDPay
	var verifyCalls int
	fake := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" {
			w.WriteHeader(403)
			return
		}
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		switch r.URL.Path {
		case "/payment":
			if in["amount"].(float64) != 1500000 { // 150,000 toman in rial
				t.Errorf("amount sent to IDPay: %v", in["amount"])
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]string{"id": "pay-abc", "link": "https://idpay.example/p/pay-abc"})
		case "/payment/verify":
			verifyCalls++
			json.NewEncoder(w).Encode(map[string]any{"status": 100, "track_id": "777", "id": "pay-abc", "order_id": in["order_id"], "amount": "1500000"})
		}
	}))
	defer fake.Close()
	gate.IDPayBase = fake.URL
	gate.IDPayClient = fake.Client()
	_ = st.SetSetting(store.SetIDPayKey, "test-key")

	code, out := call("POST", "/api/pay/online", map[string]int64{"plan_id": plan.ID})
	if code != 200 || out["link"] != "https://idpay.example/p/pay-abc" {
		t.Fatalf("pay online: %d %v", code, out)
	}
	pays, _ := st.Payments("", 0, 10)
	var order int64
	for _, p := range pays {
		if p.Method == "idpay" {
			order = p.ID
		}
	}
	if order != 0 {
		t.Fatal("an unpaid checkout must not be listed")
	}
	before, _ := st.UserByName("sara")
	noFollow := &http.Client{Jar: jar, Transport: web.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// A forged callback that IDPay would not verify must not credit anything:
	// here the id does not match the one IDPay issued.
	form := url.Values{"status": {"10"}, "id": {"forged"}, "order_id": {"gz-1"}, "track_id": {"1"}}
	resp2, err := noFollow.PostForm(portal+"/api/pay/callback", form)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if loc := resp2.Header.Get("Location"); loc != "/plans?pay=fail" {
		t.Fatalf("forged callback redirected to %q", loc)
	}
	if verifyCalls != 0 {
		t.Fatal("a mismatched id must be refused before verifying")
	}
	// The real callback.
	for _, oid := range []string{"gz-1", "gz-1"} { // twice: must count once
		form = url.Values{"status": {"10"}, "id": {"pay-abc"}, "order_id": {oid}, "track_id": {"555"}}
		resp2, err = noFollow.PostForm(portal+"/api/pay/callback", form)
		if err != nil {
			t.Fatal(err)
		}
		resp2.Body.Close()
		if loc := resp2.Header.Get("Location"); loc != "/plans?pay=ok" {
			t.Fatalf("callback redirected to %q", loc)
		}
	}
	if verifyCalls != 1 {
		t.Fatalf("verified %d times, want 1", verifyCalls)
	}
	after, _ := st.UserByName("sara")
	if *after.ExpiresAt <= *before.ExpiresAt {
		t.Fatal("paying did not extend the plan")
	}
	p2, _ := st.Payment(1)
	if p2.Status != store.PayApproved || p2.TrackID != "777" {
		t.Fatalf("payment after verify: %+v", p2)
	}

	// ---- registering another network's address by hand
	if code, out := call("POST", "/api/devices", map[string]string{"ip": "192.168.1.20"}); code != 400 || out["error"] != "ip_private" {
		t.Fatalf("private address: %d %v", code, out)
	}
	if code, out := call("POST", "/api/devices", map[string]string{"ip": "۵.۱۲۰.۴۴.۱۹", "label": "PS5"}); code != 200 {
		t.Fatalf("manual address: %d %v", code, out)
	}
	if d, err := st.DeviceByIP("5.120.44.19"); err != nil || d.Label != "PS5" {
		t.Fatalf("manual device: %v %v", d, err)
	}

	// ---- the admin console
	adminJar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: adminJar, Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	_, _ = st.CreateAdmin("root", "x")
	aBase := "https://127.0.0.1:" + strconv.Itoa(hubCfg.Hub.AdminPort)
	req, _ := http.NewRequest("GET", aBase+"/api/overview", nil)
	resp, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("overview without session: %d", resp.StatusCode)
	}
	req, _ = http.NewRequest("POST", aBase+"/api/login", strings.NewReader(`{"username":"root","password":"x"}`))
	resp, _ = admin.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("a POST without the AJAX header must be refused, got %d", resp.StatusCode)
	}
}

func statusOf(gw *gate.Gate) (bool, error) {
	gw.SyncNow()
	s := gw.Snapshot()
	return s.Clients > 0, nil
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}
