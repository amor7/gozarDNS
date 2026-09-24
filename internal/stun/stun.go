// Package stun answers STUN Binding requests and nothing else.
//
// Consoles discover their NAT mapping by asking a STUN server at one of the
// names the gate routes. That request arrives here over plain UDP, straight
// from the console, so the gate sees its real public address and can tell it
// so. No relaying (TURN) is ever offered.
package stun

import (
	"encoding/binary"
	"hash/crc32"
	"log/slog"
	"net"
	"net/netip"
)

const (
	magicCookie   = 0x2112A442
	bindingReq    = 0x0001
	bindingOK     = 0x0101
	attrMapped    = 0x0001
	attrXorMapped = 0x0020
	attrSoftware  = 0x8022
	attrFingerpr  = 0x8028
	fingerprintXO = 0x5354554e
)

// Serve answers on pc until it is closed.
func Serve(pc net.PacketConn, log *slog.Logger) {
	buf := make([]byte, 1500)
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		ua, ok := addr.(*net.UDPAddr)
		if !ok {
			continue
		}
		if resp := Answer(buf[:n], ua.AddrPort()); resp != nil {
			if _, err := pc.WriteTo(resp, addr); err != nil {
				log.Debug("stun reply failed", "err", err)
			}
		}
	}
}

// Answer builds the response to one datagram, or nil when it is not a
// well-formed Binding request.
func Answer(req []byte, from netip.AddrPort) []byte {
	if len(req) < 20 || req[0]&0xC0 != 0 {
		return nil
	}
	if binary.BigEndian.Uint16(req[0:2]) != bindingReq {
		return nil
	}
	if int(binary.BigEndian.Uint16(req[2:4]))+20 != len(req) {
		return nil
	}
	cookie := binary.BigEndian.Uint32(req[4:8])
	txid := req[8:20]
	wantFP := hasAttr(req[20:], attrFingerpr)

	addr := from.Addr().Unmap()
	port := from.Port()

	var body []byte
	body = appendAddr(body, attrMapped, addr, port, nil)
	if cookie == magicCookie { // RFC 5389 client; the XOR form is what it reads
		key := make([]byte, 16)
		binary.BigEndian.PutUint32(key, magicCookie)
		copy(key[4:], txid)
		body = appendAddr(body, attrXorMapped, addr, port, key)
	}
	body = appendAttr(body, attrSoftware, []byte("gozar"))

	msg := make([]byte, 20, 20+len(body)+8)
	binary.BigEndian.PutUint16(msg[0:], bindingOK)
	copy(msg[4:8], req[4:8])
	copy(msg[8:20], txid)
	msg = append(msg, body...)
	if wantFP && cookie == magicCookie {
		binary.BigEndian.PutUint16(msg[2:], uint16(len(msg)-20+8))
		crc := crc32.ChecksumIEEE(msg) ^ fingerprintXO
		v := make([]byte, 4)
		binary.BigEndian.PutUint32(v, crc)
		msg = appendAttr(msg, attrFingerpr, v)
	}
	binary.BigEndian.PutUint16(msg[2:], uint16(len(msg)-20))
	return msg
}

func appendAddr(b []byte, typ uint16, a netip.Addr, port uint16, xorKey []byte) []byte {
	ip := a.AsSlice()
	fam := byte(1)
	if a.Is6() {
		fam = 2
	}
	v := make([]byte, 4+len(ip))
	v[1] = fam
	p := port
	if xorKey != nil {
		p ^= uint16(magicCookie >> 16)
		for i := range ip {
			ip[i] ^= xorKey[i]
		}
	}
	binary.BigEndian.PutUint16(v[2:], p)
	copy(v[4:], ip)
	return appendAttr(b, typ, v)
}

func appendAttr(b []byte, typ uint16, v []byte) []byte {
	h := make([]byte, 4)
	binary.BigEndian.PutUint16(h, typ)
	binary.BigEndian.PutUint16(h[2:], uint16(len(v)))
	b = append(b, h...)
	b = append(b, v...)
	for len(v)%4 != 0 {
		b = append(b, 0)
		v = append(v, 0)
	}
	return b
}

func hasAttr(attrs []byte, want uint16) bool {
	for len(attrs) >= 4 {
		t := binary.BigEndian.Uint16(attrs)
		l := int(binary.BigEndian.Uint16(attrs[2:]))
		if t == want {
			return true
		}
		adv := 4 + (l+3)&^3
		if adv > len(attrs) {
			return false
		}
		attrs = attrs[adv:]
	}
	return false
}
