package bleve

import (
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Address terms are derived from the current body for the disposable index.
// They are not part of the stored projection state used for EVT replay.
type addressTerms struct {
	urls   []string
	emails []string
	hosts  []string
	parts  []string
}

var hostnamePattern = regexp.MustCompile(`(?i)^[\p{L}\p{N}](?:[\p{L}\p{N}-]*[\p{L}\p{N}])?(?:\.[\p{L}\p{N}](?:[\p{L}\p{N}-]*[\p{L}\p{N}])?)*\.[\p{L}]{2,}$`)

// Match URL and email candidates before standalone hostnames. A single scan
// prevents a host inside an address from becoming an unrelated second match.
var addressCandidatePattern = regexp.MustCompile(`(?i)https?://[^\s<>"'\[\]{}|\\^` + "`" + `]+|[^\s<>"'` + "`" + `()\[\]{};,:@]+@[^\s<>"'` + "`" + `()\[\]{};,:@/?#&]+|[\p{L}\p{N}-]+(?:\.[\p{L}\p{N}-]+)+`)

func extractAddressTerms(body string) addressTerms {
	var terms addressTerms
	seenURLs := make(map[string]bool)
	seenEmails := make(map[string]bool)
	seenHosts := make(map[string]bool)
	seenParts := make(map[string]bool)
	addHost := func(host string) {
		labels := strings.Split(host, ".")
		for i := 0; i < len(labels)-1; i++ {
			suffix := strings.Join(labels[i:], ".")
			if !seenHosts[suffix] {
				terms.hosts = append(terms.hosts, suffix)
				seenHosts[suffix] = true
			}
		}
	}
	addParts := func(value string) {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}) {
			part = strings.ToLower(part)
			if !seenParts[part] {
				terms.parts = append(terms.parts, part)
				seenParts[part] = true
			}
		}
	}

	for _, location := range addressCandidatePattern.FindAllStringIndex(body, -1) {
		if location[0] > 0 {
			previous, _ := utf8.DecodeLastRuneInString(body[:location[0]])
			if unicode.IsLetter(previous) || unicode.IsNumber(previous) || strings.ContainsRune("._-@", previous) {
				continue
			}
		}
		candidate := trimAddressPunctuation(body[location[0]:location[1]])
		if value, parsed := canonicalHTTPURL(candidate); value != "" {
			if !seenURLs[value] {
				terms.urls = append(terms.urls, value)
				seenURLs[value] = true
			}
			host := strings.ToLower(parsed.Hostname())
			addHost(host)
			addParts(host + " " + parsed.Path + " " + parsed.RawQuery + " " + parsed.Fragment)
			continue
		}
		if email, host := canonicalEmail(candidate); email != "" {
			if !seenEmails[email] {
				terms.emails = append(terms.emails, email)
				seenEmails[email] = true
			}
			addHost(host)
			addParts(strings.ReplaceAll(email, "@", " "))
			continue
		}
		if host := canonicalHostname(candidate); host != "" {
			addHost(host)
			addParts(host)
		}
	}
	return terms
}

func canonicalHTTPURL(raw string) (string, *url.URL) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return "", nil
	}
	host := canonicalHostname(parsed.Hostname())
	if host == "" {
		return "", nil
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String(), parsed
}

func canonicalHostname(raw string) string {
	if !hostnamePattern.MatchString(raw) {
		return ""
	}
	return strings.ToLower(raw)
}

func canonicalEmail(raw string) (string, string) {
	local, domain, found := strings.Cut(raw, "@")
	if !found || local == "" {
		return "", ""
	}
	host := canonicalHostname(domain)
	if host == "" {
		return "", ""
	}
	parsed, err := mail.ParseAddress(raw)
	if err != nil || parsed.Address != raw {
		return "", ""
	}
	return strings.ToLower(local) + "@" + host, host
}

func trimAddressPunctuation(value string) string {
	value = strings.TrimRight(value, ".,;:!?")
	for strings.HasSuffix(value, ")") && strings.Count(value, ")") > strings.Count(value, "(") {
		value = strings.TrimSuffix(value, ")")
	}
	return value
}
