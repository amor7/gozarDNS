package netx

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var bufPool = sync.Pool{New: func() any { b := make([]byte, 32*1024); return &b }}

// Counter receives byte counts as they move. Either field may be nil.
type Counter struct {
	Up   func(n int) // client -> upstream
	Down func(n int) // upstream -> client
}

// Pipe copies both ways until both directions are done or nothing moves for
// idle. limit, when non-nil, paces what is sent to the client: a queue rather
// than a packet-dropper, so a capped download runs smoothly a little slower.
func Pipe(client, upstream net.Conn, cnt Counter, limit *rate.Limiter, idle time.Duration) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		copyHalf(upstream, client, cnt.Up, nil, idle)
	}()
	go func() {
		defer wg.Done()
		copyHalf(client, upstream, cnt.Down, limit, idle)
	}()
	wg.Wait()
	client.Close()
	upstream.Close()
}

func copyHalf(dst, src net.Conn, count func(int), limit *rate.Limiter, idle time.Duration) {
	bp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bp)
	buf := *bp
	if limit != nil && limit.Burst() < len(buf) {
		buf = buf[:max(limit.Burst(), 1024)]
	}
	for {
		_ = src.SetReadDeadline(time.Now().Add(idle))
		n, err := src.Read(buf)
		if n > 0 {
			if limit != nil {
				if werr := limit.WaitN(context.Background(), n); werr != nil {
					break
				}
			}
			_ = dst.SetWriteDeadline(time.Now().Add(idle))
			if _, werr := dst.Write(buf[:n]); werr != nil {
				break
			}
			if count != nil {
				count(n)
			}
		}
		if err != nil {
			break
		}
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	} else {
		dst.Close()
	}
}

// NewLimiter builds a pacing limiter for a speed in kilobits per second, or
// nil for no limit.
func NewLimiter(kbps int) *rate.Limiter {
	if kbps <= 0 {
		return nil
	}
	bps := float64(kbps) * 1000 / 8
	burst := int(bps / 10)
	burst = max(burst, 16*1024)
	burst = min(burst, 256*1024)
	return rate.NewLimiter(rate.Limit(bps), burst)
}

// IsPublic reports whether addr is a globally routable unicast address - the
// only kind of destination either proxy should ever dial on someone's behalf.
func IsPublic(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() ||
		a.IsLinkLocalUnicast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	for _, p := range blocked {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

var blocked = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
}

// ChanListener is a net.Listener fed by hand, so that a connection accepted
// on one socket and inspected can be passed on to an http.Server.
type ChanListener struct {
	ch     chan net.Conn
	addr   net.Addr
	once   sync.Once
	closed chan struct{}
}

func NewChanListener(addr net.Addr) *ChanListener {
	return &ChanListener{ch: make(chan net.Conn, 64), addr: addr, closed: make(chan struct{})}
}

func (l *ChanListener) Push(c net.Conn) {
	select {
	case l.ch <- c:
	case <-l.closed:
		c.Close()
	}
}

func (l *ChanListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.ch:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *ChanListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *ChanListener) Addr() net.Addr { return l.addr }

// RemoteAddr returns the peer address of c as a netip.Addr.
func RemoteAddr(c net.Conn) netip.Addr {
	if ap, err := netip.ParseAddrPort(c.RemoteAddr().String()); err == nil {
		return ap.Addr().Unmap()
	}
	return netip.Addr{}
}

// IsClosed reports errors that only mean the listener was shut down.
func IsClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF)
}
