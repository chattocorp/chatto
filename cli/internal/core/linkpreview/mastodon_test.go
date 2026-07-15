// SPDX-FileCopyrightText: 2026 Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package linkpreview

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/assets"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestFetchMastodonStatusUsesProviderNeutralSnapshot(t *testing.T) {
	const postURL = "https://social.example/@alice/123"
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
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/oembed":
				assert.Equal(t, postURL, req.URL.Query().Get("url"))
				return response(http.StatusOK, "application/json", `{"type":"rich"}`), nil
			case "/api/v1/statuses/123":
				return response(http.StatusOK, "application/json", `{
					"id":"123",
					"url":"`+postURL+`",
					"created_at":"2026-07-15T18:00:00.000Z",
					"visibility":"public",
					"content":"<p class=\"quote-inline\">RE: duplicate quote link</p><p>Hello &amp; welcome<br>Second line</p>",
					"spoiler_text":"Plot spoilers",
					"account":{"display_name":"Alice","acct":"alice","avatar_static":"https://cdn.example/alice.png"},
					"media_attachments":[{"type":"image","url":"https://cdn.example/photo.png","description":"A landscape","meta":{"original":{"width":1200,"height":800}}}],
					"card":{"url":"https://example.com/story","title":"A story","description":"Story summary","image":"https://cdn.example/card.png"},
					"quote":{"state":"accepted","quoted_status":{
						"id":"456",
						"url":"https://remote.example/@bob/456",
						"created_at":"2026-07-15T17:00:00.000Z",
						"visibility":"unlisted",
						"content":"<p>Quoted words</p>",
						"account":{"display_name":"Bob","acct":"bob@remote.example","avatar_static":"https://cdn.example/bob.png"},
						"media_attachments":[{"type":"image","url":"https://cdn.example/quote.png","description":"Quoted attachment"}],
						"quote":{"state":"accepted","quoted_status":{
							"id":"789","url":"https://third.example/@carol/789","visibility":"public",
							"content":"<p>Too deep</p>","account":{"display_name":"Carol","acct":"carol@third.example"}
						}}
					}}
				}`), nil
			default:
				t.Fatalf("unexpected metadata request %q", req.URL.String())
				return nil, nil
			}
		})},
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

	result, err := fetcher.Fetch(context.Background(), postURL)
	require.NoError(t, err)
	assert.Equal(t, "Alice (@alice@social.example)", result.Title)
	assert.Equal(t, "Hello & welcome\nSecond line", result.Description)
	assert.Equal(t, "Mastodon", result.SiteName)
	assert.Equal(t, "mastodon", result.EmbedType)
	assert.Equal(t, "123", result.EmbedID)
	require.NotNil(t, result.SocialPost)
	assert.Equal(t, "mastodon", result.SocialPost.Provider)
	assert.Equal(t, "alice@social.example", result.SocialPost.GetAuthor().GetHandle())
	assert.Equal(t, "Plot spoilers", result.SocialPost.GetContentWarning())
	require.Len(t, result.SocialPost.Images, 1)
	assert.Equal(t, "A landscape", result.SocialPost.Images[0].Alt)
	assert.Equal(t, uint32(1200), result.SocialPost.Images[0].Width)
	assert.Equal(t, "asset-1", result.SocialPost.Images[0].GetAsset().GetId())
	require.NotNil(t, result.SocialPost.ExternalLink)
	assert.Equal(t, "https://example.com/story", result.SocialPost.ExternalLink.Url)
	assert.Equal(t, "asset-2", result.SocialPost.ExternalLink.GetImageAsset().GetId())
	assert.Equal(t, "asset-1", result.ImageAsset.GetId())

	quote := result.SocialPost.QuotedPost
	require.NotNil(t, quote)
	assert.Equal(t, "https://remote.example/@bob/456", quote.Url)
	assert.Equal(t, "bob@remote.example", quote.GetAuthor().GetHandle())
	assert.Equal(t, "Quoted words", quote.Text)
	require.Len(t, quote.Images, 1)
	assert.Equal(t, "Quoted attachment", quote.Images[0].Alt)
	assert.Nil(t, quote.QuotedPost)
	assert.True(t, proto.Equal(result.SocialPost, result.ToProto(postURL).GetSocialPost()))
}

func TestFetchMastodonBoostRendersOriginalStatus(t *testing.T) {
	const postURL = "https://social.example/@alice/123"
	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/api/oembed" {
				return response(http.StatusOK, "application/json", `{"type":"rich"}`), nil
			}
			return response(http.StatusOK, "application/json", `{
				"id":"123","url":"`+postURL+`","visibility":"public",
				"account":{"display_name":"Alice","acct":"alice"},
				"reblog":{
					"id":"456","url":"https://remote.example/@bob/456","visibility":"public",
					"content":"<p>Boosted words</p>","account":{"display_name":"Bob","acct":"bob@remote.example"}
				}
			}`), nil
		})},
	}

	result, err := fetcher.Fetch(context.Background(), postURL)
	require.NoError(t, err)
	assert.Equal(t, "Boosted words", result.SocialPost.Text)
	assert.Equal(t, "Bob", result.SocialPost.GetAuthor().GetDisplayName())
	assert.Equal(t, "https://remote.example/@bob/456", result.SocialPost.Url)
}

func TestFetchMastodonFallsBackToOpenGraphWhenDiscoveryFails(t *testing.T) {
	const postURL = "https://social.example/@alice/123"
	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/api/oembed" {
				return response(http.StatusNotFound, "application/json", `{}`), nil
			}
			assert.Equal(t, postURL, req.URL.String())
			return response(http.StatusOK, "text/html", `<meta property="og:title" content="Fallback title">`), nil
		})},
	}

	result, err := fetcher.Fetch(context.Background(), postURL)
	require.NoError(t, err)
	assert.Equal(t, "Fallback title", result.Title)
	assert.Equal(t, "generic", result.EmbedType)
}

func TestFetchMastodonDoesNotFallbackForPrivateStatus(t *testing.T) {
	const postURL = "https://social.example/@alice/123"
	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/api/oembed" {
				return response(http.StatusOK, "application/json", `{"type":"rich"}`), nil
			}
			if req.URL.Path == "/api/v1/statuses/123" {
				return response(http.StatusOK, "application/json", `{
					"id":"123","url":"`+postURL+`","visibility":"private",
					"content":"<p>Private words</p>","account":{"display_name":"Alice","acct":"alice"}
				}`), nil
			}
			t.Fatalf("private status must not fall back to OpenGraph: %s", req.URL)
			return nil, nil
		})},
	}

	_, err := fetcher.Fetch(context.Background(), postURL)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestMastodonHTMLTextPreservesBlocksAndOmitsQuoteCompatibilityLink(t *testing.T) {
	assert.Equal(t,
		"Hello world\nA full URL: https://example.com/path",
		mastodonHTMLText(`<p class="quote-inline">RE: <a href="https://quoted.example">quoted</a></p><p>Hello <strong>world</strong></p><p>A full URL: <a><span>https://example.com</span><span>/path</span></a></p>`),
	)
}
