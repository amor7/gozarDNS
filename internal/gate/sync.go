package gate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/amor7/gozarDNS/internal/auth"
	"github.com/amor7/gozarDNS/internal/buildinfo"
	"github.com/amor7/gozarDNS/internal/proto"
)

// pending is a batch sent but not yet acknowledged. It is resent unchanged
// until the hub confirms it, so a reply lost in transit can neither drop
// usage nor count it twice.
type pending struct {
	batch string
	usage map[int64]proto.Usage
	seen  map[string]int64
}

func (gt *Gate) syncLoop(ctx context.Context) {
	interval := 30 * time.Second
	var p *pending
	for {
		if p == nil {
			usage, seen := gt.state.drain()
			p = &pending{batch: auth.Token(12), usage: usage, seen: seen}
		}
		next, err := gt.syncOnce(ctx, p)
		if err != nil {
			gt.syncErr.Store(err.Error())
			gt.log.Warn("sync with hub failed", "err", err)
			interval = 15 * time.Second
		} else {
			gt.syncErr.Store("")
			gt.lastSync.Store(time.Now().Unix())
			p = nil
			if next > 0 {
				interval = time.Duration(next) * time.Second
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		case <-gt.kick:
		}
	}
}

func (gt *Gate) syncOnce(ctx context.Context, p *pending) (int, error) {
	rev, _, _, _ := gt.state.info()
	host := gt.sampler.Take()
	host.Conns = gt.openConns.Load()
	req := proto.SyncRequest{
		Batch:    p.batch,
		Usage:    p.usage,
		Seen:     p.seen,
		Host:     host,
		Version:  buildinfo.Version,
		PublicIP: gt.selfIP.String(),
		Have:     rev,
		Stats: proto.GateStats{
			Queries:   gt.dns.Queries.Swap(0),
			Routed:    gt.dns.Routed.Swap(0),
			Refused:   gt.dns.Refused.Swap(0),
			OpenConns: gt.openConns.Load(),
		},
	}
	body, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	hr, _ := http.NewRequestWithContext(ctx, http.MethodPost, gt.hubURL(proto.PathSync), bytes.NewReader(body))
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Authorization", "Bearer "+gt.g.GateID+"."+gt.g.Secret)
	resp, err := gt.hubHTTP.Do(hr)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return 0, fmt.Errorf("the hub at %s did not answer in time - check that this gate can reach it on port %d (%v)", gt.hubAddr, gt.g.HubControlPort, err)
		}
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if resp.StatusCode == http.StatusUnauthorized {
			return 0, fmt.Errorf("the hub no longer recognises this gate - it may have been removed; pair it again with `gozar setup`")
		}
		return 0, fmt.Errorf("hub answered %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out proto.SyncResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&out); err != nil {
		return 0, fmt.Errorf("reading the hub's answer: %w", err)
	}
	if out.Config != nil && out.Revision != rev {
		gt.state.apply(out.Revision, out.Config)
		if err := saveCache(gt.cachePath(), out.Revision, out.Config); err != nil {
			gt.log.Warn("could not cache config", "err", err)
		}
		_, enforce, clients, plans := gt.state.info()
		gt.log.Info("new config from hub", "revision", out.Revision, "clients", clients, "plans", plans, "enforce", enforce)
	}
	return out.Interval, nil
}

// systemResolvers reads the machine's own upstream resolvers, skipping the
// local stub that systemd-resolved puts in /etc/resolv.conf.
func systemResolvers() []string {
	var out []string
	for _, path := range []string{"/run/systemd/resolve/resolv.conf", "/etc/resolv.conf"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) >= 2 && fields[0] == "nameserver" {
				if a, err := netip.ParseAddr(fields[1]); err == nil && !a.IsLoopback() && a.Is4() {
					out = append(out, a.String())
				}
			}
		}
		f.Close()
		if len(out) > 0 {
			return out
		}
	}
	return []string{"1.1.1.1", "8.8.8.8"}
}

// SyncNow asks for a sync without waiting for the timer.
func (gt *Gate) SyncNow() {
	select {
	case gt.kick <- struct{}{}:
	default:
	}
}
