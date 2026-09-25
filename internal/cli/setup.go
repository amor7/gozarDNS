package cli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/amor7/gozarDNS/internal/auth"
	"github.com/amor7/gozarDNS/internal/catalog"
	"github.com/amor7/gozarDNS/internal/certs"
	"github.com/amor7/gozarDNS/internal/config"
	"github.com/amor7/gozarDNS/internal/hub"
	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/store"
)

func cmdSetup(cfgPath string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	banner()
	if old, err := config.Load(cfgPath); err == nil {
		info("This machine is already set up as a %s.", bold(string(old.Role)))
		if !confirm("Set it up again? Accounts and data are kept.", false) {
			return nil
		}
	}
	role := choose("What is this machine?", []string{
		bold("Hub") + dim("  - outside the filtered network. Holds accounts and the admin console. Set this up first."),
		bold("Gate") + dim(" - inside the country. Customers point their DNS here. Needs a join token from the hub."),
	}, 0)
	if role == 0 {
		return setupHub(cfgPath)
	}
	return setupGate(cfgPath)
}

// ---------------------------------------------------------------- hub

func setupHub(cfgPath string) error {
	const total = 4
	cfg := &config.Config{Role: config.RoleHub, DataDir: "/var/lib/gozar", Hub: &config.Hub{}}
	if old, err := config.Load(cfgPath); err == nil && old.Hub != nil {
		cfg.Hub = old.Hub
		cfg.DataDir = old.DataDir
	}
	h := cfg.Hub

	step(1, total, "Network")
	if h.PublicIP == "" {
		info(dim("finding this machine's public address..."))
		h.PublicIP = publicIPv4()
	}
	for {
		h.PublicIP = ask("Public IPv4 address of this machine:", h.PublicIP)
		if ip := net.ParseIP(h.PublicIP); ip != nil && ip.To4() != nil {
			break
		}
		warnf("that is not an IPv4 address")
	}
	for _, p := range []int{443, 80} {
		if !portFree("tcp", ":"+strconv.Itoa(p)) {
			warnf("port %d is in use by another program. Gozar needs it - stop that program (nginx? apache?) and run setup again.", p)
			if !confirm("Continue anyway?", false) {
				return fmt.Errorf("port %d is busy", p)
			}
		}
	}

	step(2, total, "Admin console")
	info("A domain name gives the console a real certificate and a clean address.")
	info("Point an A record at %s first. Leave empty to use the IP address.", bold(h.PublicIP))
	h.AdminDomain = strings.ToLower(strings.TrimSpace(ask("Admin domain (optional):", h.AdminDomain)))
	if h.AdminDomain != "" {
		h.ACMEEmail = ask("Email for Let's Encrypt notices (optional):", h.ACMEEmail)
	}
	if h.AdminPort == 0 {
		h.AdminPort = randomPort()
	}
	for {
		v := ask("Admin console port:", strconv.Itoa(h.AdminPort))
		p, err := strconv.Atoi(v)
		if err == nil && p > 1024 && p < 65536 && p != 7443 && p != 8443 {
			h.AdminPort = p
			break
		}
		warnf("choose a free port above 1024 (not 7443 or 8443)")
	}

	step(3, total, "Saving")
	cfg.Hub.IPv6Egress = "auto"
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	h = cfg.Hub
	okf("config written to %s", cfgPath)

	st, err := store.Open(filepath.Join(cfg.DataDir, "gozar.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	var adminUser, adminPass string
	if admins, _ := st.Admins(); len(admins) == 0 {
		adminUser = "admin"
		adminPass = auth.Password(14)
		if _, err := st.CreateAdmin(adminUser, auth.HashPassword(adminPass)); err != nil {
			return err
		}
		okf("admin account created")
	}
	if plans, _ := st.Plans(); len(plans) == 0 {
		if err := seedPlans(st); err != nil {
			return err
		}
		okf("two starter plans created - edit them in the console")
	}
	token := ""
	if gates, _ := st.Gates(); len(gates) == 0 {
		g, secret, err := st.CreateGate("gate-1")
		if err != nil {
			return err
		}
		ctrl, err := certs.SelfSigned(filepath.Join(cfg.DataDir, "tls"), "control", []string{h.PublicIP})
		if err != nil {
			return err
		}
		token = hub.JoinToken(&hub.HubInfo{PublicIP: h.PublicIP, ControlPort: h.ControlPort, HTTPSPort: h.HTTPSPort,
			HTTPPort: h.HTTPPort, Fingerprint: certs.Fingerprint(ctrl), SNI: h.AdminDomain}, g.ID, secret)
	}
	st.Log("setup", "setup", "hub_configured")

	step(4, total, "Starting")
	if err := installService("hub"); err != nil {
		return err
	}
	if err := waitHealthy(); err != nil {
		return err
	}
	okf("gozar is running")

	url := "https://" + net.JoinHostPort(h.PublicIP, strconv.Itoa(h.AdminPort))
	lines := []string{bold("Admin console"), "  " + cyan(url)}
	if h.AdminDomain != "" {
		lines = append(lines, "  "+cyan("https://"+h.AdminDomain)+dim("  (once DNS points here)"))
	}
	if adminPass != "" {
		lines = append(lines, "", "  username  "+bold(adminUser), "  password  "+bold(adminPass), dim("  shown once - store it in a password manager"))
	}
	if h.AdminDomain == "" {
		lines = append(lines, "", yellow("  Your browser will warn about the certificate: it is self-signed."), dim("  That is expected without a domain. Continue past the warning."))
	}
	lines = append(lines, "", bold("Open these ports")+dim(" in your provider's firewall:"),
		fmt.Sprintf("  tcp 80, 443, 2053, 2083, 2087, 2096, %d (gates), %d (console)", h.ControlPort, h.AdminPort))
	box("Hub is ready", lines)
	if token != "" {
		fmt.Println()
		fmt.Println("   " + bold("Join token for your first gate") + dim(" - paste it when you run setup on the gate:"))
		fmt.Println()
		fmt.Println("   " + accent(token))
		fmt.Println()
		fmt.Println(dim("   More gates: Admin console → Gates → Add gate, or `gozar gate add <name>`."))
	}
	fmt.Println()
	return nil
}

func randomPort() int {
	for i := 0; i < 50; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(40000))
		p := 20000 + int(n.Int64())
		if portFree("tcp", ":"+strconv.Itoa(p)) {
			return p
		}
	}
	return 2053
}

