package http_server

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

type trustedProxySet []netip.Prefix

func newTrustedProxySet(entries []string) (trustedProxySet, error) {
	proxies := make(trustedProxySet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			addr, addrErr := netip.ParseAddr(entry)
			if addrErr != nil {
				return nil, fmt.Errorf("invalid trusted proxy %q: expected IP address or CIDR", entry)
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		if prefix.Addr().Is4In6() {
			if prefix.Bits() < 96 {
				return nil, fmt.Errorf("invalid trusted proxy %q: IPv4-mapped CIDR prefix must be at least /96", entry)
			}
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		proxies = append(proxies, prefix.Masked())
	}
	return proxies, nil
}

func (p trustedProxySet) containsRemoteAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		host = strings.Trim(strings.TrimSpace(remoteAddr), "[]")
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range p {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func forwardedHost(header string) string {
	parts := strings.Split(header, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		if host := strings.TrimSpace(parts[i]); host != "" {
			return host
		}
	}
	return ""
}
