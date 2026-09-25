package hub

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/rules"
)

// tunnel carries one customer connection inside the gate's HTTP/2
// connection: the request body is what the customer sends, the response
// body what comes back. Many of these share one TLS connection on port 443,
// so from outside the gate is one HTTPS client talking to one server - no
// customer's destination name crosses the border in the clear.
func (s *Hub) tunnel(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor != 2 {
		http.Error(w, "HTTP/2 only", http.StatusBadRequest)
		return
	}
	host, port, err := net.SplitHostPort(r.Header.Get(proto.HeaderTarget))
	n, _ := strconv.Atoi(port)
	if err != nil || !rules.ValidDomain(host) || !proto.ServicePort(n) {
		http.Error(w, "bad target", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	var up net.Conn
	if port != "80" {
		up, err = s.dialOut(ctx, host, port)
	} else {
		// Plain HTTP goes through the hub's own port-80 handler, which
		// knows which hosts may be forwarded and which only redirected.
		var d net.Dialer
		up, err = d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.h.HTTPPort)))
	}
	cancel()
	if err != nil {
		http.Error(w, "upstream unreachable", http.StatusBadGateway)
		return
	}
	defer up.Close()

	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{}) // a stream lives as long as the connection it carries
	_ = rc.SetWriteDeadline(time.Time{})
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return
	}
	s.openConns.Add(1)
	s.tunnelStreams.Add(1)
	defer s.openConns.Add(-1)

	go func() {
		_, _ = io.Copy(up, r.Body)
		if cw, ok := up.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
	}()
	buf := make([]byte, 32*1024)
	for {
		_ = up.SetReadDeadline(time.Now().Add(10 * time.Minute))
		n, err := up.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if rc.Flush() != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
