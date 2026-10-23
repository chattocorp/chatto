package ssrf

import (
	"net/netip"
	"testing"
)

func TestBlockedAddress(t *testing.T) {
	blocked := []string{
		"0.0.0.0", "0.1.2.3", "127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.0.1",
		"100.64.0.0", "100.100.100.200", "100.127.255.255", "169.254.169.254",
		"192.0.0.1", "192.0.2.1", "192.31.196.1", "192.52.193.1", "192.88.99.1",
		"192.175.48.1", "198.18.0.1", "198.19.255.255", "198.51.100.1", "203.0.113.1",
		"224.0.0.1", "240.0.0.1", "255.255.255.255",
		"::", "::1", "::a00:1", "::ffff:0:a00:1", "::ffff:127.0.0.1", "::ffff:100.64.0.1",
		"64:ff9b::a00:1", "64:ff9b::808:808", "64:ff9b:1::a00:1",
		"100::1", "100:0:0:1::1", "2001::1", "2001:2::1", "2001:db8::1",
		"2002:a00:1::1", "2620:4f:8000::1", "3fff::1", "5f00::1",
		"fc00::1", "fe80::1", "fec0::1", "ff0e::1", "2001:4860::1%eth0",
	}
	for _, raw := range blocked {
		t.Run(raw, func(t *testing.T) {
			if !BlockedAddress(netip.MustParseAddr(raw)) {
				t.Fatal("special-use address was allowed")
			}
		})
	}
	if !BlockedAddress(netip.Addr{}) {
		t.Fatal("invalid address was allowed")
	}
	for _, raw := range []string{
		"8.8.8.8", "93.184.216.34", "100.63.255.255", "100.128.0.0",
		"198.17.255.255", "198.20.0.0", "::ffff:8.8.8.8",
		"2001:4860:4860::8888", "2606:4700:4700::1111",
	} {
		t.Run(raw, func(t *testing.T) {
			if BlockedAddress(netip.MustParseAddr(raw)) {
				t.Fatal("public address was blocked")
			}
		})
	}
}
