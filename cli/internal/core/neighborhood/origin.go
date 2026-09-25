// Package neighborhood discovers public Chatto servers through Neighbor
// recommendations. The server runs discovery in the background so that
// clients can show the Neighborhood without contacting the discovered
// servers directly. See FDR-042 and ADR-105.
package neighborhood

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

// CanonicalOrigin converts an HTTP or HTTPS origin to its canonical form. It
// lowercases the scheme and host, converts internationalized hostnames to
// ASCII, and removes default ports. It rejects a value that has credentials,
// a path other than "/", a query, or a fragment.
func CanonicalOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", false
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" || strings.Contains(hostname, "%") {
		return "", false
	}
	if ip := net.ParseIP(hostname); ip != nil {
		hostname = ip.String()
	} else {
		hostname, err = idna.Lookup.ToASCII(hostname)
		if err != nil || hostname == "" {
			return "", false
		}
		hostname = strings.ToLower(hostname)
	}
	port := parsed.Port()
	if port == "" && strings.HasSuffix(parsed.Host, ":") {
		return "", false
	}
	if port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", false
		}
		if (scheme == "http" && portNumber == 80) || (scheme == "https" && portNumber == 443) {
			port = ""
		} else {
			port = strconv.Itoa(portNumber)
		}
	}
	host := hostname
	if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	return scheme + "://" + host, true
}
