package hub

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
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
	stuckL, _ := net.Listen("tcp", "127.0.0.1:0")
	defer stuckL.Close()
	var stuck []net.Conn
	go func() {
		for {
			c, err := stuckL.Accept()
			if err != nil {
				return
			}
			stuck = append(stuck, c) // held open, never read
		}
	}()
	h.dialHook = func(ctx context.Context, name, port string) (net.Conn, error) {
		var d net.Dialer
		if name == "stuck.playstation.com" {
			return d.DialContext(ctx, "tcp", stuckL.Addr().String())
		}
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
		PublicIP: "127.0.0.1", PortalPort: freePort(t), HubSNI: config.DefaultHubSNI, DNSListen: "127.0.0.1:" + strconv.Itoa(dnsPort),
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

	// ---- the same through the tunnel
	tunnelOn := "on"
	if err := st.UpdateGate(g.ID, nil, nil, &tunnelOn); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { gw.SyncNow(); return gw.Snapshot().Tunnel != "" })
	for i := 0; i < 3; i++ { // several streams over one connection
		tc, err := tls.Dial("tcp", gateCfg.Gate.HTTPSListen, &tls.Config{ServerName: "store.playstation.com", InsecureSkipVerify: true})
		if err != nil {
			t.Fatalf("through the tunnel: %v", err)
		}
		tc.Write([]byte("hello"))
		got, _ := io.ReadAll(tc)
		tc.Close()
		if len(got) != len(payload) {
			t.Fatalf("tunnel stream %d: received %d bytes, want %d", i, len(got), len(payload))
		}
	}
	if n := h.tunnelStreams.Load(); n != 3 {
		t.Fatalf("hub saw %d tunnel streams, want 3", n)
	}
	stuckThenSecond := func(mode string) {
		// One customer uploading faster than their service accepts must not
		// stall everybody else's streams on the shared tunnel.
		hog, err := net.Dial("tcp", gateCfg.Gate.HTTPSListen)
		if err != nil {
			t.Fatal(err)
		}
		defer hog.Close()
		hello := tlsHello(t, "stuck.playstation.com")
		hog.Write(hello)
		go func() {
			junk := make([]byte, 64*1024)
			for i := 0; i < 160; i++ { // 10 MB the far end never reads
				if _, err := hog.Write(junk); err != nil {
					return
				}
			}
		}()
		time.Sleep(1500 * time.Millisecond)
		done := make(chan error, 1)
		go func() {
			tc, err := tls.Dial("tcp", gateCfg.Gate.HTTPSListen, &tls.Config{ServerName: "store.playstation.com", InsecureSkipVerify: true})
			if err != nil {
				done <- err
				return
			}
			tc.Write([]byte("hello"))
			got, _ := io.ReadAll(tc)
			tc.Close()
			if len(got) != len(payload) {
				done <- fmt.Errorf("received %d bytes", len(got))
				return
			}
			done <- nil
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%s: second customer while the first is stuck: %v", mode, err)
			}
		case <-time.After(8 * time.Second):
			t.Fatalf("%s: one stuck upload blocked other customers", mode)
		}
		if s := gw.Snapshot().Tunnel; mode == "tunnel" && s != "up" {
			t.Fatalf("tunnel status %q", s)
		}
	}
	stuckThenSecond("tunnel")

	tunnelOff := "off"
	_ = st.UpdateGate(g.ID, nil, nil, &tunnelOff)
	waitFor(t, func() bool { gw.SyncNow(); return gw.Snapshot().Tunnel == "" })
	stuckThenSecond("direct")

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

	// ---- online payment through fake IDPay, ZarinPal and PayPing
	verified := map[string]int{}
	fake := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		reply := func(v any) { json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/v1.1/payment":
			if r.Header.Get("X-API-KEY") != "idpay-key" || in["amount"].(float64) != 1500000 {
				t.Errorf("idpay create: %v %v", r.Header.Get("X-API-KEY"), in)
			}
			w.WriteHeader(201)
			reply(map[string]string{"id": "idp-1", "link": "https://idpay.test/p/idp-1"})
		case "/v1.1/payment/verify":
			verified["idpay"]++
			reply(map[string]any{"status": 100, "track_id": "777", "id": "idp-1", "order_id": in["order_id"], "amount": "1500000"})
		case "/zp/pg/v4/payment/request.json":
			if in["merchant_id"] != "zp-merchant" || in["amount"].(float64) != 1500000 {
				t.Errorf("zarinpal create: %v", in)
			}
			reply(map[string]any{"data": map[string]any{"code": 100, "authority": "A0001"}, "errors": []any{}})
		case "/zp/pg/v4/payment/verify.json":
			verified["zarinpal"]++
			reply(map[string]any{"data": map[string]any{"code": 100, "ref_id": 12345}, "errors": []any{}})
		case "/pp/pay":
			if r.Header.Get("Authorization") != "Bearer pp-token" || in["amount"].(float64) != 150000 {
				t.Errorf("payping create: %v %v", r.Header.Get("Authorization"), in)
			}
			reply(map[string]string{"paymentCode": "PPCODE"})
		case "/pp/pay/verify":
			if in["paymentRefId"] != "R1" || in["paymentCode"] != "PPCODE" || in["amount"].(float64) != 150000 {
				t.Errorf("payping verify: %v", in)
			}
			verified["payping"]++
			reply(map[string]any{"amount": 150000, "cardNumber": "6037****1234"})
		default:
			t.Errorf("unexpected gateway call %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer fake.Close()
	gate.IDPayBase = fake.URL + "/v1.1"
	gate.ZarinpalBase = fake.URL + "/zp"
	gate.PayPingBase = fake.URL + "/pp"
	gate.PayClient = fake.Client()
	_ = st.SetSetting(store.SetIDPayKey, "idpay-key")
	_ = st.SetSetting(store.SetZarinpalMerchant, "zp-merchant")
	_ = st.SetSetting(store.SetPayPingToken, "pp-token")

	_, info := call("GET", "/api/info", nil)
	if gws := info["gateways"].([]any); len(gws) != 3 {
		t.Fatalf("enabled gateways: %v", gws)
	}

	noFollow := &http.Client{Jar: jar, Transport: web.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	back := func(method, path string, form url.Values) string {
		t.Helper()
		var resp *http.Response
		var err error
		if method == "GET" {
			resp, err = noFollow.Get(portal + path + "&" + form.Encode())
		} else {
			resp, err = noFollow.PostForm(portal+path, form)
		}
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.Header.Get("Location")
	}

	cases := []struct {
		gw, link, method string
		form             url.Values
		track            string
	}{
		{"idpay", "https://idpay.test/p/idp-1", "POST", url.Values{"status": {"10"}, "id": {"idp-1"}, "order_id": {"gz-1"}, "track_id": {"555"}}, "777"},
		{"zarinpal", fake.URL + "/zp/pg/StartPay/A0001", "GET", url.Values{"Authority": {"A0001"}, "Status": {"OK"}}, "12345"},
		{"payping", fake.URL + "/pp/pay/start/PPCODE", "POST", url.Values{"data": {`{"paymentRefId":"R1","paymentCode":"PPCODE","clientRefId":"gz-3","status":1}`}}, "R1"},
	}
	for n, c := range cases {
		order := "gz-" + strconv.Itoa(n+1)
		code, out := call("POST", "/api/pay/online", map[string]any{"plan_id": plan.ID, "gateway": c.gw})
		if code != 200 || out["link"] != c.link {
			t.Fatalf("%s: start: %d %v", c.gw, code, out)
		}
		if pays, _ := st.Payments("", 0, 50); len(pays) != n {
			t.Fatalf("%s: an unpaid checkout must not be listed (%d listed)", c.gw, len(pays))
		}
		if c.gw == "idpay" {
			// A callback naming a payment the gateway never issued is ignored.
			forged := url.Values{"status": {"10"}, "id": {"forged"}, "order_id": {order}}
			if loc := back("POST", "/api/pay/callback/idpay?o="+order, forged); loc != "/plans?pay=fail" || verified["idpay"] != 0 {
				t.Fatalf("forged callback: %q, verified %d", loc, verified["idpay"])
			}
		}
		before, _ := st.UserByName("sara")
		for i := 0; i < 2; i++ { // twice: must be credited once
			if loc := back(c.method, "/api/pay/callback/"+c.gw+"?o="+order, c.form); loc != "/plans?pay=ok" {
				t.Fatalf("%s: callback redirected to %q", c.gw, loc)
			}
		}
		if verified[c.gw] != 1 {
			t.Fatalf("%s: verified %d times, want 1", c.gw, verified[c.gw])
		}
		after, _ := st.UserByName("sara")
		if *after.ExpiresAt <= *before.ExpiresAt {
			t.Fatalf("%s: paying did not extend the plan", c.gw)
		}
		p, _ := st.Payment(int64(n + 1))
		if p.Status != store.PayApproved || p.TrackID != c.track || p.Method != c.gw {
			t.Fatalf("%s: payment after verify: %+v", c.gw, p)
		}
	}

	// A gateway switched off is gone from the page and refused if asked for.
	_ = st.SetSetting("pay_zarinpal_on", "0")
	_, info = call("GET", "/api/info", nil)
	if gws := info["gateways"].([]any); len(gws) != 2 {
		t.Fatalf("gateways after switching one off: %v", gws)
	}
	if code, out := call("POST", "/api/pay/online", map[string]any{"plan_id": plan.ID, "gateway": "zarinpal"}); code != 404 || out["error"] != "online_pay_off" {
		t.Fatalf("disabled gateway: %d %v", code, out)
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

// tlsHello returns the first bytes a TLS client sends for name.
func tlsHello(t *testing.T, name string) []byte {
	a, b := net.Pipe()
	go tls.Client(a, &tls.Config{ServerName: name, InsecureSkipVerify: true}).Handshake()
	b.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 16*1024)
	n, _ := b.Read(buf)
	a.Close()
	b.Close()
	return buf[:n]
}
