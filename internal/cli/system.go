package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	binPath  = "/usr/local/bin/gozar"
	unitPath = "/etc/systemd/system/gozar.service"
)

const unitFile = `[Unit]
Description=Gozar %s
Documentation=https://github.com/amor7/gozar
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/gozar run
Restart=always
RestartSec=3
LimitNOFILE=1048576
RuntimeDirectory=gozar
StateDirectory=gozar
NoNewPrivileges=yes
ProtectSystem=full
ProtectHome=yes
PrivateTmp=yes
ProtectKernelTunables=yes
ProtectControlGroups=yes

[Install]
WantedBy=multi-user.target
`

func hasSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// installService puts this binary in place and starts it under systemd.
func installService(role string) error {
	self, err := os.Executable()
	if err == nil && self != binPath {
		if err := copyFile(self, binPath, 0o755); err != nil {
			return fmt.Errorf("copying binary to %s: %w", binPath, err)
		}
	}
	if !hasSystemd() {
		warnf("systemd was not found; start Gozar yourself with: %s", bold(binPath+" run"))
		return nil
	}
	if err := os.WriteFile(unitPath, []byte(fmt.Sprintf(unitFile, role)), 0o644); err != nil {
		return err
	}
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "enable", "gozar"); err != nil {
		return err
	}
	return run("systemctl", "restart", "gozar")
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// waitHealthy waits for the service to stay up for a few seconds.
func waitHealthy() error {
	if !hasSystemd() {
		return nil
	}
	time.Sleep(3 * time.Second)
	if err := run("systemctl", "is-active", "--quiet", "gozar"); err != nil {
		out, _ := exec.Command("journalctl", "-u", "gozar", "-n", "15", "--no-pager", "-o", "cat").Output()
		return fmt.Errorf("the service did not stay up. Last log lines:\n%s", out)
	}
	return nil
}

// publicIPv4 asks a few well-known echo services, then falls back to the
// first public address on an interface.
func publicIPv4() string {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "tcp4", addr)
			},
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	for _, u := range []string{"https://api.ipify.org", "https://ipv4.icanhazip.com", "https://ifconfig.me/ip", "https://checkip.amazonaws.com"} {
		resp, err := client.Get(u)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if a, err := netip.ParseAddr(strings.TrimSpace(string(b))); err == nil && a.Is4() {
			return a.String()
		}
	}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			ip := p.Addr()
			if ip.Is4() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				return ip.String()
			}
		}
	}
	return ""
}

func portFree(network, addr string) bool {
	switch network {
	case "udp":
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return false
		}
		pc.Close()
	default:
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return false
		}
		l.Close()
	}
	return true
}

// freeResolvedStub stops systemd-resolved's stub listener from holding port
// 53, keeping the machine's own resolving working through its upstreams.
func freeResolvedStub() error {
	if err := os.MkdirAll("/etc/systemd/resolved.conf.d", 0o755); err != nil {
		return err
	}
	conf := "# Written by gozar setup: the gate answers DNS on port 53 itself.\n[Resolve]\nDNSStubListener=no\n"
	if err := os.WriteFile("/etc/systemd/resolved.conf.d/gozar.conf", []byte(conf), 0o644); err != nil {
		return err
	}
	if target, err := os.Readlink("/etc/resolv.conf"); err == nil && strings.Contains(target, "stub-resolv.conf") {
		_ = os.Remove("/etc/resolv.conf")
		if err := os.Symlink("/run/systemd/resolve/resolv.conf", "/etc/resolv.conf"); err != nil {
			return err
		}
	}
	return run("systemctl", "restart", "systemd-resolved")
}

func resolvedHoldsPort53() bool {
	b, err := exec.Command("ss", "-lnpu", "sport = :53").Output()
	return err == nil && strings.Contains(string(b), "systemd-resolve")
}

func requireRoot() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if os.Geteuid() != 0 {
		return errors.New("run this as root (sudo gozar ...)")
	}
	return nil
}
