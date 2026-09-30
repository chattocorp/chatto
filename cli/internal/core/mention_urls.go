package core

import (
	"regexp"
	"strings"

	"mvdan.cc/xurls/v2"
)

// The bundled frontend renders message bodies with markdown-it and its
// linkify-it URL detection, and it never finds a mention inside a URL that it
// renders as a link. Mention extraction must skip the same URLs, or a handle
// in a URL path such as https://example.social/@alice/123 notifies a user
// whom the rendered message does not mention.
//
// xurls finds the URL candidates. The filters below approximate the parts of
// linkify-it's behavior that differ from xurls. The shared cases in
// testdata/mentions/extraction.json check that both sides agree.

var (
	mentionURLRegexp = xurls.Relaxed()

	// mentionURLSchemeRegexp matches the scheme prefix of a URL candidate.
	mentionURLSchemeRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)

	// mentionLinkifySchemes holds the schemes that linkify-it links by default.
	mentionLinkifySchemes = map[string]bool{"http:": true, "https:": true, "ftp:": true, "mailto:": true}

	// mentionLinkifyTLDs holds the top-level domains that linkify-it accepts
	// without a scheme. The frontend loads the full IANA list from the `tlds`
	// package; xurls.TLDs is the same list. Pseudo top-level domains such as
	// .example or .local are not included.
	mentionLinkifyTLDs = func() map[string]bool {
		tlds := make(map[string]bool, len(xurls.TLDs))
		for _, tld := range xurls.TLDs {
			tlds[strings.ToLower(tld)] = true
		}
		return tlds
	}()
)

// linkifiedRange is a half-open byte range [start, end) of a URL in the
// message source.
type linkifiedRange struct {
	start, end int
}

// linkifiedURLRanges returns the byte ranges of the URLs in source that the
// frontend renders as links.
func linkifiedURLRanges(source []byte) []linkifiedRange {
	var ranges []linkifiedRange
	for _, match := range mentionURLRegexp.FindAllIndex(source, -1) {
		if isLinkifiedURL(source, match[0], match[1]) {
			ranges = append(ranges, linkifiedRange{start: match[0], end: match[1]})
		}
	}
	return ranges
}

// isLinkifiedURL reports whether linkify-it links the xurls candidate
// source[start:end].
func isLinkifiedURL(source []byte, start, end int) bool {
	candidate := string(source[start:end])
	if scheme := mentionURLSchemeRegexp.FindString(candidate); scheme != "" {
		return mentionLinkifySchemes[strings.ToLower(scheme)]
	}

	// linkify-it does not link a URL without a scheme directly after one of
	// these characters, for example the domain-like handle in @alice.dev.
	if start > 0 && strings.IndexByte(".:/-_@", source[start-1]) >= 0 {
		return false
	}
	return mentionLinkifyTLDs[strings.ToLower(urlTopLevelDomain(candidate))]
}

// urlTopLevelDomain returns the last label of the host of a URL without a
// scheme, for example "de" for "social.5f9.de/@alice".
func urlTopLevelDomain(candidate string) string {
	host := candidate
	if at := strings.LastIndexByte(host, '@'); at >= 0 && !strings.ContainsAny(host[:at], "/?#") {
		host = host[at+1:] // Email address: use the domain part.
	}
	if cut := strings.IndexAny(host, "/?#:"); cut >= 0 {
		host = host[:cut]
	}
	return host[strings.LastIndexByte(host, '.')+1:]
}

// insideLinkifiedURL reports whether the byte at offset belongs to a URL range.
func insideLinkifiedURL(ranges []linkifiedRange, offset int) bool {
	for _, r := range ranges {
		if offset >= r.start && offset < r.end {
			return true
		}
	}
	return false
}
