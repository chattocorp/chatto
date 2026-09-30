package core

import (
	"bytes"
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

	// mentionURLSchemeRegexp matches the scheme of a URL candidate. A scheme
	// needs "//" after its colon, so the port in example.com:8443 is not a
	// scheme. mailto: is the only linkified scheme without "//".
	mentionURLSchemeRegexp = regexp.MustCompile(`^(?i:([a-z][a-z0-9+.\-]*://)|mailto:)`)

	// mentionLinkifySchemes holds the "//" schemes that linkify-it links by
	// default.
	mentionLinkifySchemes = map[string]bool{"http://": true, "https://": true, "ftp://": true}

	// mentionProtocolRelativeRegexp matches a protocol-relative URL such as
	// //example.com/@alice. linkify-it links these for localhost and for any
	// dotted host, also without a known top-level domain (//foo.local), which
	// xurls does not match. A single-label host such as //x is not linked.
	mentionProtocolRelativeRegexp = regexp.MustCompile(`//(?:localhost|[a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+)[^\s<>]*`)

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

// mentionURLIndex finds the URLs that the frontend links on the source lines
// that contain mentions. A URL cannot contain a line break, so each line is
// scanned at most once, and only when it contains a mention.
type mentionURLIndex struct {
	source []byte
	lines  map[int][]linkifiedRange // keyed by the offset of the line start
}

func newMentionURLIndex(source []byte) *mentionURLIndex {
	return &mentionURLIndex{source: source, lines: make(map[int][]linkifiedRange)}
}

// contains reports whether the byte at offset belongs to a linkified URL.
func (x *mentionURLIndex) contains(offset int) bool {
	lineStart := bytes.LastIndexByte(x.source[:offset], '\n') + 1
	ranges, scanned := x.lines[lineStart]
	if !scanned {
		lineEnd := len(x.source)
		if n := bytes.IndexByte(x.source[offset:], '\n'); n >= 0 {
			lineEnd = offset + n
		}
		ranges = linkifiedURLRanges(x.source, lineStart, lineEnd)
		x.lines[lineStart] = ranges
	}
	for _, r := range ranges {
		if offset >= r.start && offset < r.end {
			return true
		}
	}
	return false
}

// linkifiedURLRanges returns the byte ranges of the URLs in source[start:end]
// that the frontend renders as links. Ranges use offsets into source.
func linkifiedURLRanges(source []byte, start, end int) []linkifiedRange {
	var ranges []linkifiedRange
	line := source[start:end]
	for _, match := range mentionURLRegexp.FindAllIndex(line, -1) {
		if isLinkifiedURL(source, start+match[0], start+match[1]) {
			ranges = append(ranges, linkifiedRange{start: start + match[0], end: start + match[1]})
		}
	}
	for _, match := range mentionProtocolRelativeRegexp.FindAllIndex(line, -1) {
		if matchStart := start + match[0]; isLinkifyBoundary(source, matchStart) {
			ranges = append(ranges, linkifiedRange{start: matchStart, end: start + match[1]})
		}
	}
	return ranges
}

// isLinkifiedURL reports whether linkify-it links the xurls candidate
// source[start:end].
func isLinkifiedURL(source []byte, start, end int) bool {
	candidate := string(source[start:end])
	if scheme := mentionURLSchemeRegexp.FindStringSubmatch(candidate); scheme != nil {
		// scheme[1] is empty for mailto:, which linkify-it links.
		return scheme[1] == "" || mentionLinkifySchemes[strings.ToLower(scheme[1])]
	}
	if !isLinkifyBoundary(source, start) {
		return false
	}
	return mentionLinkifyTLDs[strings.ToLower(urlTopLevelDomain(candidate))]
}

// isLinkifyBoundary reports whether linkify-it can start a URL without a
// scheme at offset. It cannot start directly after a letter, a digit, or one
// of the characters .:/-_@, for example in the domain-like handle @alice.dev.
func isLinkifyBoundary(source []byte, offset int) bool {
	if offset == 0 {
		return true
	}
	previous := source[offset-1]
	return !isMentionAlphanumeric(previous) && strings.IndexByte(".:/-_@", previous) < 0
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
