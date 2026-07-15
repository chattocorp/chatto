package linkpreview

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxURLs  int
		expected []string
	}{
		{"no URLs", "hello world", 5, nil},
		{"single URL", "check out https://example.com please", 5, []string{"https://example.com"}},
		{"multiple URLs", "see https://a.com and https://b.com", 5, []string{"https://a.com", "https://b.com"}},
		{"respects maxURLs", "see https://a.com and https://b.com", 1, []string{"https://a.com"}},
		{"deduplicates", "see https://example.com and https://example.com again", 5, []string{"https://example.com"}},
		{"strips trailing punctuation", "visit https://example.com.", 5, []string{"https://example.com"}},
		{"strips multiple trailing punctuation marks", "visit https://example.com/path!!", 5, []string{"https://example.com/path"}},
		{"strips wrapper closing parenthesis", "visit (https://example.com/path)", 5, []string{"https://example.com/path"}},
		{"keeps balanced path parentheses", "visit https://example.com/path_(v1)", 5, []string{"https://example.com/path_(v1)"}},
		{"keeps query string", "visit https://example.com/path?q=1&sort=desc", 5, []string{"https://example.com/path?q=1&sort=desc"}},
		{"deduplicates ignoring fragment", "see https://example.com/a#one and https://example.com/a#two", 5, []string{"https://example.com/a#one"}},
		{"deduplicates lowercased host", "see https://Example.com/a and https://example.com/a", 5, []string{"https://Example.com/a"}},
		{"http scheme", "visit http://example.com", 5, []string{"http://example.com"}},
		{"zero maxURLs", "https://example.com", 0, nil},
		{"negative maxURLs", "https://example.com", -1, nil},
		{"ignores ftp scheme", "visit ftp://example.com", 5, nil},
		{"ignores email addresses", "mail user@example.com", 5, nil},
		{"ignores inline code URLs", "run `curl https://example.com` first", 5, nil},
		{"ignores escaped-backtick inline code URLs", "run \\`curl https://example.com\\` first", 5, nil},
		{"detects URL immediately after inline code", "`curl`https://example.com", 5, []string{"https://example.com"}},
		{"ignores fenced code URLs", "```\nhttps://example.com\n```\nhttps://outside.example", 5, []string{"https://outside.example"}},
		{"ignores indented code URLs", "    https://example.com\nhttps://outside.example", 5, []string{"https://outside.example"}},
		{"ignores blockquote URLs", "> https://quoted.example\n\nhttps://outside.example", 5, []string{"https://outside.example"}},
		{"detects explicit markdown link destination", "read [the docs](https://example.com/docs)", 5, []string{"https://example.com/docs"}},
		{"detects angle autolink", "read <https://example.com/docs>", 5, []string{"https://example.com/docs"}},
		{"preserves order around excluded markdown regions", "https://a.example `https://skip.example` https://b.example\n> https://quote.example\n\nhttps://c.example", 5, []string{"https://a.example", "https://b.example", "https://c.example"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractURLs(tt.text, tt.maxURLs)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseYouTubeVideoID(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		// Valid YouTube URLs
		{"watch URL", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"watch URL with params", "https://www.youtube.com/watch?feature=share&v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"embed URL", "https://www.youtube.com/embed/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"short URL", "https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"shorts URL", "https://www.youtube.com/shorts/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"v/ URL", "https://www.youtube.com/v/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"mobile URL", "https://m.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"no www", "https://youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"watch URL with trailing params", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=42", "dQw4w9WgXcQ"},
		{"short URL with query", "https://youtu.be/dQw4w9WgXcQ?t=42", "dQw4w9WgXcQ"},

		// Non-YouTube URLs that should NOT match (hostname-anchored)
		{"not youtube domain", "https://notyoutube.com/watch?v=dQw4w9WgXcQ", ""},
		{"youtube in path", "https://evil.com/redirect?to=youtube.com/watch?v=dQw4w9WgXcQ", ""},
		{"youtube in subdomain", "https://fakeyoutube.com/watch?v=dQw4w9WgXcQ", ""},
		{"evil redirect", "https://evil.com/youtube.com/watch?v=dQw4w9WgXcQ", ""},

		// Invalid URLs
		{"empty string", "", ""},
		{"not a URL", "not-a-url", ""},
		{"wrong ID length", "https://youtu.be/short", ""},
		{"short URL with extra path segment", "https://youtu.be/dQw4w9WgXcQ/extra", ""},
		{"no video ID", "https://www.youtube.com/watch", ""},
		{"non-http youtube URL", "ftp://www.youtube.com/watch?v=dQw4w9WgXcQ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseYouTubeVideoID(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsYouTubeURL(t *testing.T) {
	assert.True(t, IsYouTubeURL("https://www.youtube.com/watch?v=dQw4w9WgXcQ"))
	assert.True(t, IsYouTubeURL("https://youtu.be/dQw4w9WgXcQ"))
	assert.False(t, IsYouTubeURL("https://example.com"))
	assert.False(t, IsYouTubeURL("https://notyoutube.com/watch?v=dQw4w9WgXcQ"))
}

func TestIsBlueskyPostURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://bsky.app/profile/bsky.app/post/3kq7aeuwbg42k", true},
		{"https://bsky.app/profile/did:plc:z72i7hdynmk6r22z27h6tvur/post/3kq7aeuwbg42k", true},
		{"https://BSKY.APP/profile/bsky.app/post/3kq7aeuwbg42k?ref=share", true},
		{"https://bsky.app/profile/bsky.app", false},
		{"https://bsky.app/profile/bsky.app/post/", false},
		{"https://embed.bsky.app/profile/bsky.app/post/3kq7aeuwbg42k", false},
		{"https://notbsky.app/profile/bsky.app/post/3kq7aeuwbg42k", false},
		{"ftp://bsky.app/profile/bsky.app/post/3kq7aeuwbg42k", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsBlueskyPostURL(tt.url))
		})
	}
}

func TestParseMastodonStatusURL(t *testing.T) {
	tests := []struct {
		url      string
		origin   string
		statusID string
		ok       bool
	}{
		{"https://mastodon.social/@Gargron/103270115826048975", "https://mastodon.social", "103270115826048975", true},
		{"https://social.example/users/alice/statuses/12345?ref=share", "https://social.example", "12345", true},
		{"https://social.example/@alice/12345/", "https://social.example", "12345", true},
		{"https://social.example/@alice", "", "", false},
		{"https://social.example/@alice/not-a-status", "", "", false},
		{"https://social.example/tags/12345", "", "", false},
		{"ftp://social.example/@alice/12345", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			origin, statusID, ok := ParseMastodonStatusURL(tt.url)
			assert.Equal(t, tt.origin, origin)
			assert.Equal(t, tt.statusID, statusID)
			assert.Equal(t, tt.ok, ok)
		})
	}
}
