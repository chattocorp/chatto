package linkpreview

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func response(status int, contentType string, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFetchBlueskyPost(t *testing.T) {
	const postURL = "https://bsky.app/profile/bsky.app/post/3kq7aeuwbg42k"
	const atURI = "at://did:plc:z72i7hdynmk6r22z27h6tvur/app.bsky.feed.post/3kq7aeuwbg42k"

	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Host {
			case "embed.bsky.app":
				assert.Equal(t, "/oembed", req.URL.Path)
				assert.Equal(t, postURL, req.URL.Query().Get("url"))
				return response(http.StatusOK, "application/json", `{
					"html":"<blockquote class=\"bluesky-embed\" data-bluesky-uri=\"`+atURI+`\"><p lang=\"en\">Fallback text.</p></blockquote>"
				}`), nil
			case "public.api.bsky.app":
				assert.Equal(t, "/xrpc/app.bsky.feed.getPosts", req.URL.Path)
				assert.Equal(t, atURI, req.URL.Query().Get("uris"))
				return response(http.StatusOK, "application/json", `{"posts":[{
					"uri":"`+atURI+`",
					"author":{"displayName":"Bluesky","handle":"bsky.app"},
					"record":{"text":"A post with & character.","createdAt":"2024-04-15T21:48:40.709Z"},
					"embed":{}
				}]}`), nil
			default:
				t.Fatalf("unexpected request host %q", req.URL.Host)
				return nil, nil
			}
		})},
	}

	result, err := fetcher.Fetch(context.Background(), postURL)
	require.NoError(t, err)
	assert.Equal(t, "Bluesky (@bsky.app)", result.Title)
	assert.Equal(t, "A post with & character.", result.Description)
	assert.Equal(t, "Bluesky", result.SiteName)
	assert.Equal(t, "bluesky", result.EmbedType)
	assert.Equal(t, atURI, result.EmbedID)
	require.NotNil(t, result.SocialPost)
	assert.Equal(t, "bluesky", result.SocialPost.Provider)
	assert.Equal(t, "A post with & character.", result.SocialPost.Text)
	require.NotNil(t, result.SocialPost.Author)
	assert.Equal(t, "Bluesky", result.SocialPost.Author.DisplayName)
	assert.Equal(t, "bsky.app", result.SocialPost.Author.Handle)
	require.NotNil(t, result.SocialPost.PublishedAt)
	assert.Equal(t, result.SocialPost, result.ToProto(postURL).GetSocialPost())
}

func TestFetchBlueskyPostFallsBackToOpenGraph(t *testing.T) {
	const postURL = "https://bsky.app/profile/bsky.app/post/3kq7aeuwbg42k"

	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "embed.bsky.app" {
				return response(http.StatusNotFound, "application/json", `{}`), nil
			}
			assert.Equal(t, postURL, req.URL.String())
			return response(http.StatusOK, "text/html", `<html><head>
				<meta property="og:title" content="Fallback title">
				<meta property="og:description" content="Fallback description">
				<meta property="og:site_name" content="Bluesky Social">
			</head></html>`), nil
		})},
	}

	result, err := fetcher.Fetch(context.Background(), postURL)
	require.NoError(t, err)
	assert.Equal(t, "Fallback title", result.Title)
	assert.Equal(t, "Fallback description", result.Description)
	assert.Equal(t, "generic", result.EmbedType)
}

func TestParseBlueskyOEmbedHTMLRejectsInvalidURI(t *testing.T) {
	_, _, err := parseBlueskyOEmbedHTML(
		`<blockquote data-bluesky-uri="https://example.com"><p>Post</p></blockquote>`,
	)
	require.Error(t, err)
}

func TestFetchBlueskyPostRejectsLabelledContent(t *testing.T) {
	const atURI = "at://did:plc:z72i7hdynmk6r22z27h6tvur/app.bsky.feed.post/3kq7aeuwbg42k"
	fetcher := &Fetcher{
		httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return response(http.StatusOK, "application/json", `{"posts":[{
				"uri":"`+atURI+`",
				"labels":[{"val":"porn"}],
				"author":{"displayName":"Bluesky","handle":"bsky.app"},
				"record":{"text":"Labelled post","createdAt":"2024-04-15T21:48:40.709Z"},
				"embed":{}
			}]}`), nil
		})},
	}

	_, err := fetcher.fetchBlueskyPost(context.Background(), atURI)
	require.ErrorContains(t, err, "moderation")
}

func TestTruncateUTF8BytesPreservesValidUTF8(t *testing.T) {
	assert.Equal(t, "abc", truncateUTF8Bytes("abcdef", 3))
	assert.Equal(t, "🙂", truncateUTF8Bytes("🙂🙂", 5))
}

func TestSafeExternalURL(t *testing.T) {
	assert.Equal(t, "https://example.com/story", safeExternalURL("https://example.com/story"))
	assert.Empty(t, safeExternalURL("javascript:alert(1)"))
	assert.Empty(t, safeExternalURL("https:///missing-host"))
}
