package gate

import (
	"net"
	"strconv"
	"time"

	"github.com/amor7/gozarDNS/internal/netx"
	"github.com/amor7/gozarDNS/internal/rules"
)

const (
	peekTimeout = 10 * time.Second
	dialTimeout = 10 * time.Second
	idleTimeout = 10 * time.Minute
)

// handleTLS reads the name a TLS client is asking for and either hands the
// connection to the portal or carries it, untouched, to the hub. The gate
// never terminates a customer's TLS session.
func (gt *Gate) handleTLS(c net.Conn) {
	name, pc, err := netx.PeekSNI(c, peekTimeout)
	if err != nil {
		c.Close()
		return
	}
	if gt.g.PortalDomain != "" && name == gt.g.PortalDomain {
		gt.portalL.Push(pc)
		return
	}
	gt.carry(pc, name, gt.g.HubHTTPSPort, 443)
}

// handleHTTP does the same for plain HTTP. Console game downloads travel
// this way: their CDNs serve packages over HTTP only.
func (gt *Gate) handleHTTP(c net.Conn) {
	host, pc, err := netx.PeekHost(c, peekTimeout)
	if err != nil {
		c.Close()
		return
	}
	if gt.g.PortalDomain != "" && host == gt.g.PortalDomain {
		_, _ = pc.Write([]byte("HTTP/1.1 301 Moved Permanently\r\nLocation: https://" + host + "/\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
		pc.Close()
		return
	}
	gt.carry(pc, host, gt.g.HubHTTPPort, 80)
}

// handleExtraTLS carries one of the alternative HTTPS ports to the same port.
func (gt *Gate) handleExtraTLS(port int) func(net.Conn) {
	return func(c net.Conn) {
		name, pc, err := netx.PeekSNI(c, peekTimeout)
		if err != nil {
			c.Close()
			return
		}
		gt.carry(pc, name, port, port)
	}
}

// carry takes a customer connection to the hub: to hubPort when connecting
// directly, or as a stream for the service port when tunnelling.
func (gt *Gate) carry(c net.Conn, name string, hubPort, service int) {
	ip := netx.RemoteAddr(c)
	cl, set, allowed := gt.state.who(ip)
	if !allowed {
		c.Close()
		return
	}
	// Only names this customer's plan routes. Anything else here means a
	// device with a stale DNS answer, or someone trying to use the gate as
	// an open proxy.
	if a, _ := set.Match(name); a != rules.Route {
		c.Close()
		return
	}
	var up net.Conn
	var err error
	if gt.state.useTunnel() {
		if up, err = gt.tun.dial(name, service); err != nil {
			gt.log.Debug("tunnel unavailable, going direct", "name", name, "err", err)
		}
	}
	if up == nil {
		up, err = net.DialTimeout("tcp4", net.JoinHostPort(gt.hubAddr, strconv.Itoa(hubPort)), dialTimeout)
		if err != nil {
			gt.log.Debug("hub unreachable", "err", err)
			c.Close()
			return
		}
		if tc, ok := up.(*net.TCPConn); ok {
			_ = tc.SetKeepAlive(true)
			_ = tc.SetKeepAlivePeriod(30 * time.Second)
		}
	}

	var cnt netx.Counter
	var uid int64
	if cl != nil {
		uid = cl.user
		gt.state.touch(ip)
	}
	m := gt.state.meter(uid)
	cnt.Up = func(n int) { m.up.Add(int64(n)) }
	cnt.Down = func(n int) { m.down.Add(int64(n)) }

	gt.openConns.Add(1)
	defer gt.openConns.Add(-1)
	netx.Pipe(c, up, cnt, gt.state.limiter(uid), idleTimeout)
}