// seedPlans creates two starting points: everything light, and everything
// including the multi-gigabyte game downloads.
func seedPlans(st *store.Store) error {
	cat := catalog.Builtin()
	var light, all []string
	for _, s := range cat.Services {
		for _, g := range s.Groups {
			k := catalog.GroupKey(s.ID, g.ID)
			all = append(all, k)
			if !g.Heavy {
				light = append(light, k)
			}
		}
	}
	all = append(all, hub.CustomGroup)
	light = append(light, hub.CustomGroup)
	const gb = int64(1) << 30
	if err := st.SavePlan(&store.Plan{Name: "استاندارد", Description: "همهٔ سرویس‌ها به‌جز دانلود بازی کنسول",
		Price: 150000, QuotaBytes: 30 * gb, DurationDays: 30, MaxDevices: 2, Groups: light, Public: true, IsDefault: true, Color: "#7C5CFF", Sort: 1}); err != nil {
		return err
	}
	return st.SavePlan(&store.Plan{Name: "گیمر", Description: "همهٔ سرویس‌ها، همراه با دانلود بازی",
		Price: 350000, QuotaBytes: 150 * gb, DurationDays: 30, MaxDevices: 3, Groups: all, Public: true, Color: "#16C8A0", Sort: 2})
}

// ---------------------------------------------------------------- gate

