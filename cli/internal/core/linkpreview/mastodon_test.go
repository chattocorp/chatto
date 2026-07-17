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
				return response(http.StatusOK, "application/json", `{"type":"rich","provider_url":"https://social.example/","html":"<iframe class=\"mastodon-embed\" src=\"https://social.example/@alice/123/embed\"></iframe>"}`), nil
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
				return response(http.StatusOK, "application/json", `{"type":"rich","provider_url":"https://social.example/","html":"<iframe class=\"mastodon-embed\" src=\"https://social.example/@alice/123/embed\"></iframe>"}`), nil
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
			switch req.URL.Path {
			case "/api/oembed", "/api/v2/instance":
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

func TestFetchMastodonFederatedProxyUsesInstanceDiscovery(t *testing.T) {
	const postURL = "https://social.example/@alice@remote.example/123"
	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/api/oembed":
				return response(http.StatusNotFound, "application/json", `{}`), nil
			case "/api/v2/instance":
				return response(http.StatusOK, "application/json", `{
					"domain":"social.example",
					"source_url":"https://github.com/mastodon/mastodon",
					"api_versions":{"mastodon":11}
				}`), nil
			case "/api/v1/statuses/123":
				return response(http.StatusOK, "application/json", `{
					"id":"123",
					"url":"https://remote.example/@alice/456",
					"visibility":"public",
					"content":"<p>Federated words</p>",
					"account":{"display_name":"Alice","acct":"alice@remote.example"}
				}`), nil
			default:
				t.Fatalf("unexpected metadata request %q", req.URL.String())
				return nil, nil
			}
		})},
	}

	result, err := fetcher.Fetch(context.Background(), postURL)
	require.NoError(t, err)
	assert.Equal(t, "mastodon", result.EmbedType)
	assert.Equal(t, "Federated words", result.SocialPost.Text)
	assert.Equal(t, "https://remote.example/@alice/456", result.SocialPost.Url)
}

func TestFetchMastodonDoesNotFallbackForPrivateStatus(t *testing.T) {
	const postURL = "https://social.example/@alice/123"
	fetcher := &Fetcher{
		logger: log.New(io.Discard),
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/api/oembed" {
				return response(http.StatusOK, "application/json", `{"type":"rich","provider_url":"https://social.example/","html":"<iframe class=\"mastodon-embed\" src=\"https://social.example/@alice/123/embed\"></iframe>"}`), nil
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

func TestVerifyMastodonOEmbedRejectsCrossOriginMetadata(t *testing.T) {
	fetcher := &Fetcher{httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(http.StatusOK, "application/json", `{
			"type":"rich",
			"provider_url":"https://attacker.example/",
			"html":"<iframe class=\"mastodon-embed\" src=\"https://attacker.example/@alice/123/embed\"></iframe>"
		}`), nil
	})}}

	err := fetcher.verifyMastodonOEmbed(context.Background(), "https://social.example", "https://social.example/@alice/123")
	require.ErrorContains(t, err, "not bound")
}

func TestVerifyMastodonOEmbedRejectsGenericRichEmbed(t *testing.T) {
	fetcher := &Fetcher{httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(http.StatusOK, "application/json", `{
			"type":"rich",
			"provider_url":"https://social.example/",
			"html":"<iframe src=\"https://social.example/widget/embed\"></iframe>"
		}`), nil
	})}}

	err := fetcher.verifyMastodonOEmbed(context.Background(), "https://social.example", "https://social.example/@alice/123")
	require.ErrorContains(t, err, "not bound")
}

func TestVerifyMastodonOEmbedAcceptsCurrentBlockquoteMarkup(t *testing.T) {
	fetcher := &Fetcher{httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(http.StatusOK, "application/json", `{
			"type":"rich",
			"provider_url":"https://social.example/",
			"html":"<blockquote class=\"mastodon-embed\" data-embed-url=\"https://social.example/@alice/123/embed\"></blockquote>"
		}`), nil
	})}}

	require.NoError(t, fetcher.verifyMastodonOEmbed(
		context.Background(),
		"https://social.example",
		"https://social.example/@alice/123",
	))
}

func TestVerifyMastodonInstanceRejectsMismatchedDomain(t *testing.T) {
	fetcher := &Fetcher{httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(http.StatusOK, "application/json", `{
			"domain":"attacker.example",
			"source_url":"https://github.com/mastodon/mastodon",
			"api_versions":{"mastodon":11}
		}`), nil
	})}}

	err := fetcher.verifyMastodonInstance(context.Background(), "https://social.example")
	require.ErrorContains(t, err, "not bound")
}

func TestFetchMastodonStatusAcceptsFederatedCanonicalURL(t *testing.T) {
	fetcher := &Fetcher{httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(http.StatusOK, "application/json", `{
			"id":"123","url":"https://home.example/@alice/123","visibility":"public",
			"content":"<p>Federated</p>","account":{"display_name":"Alice","acct":"alice@home.example"}
		}`), nil
	})}}

	status, err := fetcher.fetchMastodonStatus(context.Background(), "https://social.example", "123")
	require.NoError(t, err)
	assert.Equal(t, "https://home.example/@alice/123", status.URL)
}

func TestFetchMastodonStatusAcceptsFederatedBoostCanonicalURL(t *testing.T) {
	fetcher := &Fetcher{httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(http.StatusOK, "application/json", `{
			"id":"123","url":"https://social.example/users/alice/statuses/123/activity","visibility":"public",
			"account":{"display_name":"Alice","acct":"alice"},
			"reblog":{"id":"456","url":"https://home.example/objects/01JABCDEF","visibility":"public","account":{"display_name":"Bob","acct":"bob@home.example"}}
		}`), nil
	})}}

	status, err := fetcher.fetchMastodonStatus(context.Background(), "https://social.example", "123")
	require.NoError(t, err)
	require.NotNil(t, status.Reblog)
	assert.Equal(t, "https://home.example/objects/01JABCDEF", status.Reblog.URL)
}

func TestFetchMastodonStatusRejectsInvalidCanonicalURL(t *testing.T) {
	fetcher := &Fetcher{httpClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return response(http.StatusOK, "application/json", `{
			"id":"123","url":"javascript:alert(1)","visibility":"public",
			"content":"<p>Mislabelled</p>","account":{"display_name":"Alice","acct":"alice"}
		}`), nil
	})}}

	_, err := fetcher.fetchMastodonStatus(context.Background(), "https://social.example", "123")
	require.ErrorContains(t, err, "invalid canonical URL")
}

func TestMastodonSensitiveStatusGetsContentWarning(t *testing.T) {
	fetcher := &Fetcher{}
	snapshot := fetcher.mastodonStatusSnapshot(context.Background(), &mastodonStatus{
		ID:         "123",
		URL:        "https://social.example/@alice/123",
		Visibility: "public",
		Sensitive:  true,
		Content:    "<p>Sensitive words</p>",
		Account:    mastodonAccount{DisplayName: "Alice", Acct: "alice"},
	}, "", "https://social.example", &socialPostImageBudget{}, 0)

	require.NotNil(t, snapshot)
	assert.Equal(t, "Sensitive content", snapshot.GetContentWarning())
}

func TestMastodonHTTPClientRejectsCrossOriginRedirect(t *testing.T) {
	fetcher := &Fetcher{httpClient: &http.Client{}}
	client := fetcher.mastodonHTTPClient("https://social.example")
	req, err := http.NewRequest(http.MethodGet, "https://attacker.example/api/oembed", nil)
	require.NoError(t, err)
	require.ErrorContains(t, client.CheckRedirect(req, nil), "changed instance origin")
}
