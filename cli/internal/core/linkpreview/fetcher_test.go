package linkpreview

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/assets"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
					"embed":{"$type":"app.bsky.embed.record#view","record":{
						"$type":"app.bsky.embed.record#viewRecord",
						"uri":"at://did:plc:quoted/app.bsky.feed.post/quote123",
						"author":{"displayName":"Quoted author","handle":"quoted.example"},
						"value":{"text":"Quoted words","createdAt":"2024-04-15T20:00:00.000Z"},
						"embeds":[]
					}}
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
	assert.Equal(t, postURL, result.SocialPost.Url)
	assert.Equal(t, "A post with & character.", result.SocialPost.Text)
	require.NotNil(t, result.SocialPost.Author)
	assert.Equal(t, "Bluesky", result.SocialPost.Author.DisplayName)
	assert.Equal(t, "bsky.app", result.SocialPost.Author.Handle)
	require.NotNil(t, result.SocialPost.PublishedAt)
	require.NotNil(t, result.SocialPost.QuotedPost)
	assert.Equal(t, "Quoted words", result.SocialPost.QuotedPost.Text)
	assert.Equal(t, "https://bsky.app/profile/quoted.example/post/quote123", result.SocialPost.QuotedPost.Url)
	assert.Equal(t, result.SocialPost, result.ToProto(postURL).GetSocialPost())
}

func TestApplyBlueskyEmbedIncludesQuotedPostMedia(t *testing.T) {
	var pngData bytes.Buffer
	require.NoError(t, png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	assetNumber := 0
	assetsConfig := assets.DefaultConfig()
	fetcher := &Fetcher{
		logger:       log.New(io.Discard),
		assetsConfig: &assetsConfig,
		newAssetID: func() string {
			assetNumber++
			return fmt.Sprintf("asset-%d", assetNumber)
		},
		imageClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return response(http.StatusOK, "image/png", pngData.String()), nil
		})},
		storeImage: func(_ context.Context, assetID string, _ []byte, _ string) (*corev1.AssetRecord, error) {
			return &corev1.AssetRecord{
				Id:      assetID,
				Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: assetID}},
			}, nil
		},
	}
	snapshot := &corev1.SocialPostPreview{Provider: "bluesky"}
	embed := &blueskyEmbed{Record: &blueskyRecordView{
		URI:    "at://did:plc:quoted/app.bsky.feed.post/quote123",
		Author: blueskyAuthor{DisplayName: "Quoted author", Handle: "quoted.example"},
		Value:  blueskyRecord{Text: "Quoted words", CreatedAt: "2026-07-15T14:50:19.560Z"},
		Embeds: []blueskyEmbed{{
			// Bluesky returns this shape when the quoted post combines its own
			// quoted record with attached media.
			Media: &blueskyEmbed{Images: []blueskyImage{{
				Fullsize: "https://cdn.example/quote.png",
				Alt:      "A quoted attachment",
			}}},
			Record: &blueskyRecordView{Record: &blueskyRecordView{
				URI:    "at://did:plc:nested/app.bsky.feed.post/nested123",
				Author: blueskyAuthor{Handle: "nested.example"},
			}},
		}},
	}}
	budget := socialPostImageBudget{bytesRemaining: MaxSocialPostImageBytes, fetchesRemaining: MaxSocialPostImageFetches}

	fetcher.applyBlueskyEmbed(context.Background(), snapshot, embed, &budget, true)

	require.NotNil(t, snapshot.QuotedPost)
	assert.Equal(t, "https://bsky.app/profile/quoted.example/post/quote123", snapshot.QuotedPost.Url)
	assert.Equal(t, "Quoted words", snapshot.QuotedPost.Text)
	require.Len(t, snapshot.QuotedPost.Images, 1)
	assert.Equal(t, "A quoted attachment", snapshot.QuotedPost.Images[0].Alt)
	assert.Equal(t, "asset-1", snapshot.QuotedPost.Images[0].GetAsset().GetId())
}

func TestApplyBlueskyRecordWithMediaIncludesDirectMediaAndQuote(t *testing.T) {
	snapshot := &corev1.SocialPostPreview{Provider: "bluesky"}
	embed := &blueskyEmbed{
		Media: &blueskyEmbed{External: &blueskyExternal{URI: "https://example.com/story", Title: "Story"}},
		Record: &blueskyRecordView{Record: &blueskyRecordView{
			URI:    "at://did:plc:quoted/app.bsky.feed.post/quote123",
			Author: blueskyAuthor{Handle: "quoted.example"},
			Value:  blueskyRecord{Text: "Quoted words"},
		}},
	}
	fetcher := &Fetcher{logger: log.New(io.Discard)}
	budget := socialPostImageBudget{}

	fetcher.applyBlueskyEmbed(context.Background(), snapshot, embed, &budget, true)

	require.NotNil(t, snapshot.ExternalLink)
	assert.Equal(t, "https://example.com/story", snapshot.ExternalLink.Url)
	require.NotNil(t, snapshot.QuotedPost)
	assert.Equal(t, "Quoted words", snapshot.QuotedPost.Text)
}

