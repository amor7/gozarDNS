package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/amor7/gozarDNS/internal/auth"
	"github.com/amor7/gozarDNS/internal/buildinfo"
	"github.com/amor7/gozarDNS/internal/certs"
	"github.com/amor7/gozarDNS/internal/config"
	"github.com/amor7/gozarDNS/internal/ctl"
	"github.com/amor7/gozarDNS/internal/dnsd"
	"github.com/amor7/gozarDNS/internal/gate"
	"github.com/amor7/gozarDNS/internal/hub"
	"github.com/amor7/gozarDNS/internal/store"
)

const usage = `Gozar - smart DNS gateway

Usage: gozar <command> [arguments]

Setting up
  setup                 interactive setup (hub or gate)
  update [--file F]     install the latest release (or a downloaded binary)
  uninstall             stop and remove Gozar from this machine

Everyday
  status                what this machine is doing right now
  logs [-f] [-e]        service logs (-f follow, -e warnings and errors only)
  restart               restart the service

On a gate
  watch [user|ip]       stream DNS lookups live - who asked for what, and what happened
  check <domain> [user] what the gate does with a name, for everyone or one customer

On the hub
  admin url             where the admin console is
  admin password [name] set a new admin password (default: admin)
  admin 2fa-off [name]  turn off two-step sign-in for an admin who lost their phone
  gate add <name>       register a new gate and print its join token
  gate list             list gates
  backup [file]         write a full database backup

  version               print the version
`

// Main runs the command line and returns the process exit code.
func Main(args []string) int {
	fs := flag.NewFlagSet("gozar", flag.ContinueOnError)
	cfgPath := fs.String("config", envOr("GOZAR_CONFIG", config.DefaultPath), "config file")
	fs.Usage = func() { fmt.Print(usage) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Print(usage)
		return 0
	}
	var err error
	switch cmd, a := rest[0], rest[1:]; cmd {
	case "run":
		err = cmdRun(*cfgPath)
	case "setup", "install":
		err = cmdSetup(*cfgPath)
	case "status":
		err = cmdStatus(*cfgPath)
	case "watch":
		err = cmdWatch(a)
	case "check":
		err = cmdCheck(a)
	case "logs":
		err = cmdLogs(a)
	case "restart":
		err = run("systemctl", "restart", "gozar")
		if err == nil {
			okf("restarted")
		}
	case "admin":
		err = cmdAdmin(*cfgPath, a)
	case "gate":
		err = cmdGate(*cfgPath, a)
	case "backup":
		err = cmdBackup(*cfgPath, a)
	case "update":
		err = cmdUpdate(a)
	case "uninstall":
		err = cmdUninstall(*cfgPath)
	case "version", "--version", "-v":
		fmt.Println("gozar", buildinfo.Version, runtime.GOOS+"/"+runtime.GOARCH)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		errf("unknown command %q - run `gozar help`", cmd)
		return 2
	}
	if err != nil {
		errf("%v", err)
		return 1
	}
	return 0
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---------------------------------------------------------------- run

func cmdRun(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	level := slog.LevelInfo
	if os.Getenv("GOZAR_DEBUG") == "1" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level, ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey { // journald stamps lines itself
			return slog.Attr{}
		}
		return a
	}}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Info("starting gozar", "version", buildinfo.Version, "role", cfg.Role)

	switch cfg.Role {
	case config.RoleHub:
		st, err := store.Open(filepath.Join(cfg.DataDir, "gozar.db"))
		if err != nil {
			return err
		}
		defer st.Close()
		h, err := hub.New(cfg, st, log)
		if err != nil {
			return err
		}
		err = h.Run(ctx)
		if errors.Is(err, hub.ErrRestart) {
			log.Info("restarting to load the restored database")
			return nil
		}
		return err
	case config.RoleGate:
		g, err := gate.New(cfg, log)
		if err != nil {
			return err
		}
		return g.Run(ctx)
	}
	return fmt.Errorf("unknown role %q", cfg.Role)
}

// ---------------------------------------------------------------- status