func setupGate(cfgPath string) error {
	const total = 5
	cfg := &config.Config{Role: config.RoleGate, DataDir: "/var/lib/gozar", Gate: &config.Gate{}}
	if old, err := config.Load(cfgPath); err == nil && old.Gate != nil {
		cfg.Gate = old.Gate
		cfg.DataDir = old.DataDir
	}
	g := cfg.Gate

	step(1, total, "Pair with the hub")
	info("Paste the join token the hub printed (Admin console → Gates also shows one).")
	for {
		v := ask("Join token:", "")
		if v == "" && g.GateID != "" {
			info(dim("keeping the current pairing"))
			break
		}
		t, err := proto.ParseJoinToken(v)
		if err != nil {
			warnf("%v", err)
			continue
		}
		g.HubAddr, g.HubControlPort, g.Fingerprint, g.GateID, g.Secret = t.Hub, t.ControlPort, t.Fingerprint, t.GateID, t.Secret
		g.HubHTTPSPort, g.HubHTTPPort = t.HTTPSPort, t.HTTPPort
		g.HubSNI = t.SNI
		break
	}
	info(dim("contacting the hub..."))
	name, err := helloHub(g)
	if err != nil {
		warnf("could not reach the hub at %s:%d - %v", g.HubAddr, g.HubControlPort, err)
		info("Check that port %d is open on the hub and that this machine can reach it.", g.HubControlPort)
		if !confirm("Save the settings anyway?", false) {
			return err
		}
	} else {
		okf("connected to the hub as %s", bold(name))
	}

	step(2, total, "Network")
	if g.PublicIP == "" {
		info(dim("finding this machine's public address..."))
		g.PublicIP = publicIPv4()
	}
	for {
		g.PublicIP = ask("Public IPv4 address customers will use as DNS:", g.PublicIP)
		if ip := net.ParseIP(g.PublicIP); ip != nil && ip.To4() != nil {
			break
		}
		warnf("that is not an IPv4 address")
	}
	if resolvedHoldsPort53() {
		warnf("systemd-resolved is holding port 53, which the gate needs.")
		if confirm("Free it? (this machine keeps resolving names normally)", true) {
			if err := freeResolvedStub(); err != nil {
				return fmt.Errorf("freeing port 53: %w", err)
			}
			okf("port 53 is free")
		}
	}
	for _, p := range []int{443, 80} {
		if !portFree("tcp", ":"+strconv.Itoa(p)) {
			warnf("port %d is used by another program. Gozar needs it; stop that program first.", p)
		}
	}

	step(3, total, "Customer portal")
	info("Customers sign up, register devices and see their usage here.")
	info("With a domain it gets a real certificate. Point an A record at %s first.", bold(g.PublicIP))
	g.PortalDomain = strings.ToLower(strings.TrimSpace(ask("Portal domain (optional):", g.PortalDomain)))
	if g.PortalDomain != "" {
		g.ACMEEmail = ask("Email for Let's Encrypt notices (optional):", g.ACMEEmail)
	}

	step(4, total, "Saving")
	if g.STUNListen == "" {
		g.STUNListen = ":3478"
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	if err := cfg.Save(cfgPath); err != nil {
		return err
	}
	okf("config written to %s", cfgPath)

	step(5, total, "Starting")
	if err := installService("gate"); err != nil {
		return err
	}
	if err := waitHealthy(); err != nil {
		return err
	}
	okf("gozar is running")

	portal := "https://" + net.JoinHostPort(g.PublicIP, "8443")
	if g.PortalDomain != "" {
		portal = "https://" + g.PortalDomain
	}
	box("Gate is ready", []string{
		bold("DNS address for customers"),
		"  " + accent(g.PublicIP),
		"",
		bold("Customer portal"),
		"  " + cyan(portal),
		"",
		bold("Open these ports") + dim(" in your provider's firewall:"),
		"  udp+tcp 53, tcp 80, 443, 8443, 2053, 2083, 2087, 2096, udp 3478",
		"",
		dim("The gate serves everyone until the first customer registers,"),
		dim("then only registered addresses. Change that per gate in the console."),
	})
	fmt.Println()
	return nil
}

func helloHub(g *config.Gate) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: certs.PinnedConfig(g.Fingerprint, sniOr(g.HubSNI))}}
	port := g.HubControlPort
	if port == 0 {
		port = 7443
	}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "https://"+net.JoinHostPort(g.HubAddr, strconv.Itoa(port))+proto.PathHello, nil)
	req.Header.Set("Authorization", "Bearer "+g.GateID+"."+g.Secret)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return "", fmt.Errorf("the hub rejected this token - make a new one in the console")
	}
	var out struct{ Name string }
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Name, nil
}

func sniOr(s string) string {
	if s == "" {
		return config.DefaultHubSNI
	}
	return s
}
