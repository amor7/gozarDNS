// Command devstack runs a hub and a gate on this machine with demo data, for
// working on the web apps:
//
//	go run ./tools/devstack
//
// Admin console:   https://127.0.0.1:12053   (admin / admin12345)
// Customer portal: https://127.0.0.1:18843   (sara / sara1234)
//
// Then `npm run dev:admin` / `npm run dev:portal` in web/ for hot reload.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/amor7/gozarDNS/internal/auth"
	"github.com/amor7/gozarDNS/internal/catalog"
	"github.com/amor7/gozarDNS/internal/config"
	"github.com/amor7/gozarDNS/internal/ctl"
	"github.com/amor7/gozarDNS/internal/gate"
	"github.com/amor7/gozarDNS/internal/hub"
	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/store"
	"github.com/amor7/gozarDNS/internal/sysmetrics"
)

func main() {
	dir := flag.String("dir", ".devstack", "where the demo data lives")
	fresh := flag.Bool("fresh", false, "throw away the demo data and seed again")
	flag.Parse()
	if *fresh {
		os.RemoveAll(*dir)
	}
	abs, _ := filepath.Abs(*dir)
	ctl.SocketPath = filepath.Join(os.TempDir(), "gozar-dev.sock")
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	hubCfg := &config.Config{Role: config.RoleHub, DataDir: filepath.Join(abs, "hub"), Hub: &config.Hub{
		PublicIP: "127.0.0.1", AdminPort: 12053, ControlPort: 17443, HTTPSPort: 18443, HTTPPort: 18080, IPv6Egress: "off",
	}}
	os.MkdirAll(hubCfg.DataDir, 0o700)
	st, err := store.Open(filepath.Join(hubCfg.DataDir, "gozar.db"))
	must(err)
	defer st.Close()

	h, err := hub.New(hubCfg, st, log)
	must(err)

	gateFile := filepath.Join(abs, "gate.json")
	var gw config.Gate
	if b, err := os.ReadFile(gateFile); err == nil {
		must(json.Unmarshal(b, &gw))
	} else {
		g := seed(st)
		gw = config.Gate{
			HubAddr: "127.0.0.1", HubControlPort: 17443, HubHTTPSPort: 18443, HubHTTPPort: 18080,
			Fingerprint: h.ControlFingerprint(), GateID: g.id, Secret: g.secret, PublicIP: "127.0.0.1",
			HubSNI:     config.DefaultHubSNI,
			PortalPort: 18843, DNSListen: "127.0.0.1:15353", HTTPSListen: "127.0.0.1:19443", HTTPListen: "127.0.0.1:19080",
			Upstreams: []string{"1.1.1.1"}, CleanUpstreams: []string{"1.1.1.1"},
		}
		b, _ := json.MarshalIndent(gw, "", "  ")
		must(os.WriteFile(gateFile, b, 0o600))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		if err := h.Run(ctx); err != nil {
			log.Error("hub stopped", "err", err)
			stop()
		}
	}()
	gateCfg := &config.Config{Role: config.RoleGate, DataDir: filepath.Join(abs, "gate"), Gate: &gw}
	os.MkdirAll(gateCfg.DataDir, 0o700)
	g, err := gate.New(gateCfg, log)
	must(err)
	go func() {
		if err := g.Run(ctx); err != nil {
			log.Error("gate stopped", "err", err)
			stop()
		}
	}()
	fmt.Println()
	fmt.Println("  admin console    https://127.0.0.1:12053   admin / admin12345")
	fmt.Println("  customer portal  https://127.0.0.1:18843   sara / sara1234")
	fmt.Println("  gate DNS         127.0.0.1:15353")
	fmt.Println()
	<-ctx.Done()
	time.Sleep(300 * time.Millisecond)
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "devstack:", err)
		os.Exit(1)
	}
}

type seededGate struct{ id, secret string }

var people = []struct{ user, name string }{
	{"sara", "سارا"}, {"ali.r", "علی رضایی"}, {"mehdi_gamer", "مهدی"}, {"neda", "ندا کریمی"}, {"reza92", "رضا"},
	{"parisa", "پریسا"}, {"hossein", "حسین احمدی"}, {"maryam.dev", "مریم"}, {"kian", "کیان"}, {"arash", "آرش"},
	{"shirin", "شیرین"}, {"omid", "امید"}, {"yasmin", "یاسمین"}, {"babak", "بابک"}, {"leila", "لیلا"},
	{"farhad", "فرهاد"}, {"nazanin", "نازنین"}, {"pouya", "پویا"}, {"mina", "مینا"}, {"amir.k", "امیر"},
	{"sina", "سینا"}, {"tara", "تارا"}, {"dariush", "داریوش"}, {"elham", "الهام"},
}