func TestBlueskyRecordSnapshotBuildsURLBeforeTruncatingHandle(t *testing.T) {
	handle := strings.Repeat("a", 210) + ".example"
	snapshot := blueskyRecordSnapshot(&blueskyRecordView{
		URI:    "at://did:plc:quoted/app.bsky.feed.post/quote123",
		Author: blueskyAuthor{Handle: handle},
		Value:  blueskyRecord{Text: "Quoted words"},
	})

	require.NotNil(t, snapshot)
	assert.Len(t, snapshot.Author.Handle, 200)
	assert.Equal(t, "https://bsky.app/profile/"+handle+"/post/quote123", snapshot.Url)
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

func TestFetchBlueskyPostDoesNotOpenGraphFallbackForLabelledContent(t *testing.T) {
	const postURL = "https://bsky.app/profile/bsky.app/post/3kq7aeuwbg42k"
	const atURI = "at://did:plc:z72i7hdynmk6r22z27h6tvur/app.bsky.feed.post/3kq7aeuwbg42k"
	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Host {
			case "embed.bsky.app":
				return response(http.StatusOK, "application/json", `{
					"html":"<blockquote data-bluesky-uri=\"`+atURI+`\"></blockquote>"
				}`), nil
			case "public.api.bsky.app":
				return response(http.StatusOK, "application/json", `{"posts":[{
					"uri":"`+atURI+`",
					"labels":[{"val":"porn"}],
					"author":{"displayName":"Bluesky","handle":"bsky.app"},
					"record":{"text":"Labelled post","createdAt":"2024-04-15T21:48:40.709Z"},
					"embed":{}
				}]}`), nil
			default:
				t.Fatalf("unexpected fallback request to %q", req.URL.String())
				return nil, nil
			}
		})},
	}

	_, err := fetcher.Fetch(context.Background(), postURL)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestFetchBlueskyPostBoundsCompatibilityTitle(t *testing.T) {
	const postURL = "https://bsky.app/profile/bsky.app/post/3kq7aeuwbg42k"
	const atURI = "at://did:plc:z72i7hdynmk6r22z27h6tvur/app.bsky.feed.post/3kq7aeuwbg42k"
	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "embed.bsky.app" {
				return response(http.StatusOK, "application/json", `{
					"html":"<blockquote data-bluesky-uri=\"`+atURI+`\"></blockquote>"
				}`), nil
			}
			return response(http.StatusOK, "application/json", `{"posts":[{
				"uri":"`+atURI+`",
				"author":{"displayName":"`+strings.Repeat("d", 300)+`","handle":"`+strings.Repeat("h", 200)+`"},
				"record":{"text":"Post","createdAt":"2024-04-15T21:48:40.709Z"},
				"embed":{}
			}]}`), nil
		})},
	}

	result, err := fetcher.Fetch(context.Background(), postURL)
	require.NoError(t, err)
	assert.Len(t, result.Title, 300)
}

func TestSocialPostImageBudgetBoundsFetchesAndBytes(t *testing.T) {
	requests := 0
	fetcher := &Fetcher{
		logger:       log.New(io.Discard),
		assetsConfig: &assets.Config{},
		imageClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			requests++
			return response(http.StatusOK, "image/png", "bad"), nil
		})},
	}
	budget := socialPostImageBudget{bytesRemaining: 6, fetchesRemaining: 2}

	assert.Nil(t, fetcher.downloadSocialPostImage(context.Background(), "https://example.com/one.png", &budget))
	assert.Nil(t, fetcher.downloadSocialPostImage(context.Background(), "https://example.com/two.png", &budget))
	assert.Nil(t, fetcher.downloadSocialPostImage(context.Background(), "https://example.com/three.png", &budget))
	assert.Equal(t, 2, requests)
	assert.Zero(t, budget.fetchesRemaining)
	assert.Zero(t, budget.bytesRemaining)
}

func TestFetchResultPopulatesCompatibilityImage(t *testing.T) {
	asset := &corev1.AssetRecord{
		Id:      "preview_asset",
		Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: "preview_asset"}},
	}
	result := &FetchResult{
		ImageAsset: asset,
		SocialPost: &corev1.SocialPostPreview{
			Provider: "bluesky",
			Author:   &corev1.SocialPostAuthor{Handle: "bsky.app"},
		},
	}

	preview := result.ToProto("https://bsky.app/profile/bsky.app/post/example")
	assert.Equal(t, "preview_asset", preview.GetImageAssetId())
	require.NotNil(t, preview.GetImageAsset())
	assert.Equal(t, "preview_asset", preview.GetImageAsset().GetId())
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
