package stun

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func TestBindingResponseCarriesXorMappedAddress(t *testing.T) {
	req := make([]byte, 20)
	binary.BigEndian.PutUint16(req, bindingReq)
	binary.BigEndian.PutUint32(req[4:], magicCookie)
	copy(req[8:], "abcdefghijkl")

	from := netip.MustParseAddrPort("5.188.44.19:51234")
	resp := Answer(req, from)
	if resp == nil {
		t.Fatal("no response")
	}
	if binary.BigEndian.Uint16(resp) != bindingOK {
		t.Fatalf("type %x", resp[:2])
	}
	if int(binary.BigEndian.Uint16(resp[2:]))+20 != len(resp) {
		t.Fatal("length field wrong")
	}
	attrs := resp[20:]
	for len(attrs) >= 4 {
		typ := binary.BigEndian.Uint16(attrs)
		l := int(binary.BigEndian.Uint16(attrs[2:]))
		v := attrs[4 : 4+l]
		if typ == attrXorMapped {
			port := binary.BigEndian.Uint16(v[2:]) ^ uint16(magicCookie>>16)
			var ip [4]byte
			binary.BigEndian.PutUint32(ip[:], binary.BigEndian.Uint32(v[4:])^magicCookie)
			got := netip.AddrPortFrom(netip.AddrFrom4(ip), port)
			if got != from {
				t.Fatalf("got %v", got)
			}
			return
		}
		attrs = attrs[4+(l+3)&^3:]
	}
	t.Fatal("no XOR-MAPPED-ADDRESS")
}

func TestIgnoresGarbage(t *testing.T) {
	if Answer([]byte("hello, world, this is not stun"), netip.MustParseAddrPort("1.2.3.4:5")) != nil {
		t.Fatal("answered garbage")
	}
}