func cmdStatus(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if cfg.Role == config.RoleHub {
		var s hub.HubStatus
		if err := ctl.Get("/status", &s); err != nil {
			return err
		}
		lines := []string{
			"role        " + bold("hub") + dim("  "+s.Version),
			"console     " + cyan(s.AdminURL),
			fmt.Sprintf("customers   %d  (%d active, %d devices, %d online now)", s.Totals.Users, s.Totals.Active, s.Totals.Devices, s.Totals.OnlineNow),
			fmt.Sprintf("today       %s", humanBytes(s.Totals.TrafficToday)),
			fmt.Sprintf("receipts    %d waiting", s.Totals.Pending),
			fmt.Sprintf("connections %d open", s.OpenConns),
			fmt.Sprintf("google v6   %v", s.IPv6),
		}
		for _, g := range s.Gates {
			state := green("online")
			if !g.Online {
				state = red("offline")
			}
			ip := g.PublicIP
			if ip == "" {
				ip = dim("never connected")
			}
			lines = append(lines, fmt.Sprintf("gate        %s %s %s", bold(g.Name), ip, state))
		}
		box("Gozar hub", lines)
		return nil
	}
	var s gate.Status
	if err := ctl.Get("/status", &s); err != nil {
		return err
	}
	syncLine := green("ok") + dim(" "+ago(s.LastSync))
	if s.SyncError != "" {
		syncLine = red(s.SyncError)
	} else if s.LastSync == 0 {
		syncLine = yellow("not yet")
	}
	mode := green("registered addresses only")
	if !s.Enforce {
		mode = yellow("open to everyone") + dim(" (until the first customer registers)")
	}
	box("Gozar gate", []string{
		"role        " + bold("gate") + dim("  "+s.Version),
		"dns         " + accent(s.PublicIP),
		"portal      " + cyan(s.Portal),
		"hub         " + s.Hub + "  sync " + syncLine,
		"access      " + mode,
		fmt.Sprintf("customers   %d addresses, %d plans", s.Clients, s.Plans),
		fmt.Sprintf("connections %d open", s.OpenConns),
		"upstream    " + strings.Join(s.Upstreams, ", "),
		"tunnel      " + map[string]string{"": dim("off"), "up": green("up"), "down": red("down - going direct")}[s.Tunnel],
	})
	return nil
}

func ago(ts int64) string {
	if ts == 0 {
		return ""
	}
	d := time.Since(time.Unix(ts, 0)).Round(time.Second)
	return d.String() + " ago"
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ---------------------------------------------------------------- gate tools

func cmdWatch(a []string) error {
	path := "/watch"
	if len(a) > 0 {
		if strings.Count(a[0], ".") == 3 {
			path += "?ip=" + a[0]
		} else {
			path += "?user=" + a[0]
		}
	}
	body, err := ctl.Stream(path)
	if err != nil {
		return err
	}
	defer body.Close()
	fmt.Println(dim("   watching DNS lookups - open the app or game on the device now. Ctrl-C to stop."))
	fmt.Println(dim("   routed = through the hub · direct = straight out · filtered = blocked inside the country"))
	fmt.Println()
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e dnsd.Event
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		who := e.User
		if who == "" {
			who = e.IP.String()
		}
		v := e.Verdict
		switch v {
		case dnsd.VerdictRouted:
			v = green("routed  ")
		case dnsd.VerdictDirect:
			v = dim("direct  ")
		case dnsd.VerdictPinned:
			v = cyan("pinned  ")
		case dnsd.VerdictFiltered:
			v = red("filtered")
		case dnsd.VerdictRefused:
			v = yellow("refused ")
		default:
			v = yellow(fmt.Sprintf("%-8s", v))
		}
		fmt.Printf("%s  %-14s %s %-5s %s %s\n", dim(e.At.Format("15:04:05")), who, v, e.Type, strings.TrimSuffix(e.Name, "."), dim(e.Answer))
	}
	return sc.Err()
}

func cmdCheck(a []string) error {
	if len(a) == 0 {
		return errors.New("usage: gozar check <domain> [user]")
	}
	path := "/check?name=" + a[0]
	if len(a) > 1 {
		path += "&user=" + a[1]
	}
	var r gate.CheckResult
	if err := ctl.Get(path, &r); err != nil {
		return err
	}
	who := "an unregistered address"
	if r.User != "" {
		who = r.User
	}
	switch {
	case !r.Allowed:
		fmt.Printf("%s: %s is refused (not registered)\n", r.Name, who)
	case r.Action == "route":
		fmt.Printf("%s: %s - via %s\n", r.Name, green("routed through the hub"), r.Matched)
	case r.Action == "direct":
		fmt.Printf("%s: %s - rule on %s\n", r.Name, dim("resolved normally (exception)"), r.Matched)
	case r.Action == "pin":
		fmt.Printf("%s: %s - rule on %s\n", r.Name, cyan("resolved to reachable addresses only"), r.Matched)
	default:
		fmt.Printf("%s: %s for %s\n", r.Name, dim("not routed"), who)
	}
	return nil
}

