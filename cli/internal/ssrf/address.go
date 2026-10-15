// Package ssrf provides the address policy for HTTP requests to untrusted
// destinations. Callers must apply it to every DNS result at connection time
// and connect to the validated IP without a second hostname lookup.
package ssrf

import "net/netip"

// BlockedAddress reports whether an address is unsuitable for an untrusted
// outbound HTTP destination. It rejects invalid, scoped, private, and special-use
// addresses. IPv4-mapped IPv6 addresses use the IPv4 policy. Standardized NAT64,
// Teredo, and 6to4 ranges are blocked in full, including public embedded IPv4
// addresses; the client cannot establish the translation's destination policy.
// Deployment-specific translation routes still require network egress controls.
func BlockedAddress(address netip.Addr) bool {
	if address.Zone() != "" {
		return true
	}
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() {
		return true
	}
	for _, prefix := range specialUsePrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// This conservative policy also excludes special-purpose anycast services.
// Sources: https://www.iana.org/assignments/iana-ipv4-special-registry/
// and https://www.iana.org/assignments/iana-ipv6-special-registry/.
// Keep it shared by preview fetches and Web Push delivery.
var specialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"), netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("100:0:0:1::/64"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("2620:4f:8000::/48"),
	netip.MustParsePrefix("3fff::/20"), netip.MustParsePrefix("5f00::/16"),
}