func seed(st *store.Store) seededGate {
	r := rand.New(rand.NewPCG(7, 7))
	now := time.Now()
	const gb = int64(1) << 30
	_, _ = st.CreateAdmin("admin", auth.HashPassword("admin12345"))
	_ = st.SetSetting(store.SetBrand, "Gozar")
	_ = st.SetSetting(store.SetPaymentInfo, "6037 9975 1234 5678\nبانک ملی — به نام علی رضایی")
	_ = st.SetSetting(store.SetSupport, "تلگرام: @gozar_support\nپاسخ‌گویی ۹ صبح تا ۱۱ شب")
	_ = st.SetSetting(store.SetNotice, "سرویس Xbox از امشب با سرعت بیشتری کار می‌کند. 🎮")

	cat := catalog.Builtin()
	var light, all, games []string
	for _, s := range cat.Services {
		for _, g := range s.Groups {
			k := catalog.GroupKey(s.ID, g.ID)
			all = append(all, k)
			if !g.Heavy {
				light = append(light, k)
			}
			if s.Category == "games" {
				games = append(games, k)
			}
		}
	}
	plans := []*store.Plan{
		{Name: "استاندارد", Description: "همهٔ سرویس‌ها به‌جز دانلود بازی کنسول", Price: 150000, QuotaBytes: 30 * gb, DurationDays: 30, MaxDevices: 2, Groups: append(light, hub.CustomGroup), Public: true, IsDefault: true, Color: "#7C5CFF", Sort: 1},
		{Name: "گیمر", Description: "همه‌چیز، همراه با دانلود بازی", Price: 350000, QuotaBytes: 150 * gb, DurationDays: 30, MaxDevices: 3, Groups: append(all, hub.CustomGroup), Public: true, Color: "#16C8A0", Sort: 2},
		{Name: "فقط کنسول", Description: "پلی‌استیشن، ایکس‌باکس و بازی‌های PC", Price: 220000, QuotaBytes: 80 * gb, DurationDays: 30, MaxDevices: 1, Groups: games, Public: true, Color: "#F59E0B", Sort: 3, Renewal: "reset"},
	}
	for _, p := range plans {
		must(st.SavePlan(p))
	}
	_ = st.SetSetting(store.SetSignupPlan, "0")

	g, secret, err := st.CreateGate("tehran-1")
	must(err)
	g2, _, _ := st.CreateGate("shiraz-1")

	for i, p := range people {
		plan := plans[r.IntN(len(plans))]
		u, err := st.CreateUser(store.NewUser{Username: p.user, DisplayName: p.name, Password: auth.HashPassword(p.user + "1234"),
			Phone: fmt.Sprintf("0912%07d", r.IntN(9999999))})
		must(err)
		_ = st.SeedCreatedAt(u.ID, now.AddDate(0, 0, -r.IntN(120)-5).Unix())
		switch {
		case i == 0: // sara: the portal demo account, comfortably mid-period
			plan = plans[1]
			must(st.ApplyPlan(u.ID, plan.ID))
			exp := now.AddDate(0, 0, 18).Unix()
			used := int64(614) * gb / 10
			_ = st.UpdateUser(u.ID, store.UserPatch{ExpiresAt: &exp, UsedBytes: &used})
			_, _ = st.ClaimIP(u.ID, "127.0.0.1", "PS5 خانه", 0)
			_, _ = st.ClaimIP(u.ID, "5.120.44.19", "گوشی", 0)
		case i%9 == 4: // never picked a plan
		default:
			must(st.ApplyPlan(u.ID, plan.ID))
			days := r.IntN(34) - 3
			exp := now.AddDate(0, 0, days).Unix()
			used := int64(r.Float64() * float64(plan.QuotaBytes) * 1.05)
			_ = st.UpdateUser(u.ID, store.UserPatch{ExpiresAt: &exp, UsedBytes: &used})
			if i%11 == 7 {
				off := false
				_ = st.UpdateUser(u.ID, store.UserPatch{Enabled: &off})
			}
			for d := 0; d < 1+r.IntN(plan.MaxDevices); d++ {
				ip := fmt.Sprintf("%d.%d.%d.%d", []int{5, 31, 37, 46, 78, 89, 91, 151, 185}[r.IntN(9)], r.IntN(250)+1, r.IntN(250), r.IntN(250)+1)
				_, _ = st.ClaimIP(u.ID, ip, []string{"PS5", "Xbox", "لپ‌تاپ", "گوشی", "مودم خانه", ""}[r.IntN(6)], 0)
				_ = st.SeedDeviceSeen(ip, now.Add(-time.Duration(r.IntN(3*86400))*time.Second).Unix())
			}
		}
		for d := 0; d < 30; d++ {
			if r.Float64() < 0.2 && i != 0 {
				continue
			}
			base := 0.4 + r.Float64()*3.5
			if i == 0 {
				base = 1 + r.Float64()*3.2
			}
			_ = st.SeedUsage(u.ID, now.AddDate(0, 0, -d).Format("2006-01-02"), int64(base*float64(gb)))
		}
		if i%4 == 1 || i == 0 {
			pid := plan.ID
			id, err := st.CreatePayment(u.ID, &pid, plan.Price, []string{"", "۴ رقم آخر: ۵۶۷۸", "واریز از کارت همسرم"}[r.IntN(3)], receiptPNG(r), "image/png")
			if err == nil {
				_ = st.SeedPaymentTime(id, now.Add(-time.Duration(r.IntN(36*3600))*time.Second).Unix())
				if i == 0 {
					_ = st.DecidePayment(id, store.PayApproved, "", "admin")
				} else if i%8 == 5 {
					_ = st.DecidePayment(id, store.PayApproved, "", "admin")
				}
			}
		}
	}

	_ = st.AddCustomDomain(store.CustomDomain{Domain: "example-game.com", Action: "route", Note: "تست برای یک مشتری"})

	// A day of traffic and host health.
	for ts := now.Add(-24 * time.Hour).Unix(); ts <= now.Unix(); ts += 300 {
		hour := float64(time.Unix(ts, 0).Hour())
		curve := 0.35 + 0.65*(0.5+0.5*math.Sin((hour-8)/24*2*math.Pi))
		down := int64(curve * (1.2 + r.Float64()*0.6) * float64(gb) / 6)
		_ = st.SeedTraffic(ts, g.ID, down/9, down)
		_ = st.SeedTraffic(ts, g2.ID, down/30, down/3)
		for _, node := range []string{"hub", g.ID, g2.ID} {
			if ts%60 != 0 && node != "hub" {
				continue
			}
			_ = st.SeedMetrics(node, ts, sysmetrics.Sample{CPU: 8 + curve*40 + r.Float64()*8, MemUsed: uint64((1.1 + curve*0.8) * float64(gb)),
				MemTotal: uint64(4 * gb), DiskUsed: uint64(9 * gb), DiskTotal: uint64(40 * gb),
				RxBps: uint64(curve * 38e6 * (0.8 + r.Float64()*0.4)), TxBps: uint64(curve * 36e6 * (0.8 + r.Float64()*0.4)),
				Conns: int64(curve * 900), Uptime: 86400 * 31})
		}
	}
	status := func(q int64) string {
		b, _ := json.Marshal(store.GateStatus{
			Host:  sysmetrics.Sample{CPU: 27.5, MemUsed: uint64(16 * gb / 10), MemTotal: uint64(4 * gb), DiskUsed: uint64(9 * gb), DiskTotal: uint64(40 * gb), RxBps: 21e6, TxBps: 20e6, Uptime: 86400 * 31, Conns: 540},
			Stats: proto.GateStats{Queries: q, Routed: q / 3, Refused: q / 40, OpenConns: 540},
		})
		return string(b)
	}
	_ = st.SeedGateSeen(g2.ID, "185.55.226.14", now.Add(-40*time.Second).Unix(), status(2140), "v1.0.0")
	st.Log("admin", "login", "login", "127.0.0.1")
	st.Log("sara", "payment", "payment_sent", "1")
	st.Log("admin", "plan", "plan_saved", "گیمر")
	return seededGate{g.ID, secret}
}

// receiptPNG draws a small fake bank slip.
func receiptPNG(r *rand.Rand) []byte {
	w, h := 360, 520
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{250, 250, 252, 255})
		}
	}
	hue := uint8(r.IntN(120))
	for y := 0; y < 90; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{30 + hue/2, 60, 160 + hue/2, 255})
		}
	}
	for row := 0; row < 9; row++ {
		y0 := 130 + row*40
		lw := 120 + r.IntN(160)
		for y := y0; y < y0+10; y++ {
			for x := 30; x < 30+lw; x++ {
				img.Set(x, y, color.RGBA{200, 202, 214, 255})
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