func cmdLogs(a []string) error {
	args := []string{"-u", "gozar", "--no-pager", "-o", "cat", "-n", "200"}
	for _, x := range a {
		switch x {
		case "-f", "--follow":
			args = append(args, "-f")
		case "-e", "--errors":
			args = append(args, "-p", "warning")
		}
	}
	cmd := exec.Command("journalctl", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

// ---------------------------------------------------------------- hub tools

func openHubStore(cfgPath string) (*config.Config, *store.Store, error) {
	if err := requireRoot(); err != nil {
		return nil, nil, err
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, err
	}
	if cfg.Role != config.RoleHub {
		return nil, nil, errors.New("this command runs on the hub")
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "gozar.db"))
	return cfg, st, err
}

func cmdAdmin(cfgPath string, a []string) error {
	if len(a) == 0 {
		a = []string{"url"}
	}
	cfg, st, err := openHubStore(cfgPath)
	if err != nil {
		return err
	}
	defer st.Close()
	name := "admin"
	if len(a) > 1 {
		name = a[1]
	}
	switch a[0] {
	case "url":
		h := cfg.Hub
		fmt.Printf("https://%s:%d\n", h.PublicIP, h.AdminPort)
		if h.AdminDomain != "" {
			fmt.Printf("https://%s\n", h.AdminDomain)
		}
	case "password":
		ad, err := st.AdminByName(name)
		if errors.Is(err, store.ErrNotFound) {
			pw := auth.Password(14)
			if _, err := st.CreateAdmin(name, auth.HashPassword(pw)); err != nil {
				return err
			}
			box("New admin", []string{"username  " + bold(name), "password  " + bold(pw)})
			return nil
		} else if err != nil {
			return err
		}
		pw := auth.Password(14)
		if err := st.SetAdminPassword(ad.ID, auth.HashPassword(pw)); err != nil {
			return err
		}
		st.Log("cli", "security", "admin_password_reset", name)
		box("Password changed", []string{"username  " + bold(name), "password  " + bold(pw), dim("every session of this admin was signed out")})
	case "2fa-off":
		ad, err := st.AdminByName(name)
		if err != nil {
			return fmt.Errorf("no admin called %s", name)
		}
		if err := st.SetAdminTOTP(ad.ID, "", false); err != nil {
			return err
		}
		st.Log("cli", "security", "admin_2fa_reset", name)
		okf("two-step sign-in is off for %s", name)
	default:
		return fmt.Errorf("unknown admin command %q", a[0])
	}
	return nil
}

func cmdGate(cfgPath string, a []string) error {
	if len(a) == 0 {
		a = []string{"list"}
	}
	cfg, st, err := openHubStore(cfgPath)
	if err != nil {
		return err
	}
	defer st.Close()
	switch a[0] {
	case "list":
		gates, err := st.Gates()
		if err != nil {
			return err
		}
		for _, g := range gates {
			state := green("online")
			if !g.Online {
				state = red("offline")
			}
			fmt.Printf("%-10s %-16s %-16s %s  %s\n", g.ID, g.Name, g.PublicIP, state, dim("enforce="+g.Enforce))
		}
	case "add":
		name := "gate"
		if len(a) > 1 {
			name = a[1]
		}
		g, secret, err := st.CreateGate(name)
		if err != nil {
			return err
		}
		ctrl, err := certs.SelfSigned(filepath.Join(cfg.DataDir, "tls"), "control", []string{cfg.Hub.PublicIP})
		if err != nil {
			return err
		}
		h := cfg.Hub
		tok := hub.JoinToken(&hub.HubInfo{PublicIP: h.PublicIP, ControlPort: h.ControlPort, HTTPSPort: h.HTTPSPort,
			HTTPPort: h.HTTPPort, Fingerprint: certs.Fingerprint(ctrl), SNI: h.AdminDomain}, g.ID, secret)
		st.Log("cli", "gate", "gate_added", name)
		fmt.Println()
		fmt.Println("   " + bold("Join token for "+name) + dim(" - run `gozar setup` on the gate and paste it:"))
		fmt.Println()
		fmt.Println("   " + accent(tok))
		fmt.Println()
	default:
		return fmt.Errorf("unknown gate command %q", a[0])
	}
	return nil
}

func cmdBackup(cfgPath string, a []string) error {
	cfg, st, err := openHubStore(cfgPath)
	if err != nil {
		return err
	}
	defer st.Close()
	dst := filepath.Join(cfg.DataDir, "backups", "gozar-"+time.Now().Format("2006-01-02-150405")+".db")
	if len(a) > 0 {
		dst = a[0]
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	if err := st.Backup(context.Background(), dst); err != nil {
		return err
	}
	_ = os.Chmod(dst, 0o600)
	okf("backup written to %s", dst)
	return nil
}

// ---------------------------------------------------------------- update

func cmdUpdate(a []string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	var src string
	for i := 0; i < len(a); i++ {
		if a[i] == "--file" && i+1 < len(a) {
			src = a[i+1]
		}
	}
	if src == "" {
		asset := "gozar-linux-" + runtime.GOARCH
		base := "https://github.com/" + buildinfo.Repo + "/releases/latest/download/"
		info("downloading %s ...", asset)
		// Next to the installed binary rather than in /tmp, which is often
		// mounted noexec - the new file has to run once before it is trusted.
		tmp, err := os.CreateTemp(filepath.Dir(binPath), ".gozar-update-*")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		sum, err := download(base+asset, tmp)
		tmp.Close()
		if err != nil {
			return err
		}
		want, err := fetchChecksum(base+"checksums.txt", asset)
		if err != nil {
			return err
		}
		if sum != want {
			return errors.New("checksum mismatch - the download is damaged or was tampered with; nothing was changed")
		}
		okf("checksum verified")
		src = tmp.Name()
	}
	if err := os.Chmod(src, 0o755); err != nil {
		return err
	}
	out, err := exec.Command(src, "version").Output()
	if err != nil || !strings.HasPrefix(string(out), "gozar ") {
		return errors.New("that file is not a working gozar binary")
	}
	info("installing %s", strings.TrimSpace(string(out)))
	if err := copyFile(src, binPath, 0o755); err != nil {
		return err
	}
	if hasSystemd() {
		if err := run("systemctl", "restart", "gozar"); err != nil {
			return err
		}
		if err := waitHealthy(); err != nil {
			return err
		}
	}
	okf("updated. Accounts, settings and certificates are unchanged.")
	return nil
}

func download(url string, w io.Writer) (string, error) {
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), resp.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fetchChecksum(url, asset string) (string, error) {
	var b strings.Builder
	if _, err := download(url, &b); err != nil {
		return "", err
	}
	for _, line := range strings.Split(b.String(), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			return f[0], nil
		}
	}
	return "", errors.New("the release has no checksum for " + asset)
}

// ---------------------------------------------------------------- uninstall

func cmdUninstall(cfgPath string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	cfg, _ := config.Load(cfgPath)
	fmt.Println()
	warnf("This stops Gozar and removes the service and the binary.")
	if !confirm("Continue?", false) {
		return nil
	}
	if hasSystemd() {
		_ = run("systemctl", "disable", "--now", "gozar")
		_ = os.Remove(unitPath)
		_ = run("systemctl", "daemon-reload")
	}
	if _, err := os.Stat("/etc/systemd/resolved.conf.d/gozar.conf"); err == nil {
		_ = os.Remove("/etc/systemd/resolved.conf.d/gozar.conf")
		_ = run("systemctl", "restart", "systemd-resolved")
	}
	dataDir := "/var/lib/gozar"
	if cfg != nil {
		dataDir = cfg.DataDir
	}
	if confirm("Also delete all data - accounts, usage, certificates, config?", false) {
		_ = os.RemoveAll(dataDir)
		_ = os.RemoveAll(filepath.Dir(cfgPath))
		okf("data deleted")
	} else {
		info("data kept in %s and %s", dataDir, filepath.Dir(cfgPath))
	}
	_ = os.Remove(binPath)
	okf("Gozar is uninstalled")
	return nil
}
