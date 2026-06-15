package linkpreview

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	textm "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// urlRegex matches HTTP/HTTPS URLs in text.
// This is a simplified regex that captures most common URL patterns.
var urlRegex = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `\[\]{}|\\^]+`)

var urlMarkdown = goldmark.New(
	goldmark.WithParser(parser.NewParser(
		parser.WithBlockParsers(
			util.Prioritized(parser.NewSetextHeadingParser(), 100),
			util.Prioritized(parser.NewThematicBreakParser(), 200),
			util.Prioritized(parser.NewListParser(), 300),
			util.Prioritized(parser.NewListItemParser(), 400),
			util.Prioritized(parser.NewCodeBlockParser(), 500),
			util.Prioritized(parser.NewATXHeadingParser(), 600),
			util.Prioritized(parser.NewFencedCodeBlockParser(), 700),
			util.Prioritized(parser.NewBlockquoteParser(), 800),
			util.Prioritized(parser.NewParagraphParser(), 1000),
		),
		parser.WithInlineParsers(
			util.Prioritized(parser.NewCodeSpanParser(), 100),
			util.Prioritized(parser.NewLinkParser(), 200),
			util.Prioritized(parser.NewAutoLinkParser(), 300),
		),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)),
)

func urlMarkdownSource(text string) string {
	// Match the Chatto renderer's disabled backslash escapes for code spans.
	return strings.ReplaceAll(text, "\\`", "`")
}

// ExtractURLs extracts unique HTTP/HTTPS URLs from text.
// Returns at most maxURLs URLs, in the order they appear.
func ExtractURLs(text string, maxURLs int) []string {
	if maxURLs <= 0 {
		return nil
	}
	if !strings.Contains(text, "http://") && !strings.Contains(text, "https://") {
		return nil
	}

	seen := make(map[string]bool)
	var result []string

	add := func(raw string) {
		if len(result) >= maxURLs {
			return
		}
		match := cleanURLMatch(raw)
		if match == "" {
			return
		}
		// Validate the URL
		u, err := url.Parse(match)
		if err != nil {
			return
		}

		// Only allow http/https schemes
		if u.Scheme != "http" && u.Scheme != "https" {
			return
		}

		// Skip if we've already seen this URL
		normalized := normalizeURL(u)
		if seen[normalized] {
			return
		}
		seen[normalized] = true

		result = append(result, match)
	}

	source := []byte(urlMarkdownSource(text))
	root := urlMarkdown.Parser().Parse(textm.NewReader(source))
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || len(result) >= maxURLs {
			return ast.WalkContinue, nil
		}

		switch node.Kind() {
		case ast.KindCodeSpan, ast.KindCodeBlock, ast.KindFencedCodeBlock, ast.KindBlockquote:
			return ast.WalkSkipChildren, nil
		case ast.KindLink:
			add(string(node.(*ast.Link).Destination))
			return ast.WalkSkipChildren, nil
		case ast.KindAutoLink:
			autoLink := node.(*ast.AutoLink)
			if autoLink.AutoLinkType == ast.AutoLinkURL {
				add(string(autoLink.URL(source)))
			}
			return ast.WalkSkipChildren, nil
		case ast.KindText:
			textNode := node.(*ast.Text)
			for _, match := range urlRegex.FindAllString(string(textNode.Segment.Value(source)), -1) {
				add(match)
			}
		case ast.KindString:
			for _, match := range urlRegex.FindAllString(string(node.(*ast.String).Value), -1) {
				add(match)
			}
		}

		return ast.WalkContinue, nil
	})

	if len(result) == 0 {
		return nil
	}

	return result
}

func cleanURLMatch(match string) string {
	for {
		cleaned := strings.TrimRight(match, ".,;:!?")
		if cleaned != match {
			match = cleaned
			continue
		}
		if strings.HasSuffix(match, ")") && countByte(match, ')') > countByte(match, '(') {
			match = strings.TrimSuffix(match, ")")
			continue
		}
		return match
	}
}

func countByte(value string, target byte) int {
	count := 0
	for i := range len(value) {
		if value[i] == target {
			count++
		}
	}
	return count
}

// normalizeURL normalizes a URL for deduplication and caching.
func normalizeURL(u *url.URL) string {
	// Lowercase scheme and host
	normalized := &url.URL{
		Scheme:   strings.ToLower(u.Scheme),
		Host:     strings.ToLower(u.Host),
		Path:     u.Path,
		RawQuery: u.RawQuery,
		Fragment: "", // Ignore fragments
	}
	return normalized.String()
}

// NormalizeURLString normalizes a URL string for caching.
func NormalizeURLString(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return normalizeURL(u)
}

// youtubeHosts is the set of valid YouTube hostnames.
var youtubeHosts = map[string]bool{
	"youtube.com":     true,
	"www.youtube.com": true,
	"m.youtube.com":   true,
	"youtu.be":        true,
}

// youtubePathRegex matches YouTube video path/query patterns.
// Only applied after hostname validation to prevent matching non-YouTube domains.
var youtubePathRegex = regexp.MustCompile(
	`^/(?:watch\?(?:.*&)?v=|embed/|v/|shorts/)([a-zA-Z0-9_-]{11})`,
)

// ParseYouTubeVideoID extracts the video ID from a YouTube URL.
// Returns empty string if the URL is not a valid YouTube video URL.
func ParseYouTubeVideoID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}

	host := strings.ToLower(u.Hostname())
	if !youtubeHosts[host] {
		return ""
	}

	// For youtu.be short URLs, the video ID is the path
	if host == "youtu.be" {
		id := strings.TrimPrefix(u.Path, "/")
		if len(id) == 11 {
			return id
		}
		return ""
	}

	// For youtube.com, match path/query patterns
	pathAndQuery := u.Path
	if u.RawQuery != "" {
		pathAndQuery += "?" + u.RawQuery
	}
	matches := youtubePathRegex.FindStringSubmatch(pathAndQuery)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// IsYouTubeURL checks if a URL is a YouTube video URL.
func IsYouTubeURL(rawURL string) bool {
	return ParseYouTubeVideoID(rawURL) != ""
}
