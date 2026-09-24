// Package ctl is the local control socket: how the `gozar` command talks to
// the running service on the same machine. It is a unix socket readable by
// root only, so it needs no authentication of its own.
package ctl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var SocketPath = "/run/gozar/gozar.sock"

// Listen serves h on the control socket.
func Listen(h http.Handler) (io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(SocketPath), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(SocketPath)
	l, err := net.Listen("unix", SocketPath)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(SocketPath, 0o600)
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(l)
	return srv, nil
}

// Client talks to the socket.
func Client() *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", SocketPath)
		},
	}}
}

// Get fetches path and decodes JSON into v.
func Get(path string, v any) error {
	resp, err := Client().Get("http://gozar" + path)
	if err != nil {
		return fmt.Errorf("the gozar service is not running (or you are not root): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("%s", b)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// Stream opens path and hands back the body for line-by-line reading.
func Stream(path string) (io.ReadCloser, error) {
	resp, err := Client().Get("http://gozar" + path)
	if err != nil {
		return nil, fmt.Errorf("the gozar service is not running (or you are not root): %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, fmt.Errorf("%s", b)
	}
	return resp.Body, nil
}
