package gate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"

	"github.com/amor7/gozarDNS/internal/certs"
	"github.com/amor7/gozarDNS/internal/proto"
)

// tunnel carries customer connections to the hub as streams inside one
// HTTP/2 connection on the hub's port 443. From outside it is a single
// HTTPS session to one server; the names customers are reaching stay inside
// it. When it cannot be opened, connections go direct instead and the tunnel
// is given a rest before it is tried again.
type tunnel struct {
	gt        *Gate
	client    *http.Client
	url       string
	restUntil atomic.Int64 // unix time before which the tunnel is not tried
	healthy   atomic.Bool
}

var errTunnelResting = errors.New("tunnel resting after a failure")

func newTunnel(gt *Gate) *tunnel {
	tr := &http2.Transport{
		TLSClientConfig: certs.PinnedConfig(gt.g.Fingerprint, gt.g.HubSNI),
		ReadIdleTimeout: 30 * time.Second, // ping when quiet, to notice a dead path
		PingTimeout:     15 * time.Second,
	}
	return &tunnel{
		gt:     gt,
		client: &http.Client{Transport: tr},
		url:    "https://" + net.JoinHostPort(gt.hubAddr, strconv.Itoa(gt.g.HubHTTPSPort)) + "/v1/tunnel",
	}
}

// status is what the gate reports to the hub: "up", "down", or "" when the
// tunnel is not in use.
func (t *tunnel) status(on bool) string {
	switch {
	case !on:
		return ""
	case t.healthy.Load():
		return "up"
	default:
		return "down"
	}
}

func (t *tunnel) dial(name string, port int) (net.Conn, error) {
	if time.Now().Unix() < t.restUntil.Load() {
		return nil, errTunnelResting
	}
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, t.url, pr)
	req.Header.Set("Authorization", "Bearer "+t.gt.g.GateID+"."+t.gt.g.Secret)
	req.Header.Set(proto.HeaderTarget, net.JoinHostPort(name, strconv.Itoa(port)))
	req.ContentLength = -1

	// Headers must come back quickly; the stream itself may then live long.
	timer := time.AfterFunc(dialTimeout, cancel)
	resp, err := t.client.Do(req)
	stopped := timer.Stop()
	if err != nil || !stopped {
		cancel()
		pw.Close()
		if err == nil {
			err = errors.New("tunnel: no answer in time")
		}
		t.fail(err)
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		pw.Close()
		if resp.StatusCode == http.StatusBadGateway {
			// The hub is fine; that one destination is not.
			return nil, fmt.Errorf("tunnel: hub cannot reach %s", name)
		}
		err := fmt.Errorf("tunnel: hub answered %s", resp.Status)
		t.fail(err)
		return nil, err
	}
	t.healthy.Store(true)
	return &streamConn{body: resp.Body, pw: pw, cancel: cancel}, nil
}

// check asks the hub a trivial question over the tunnel's connection, so
// its state is known before any customer needs it, and the connection stays
// warm. Called with every sync while the tunnel is on.
func (t *tunnel) check(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(t.url, "/v1/tunnel")+proto.PathHello, nil)
	req.Header.Set("Authorization", "Bearer "+t.gt.g.GateID+"."+t.gt.g.Secret)
	resp, err := t.client.Do(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("hub answered %s", resp.Status)
		}
	}
	if err != nil {
		t.fail(err)
		return
	}
	if !t.healthy.Swap(true) {
		t.gt.log.Info("tunnel up")
	}
	t.restUntil.Store(0)
}

func (t *tunnel) fail(err error) {
	if t.healthy.Swap(false) {
		t.gt.log.Warn("tunnel down, going direct for a while", "err", err)
	}
	t.restUntil.Store(time.Now().Add(30 * time.Second).Unix())
}

// streamConn is one tunnel stream seen as a net.Conn.
type streamConn struct {
	body   io.ReadCloser
	pw     *io.PipeWriter
	cancel context.CancelFunc
	closed atomic.Bool
}

func (c *streamConn) Read(p []byte) (int, error)  { return c.body.Read(p) }
func (c *streamConn) Write(p []byte) (int, error) { return c.pw.Write(p) }
func (c *streamConn) CloseWrite() error           { return c.pw.Close() }

func (c *streamConn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	c.pw.Close()
	c.body.Close()
	c.cancel()
	return nil
}

func (c *streamConn) LocalAddr() net.Addr  { return &net.TCPAddr{} }
func (c *streamConn) RemoteAddr() net.Addr { return &net.TCPAddr{} }

// Deadlines are kept by the customer's side of the copy and by HTTP/2
// pings; a stream has none of its own.
func (c *streamConn) SetDeadline(time.Time) error      { return nil }
func (c *streamConn) SetReadDeadline(time.Time) error  { return nil }
func (c *streamConn) SetWriteDeadline(time.Time) error { return nil }
