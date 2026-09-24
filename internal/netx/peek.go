// Package netx holds the small pieces of TCP plumbing both roles share:
// reading the destination out of the first bytes of a connection, handing a
// connection on with those bytes put back, and copying in both directions.
package netx

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"time"
)

var ErrNotTLS = errors.New("not a TLS ClientHello")

// Conn is a net.Conn whose first bytes have already been read once.
type Conn struct {
	net.Conn
	r io.Reader
}

func (c *Conn) Read(p []byte) (int, error) { return c.r.Read(p) }

// CloseWrite passes a half-close through when the underlying conn supports it.
func (c *Conn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return c.Conn.Close()
}

func Rewind(c net.Conn, prefix []byte) *Conn {
	return &Conn{Conn: c, r: io.MultiReader(bytes.NewReader(prefix), c)}
}

const maxHello = 64 * 1024

// PeekSNI reads a TLS ClientHello and returns the server name it announces.
// The returned conn replays everything that was read.
func PeekSNI(c net.Conn, timeout time.Duration) (string, *Conn, error) {
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	defer c.SetReadDeadline(time.Time{})

	var buf bytes.Buffer
	var hs []byte // handshake bytes, reassembled across records
	hdr := make([]byte, 5)
	for {
		if _, err := io.ReadFull(c, hdr); err != nil {
			return "", Rewind(c, buf.Bytes()), err
		}
		buf.Write(hdr)
		if hdr[0] != 0x16 {
			return "", Rewind(c, buf.Bytes()), ErrNotTLS
		}
		n := int(binary.BigEndian.Uint16(hdr[3:5]))
		if n == 0 || buf.Len()+n > maxHello {
			return "", Rewind(c, buf.Bytes()), ErrNotTLS
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(c, body); err != nil {
			buf.Write(body)
			return "", Rewind(c, buf.Bytes()), err
		}
		buf.Write(body)
		hs = append(hs, body...)
		if len(hs) >= 4 {
			if hs[0] != 0x01 {
				return "", Rewind(c, buf.Bytes()), ErrNotTLS
			}
			need := 4 + (int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3]))
			if need > maxHello {
				return "", Rewind(c, buf.Bytes()), ErrNotTLS
			}
			if len(hs) >= need {
				name, err := parseClientHello(hs[4:need])
				return name, Rewind(c, buf.Bytes()), err
			}
		}
	}
}

func parseClientHello(b []byte) (string, error) {
	r := reader(b)
	if !r.skip(2 + 32) { // version, random
		return "", ErrNotTLS
	}
	if !r.skipVec8() || !r.skipVec16() || !r.skipVec8() { // session id, suites, compression
		return "", ErrNotTLS
	}
	if len(r) == 0 {
		return "", nil // no extensions at all
	}
	exts, ok := r.vec16()
	if !ok {
		return "", ErrNotTLS
	}
	for len(exts) >= 4 {
		typ := binary.BigEndian.Uint16(exts)
		n := int(binary.BigEndian.Uint16(exts[2:]))
		if len(exts) < 4+n {
			return "", ErrNotTLS
		}
		data := exts[4 : 4+n]
		exts = exts[4+n:]
		if typ != 0 { // server_name
			continue
		}
		list := reader(data)
		names, ok := list.vec16()
		if !ok {
			return "", ErrNotTLS
		}
		for len(names) >= 3 {
			kind := names[0]
			l := int(binary.BigEndian.Uint16(names[1:]))
			if len(names) < 3+l {
				return "", ErrNotTLS
			}
			if kind == 0 {
				return strings.ToLower(string(names[3 : 3+l])), nil
			}
			names = names[3+l:]
		}
	}
	return "", nil
}

type reader []byte

func (r *reader) skip(n int) bool {
	if len(*r) < n {
		return false
	}
	*r = (*r)[n:]
	return true
}

func (r *reader) skipVec8() bool {
	if len(*r) < 1 {
		return false
	}
	return r.skip(1 + int((*r)[0]))
}

func (r *reader) skipVec16() bool {
	_, ok := r.vec16()
	return ok
}

func (r *reader) vec16() ([]byte, bool) {
	if len(*r) < 2 {
		return nil, false
	}
	n := int(binary.BigEndian.Uint16(*r))
	if len(*r) < 2+n {
		return nil, false
	}
	v := (*r)[2 : 2+n]
	*r = (*r)[2+n:]
	return v, true
}

// PeekHost reads an HTTP/1.x request head and returns its Host, without the
// port. The returned conn replays everything that was read from the socket.
func PeekHost(c net.Conn, timeout time.Duration) (string, *Conn, error) {
	_ = c.SetReadDeadline(time.Now().Add(timeout))
	defer c.SetReadDeadline(time.Time{})

	// The tee sits under the bufio reader, so buf holds every byte taken off
	// the socket - including any the bufio reader fetched but never returned.
	var buf bytes.Buffer
	br := bufio.NewReaderSize(io.TeeReader(c, &buf), 4096)
	host := ""
	for i := 0; ; i++ {
		line, err := br.ReadString('\n')
		if err == nil && (buf.Len() > 16*1024 || i > 100) {
			err = errors.New("request head too large")
		}
		if err != nil {
			return "", Rewind(c, buf.Bytes()), err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if i > 0 {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "host") {
				host = strings.ToLower(strings.TrimSpace(v))
			}
		}
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host, Rewind(c, buf.Bytes()), nil
}
