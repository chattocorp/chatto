package linkpreview

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"
)

func TestLogErrorKind(t *testing.T) {
	for _, scenario := range []struct {
		name string
		err  error
		want string
	}{
		{"canceled", fmt.Errorf("private-error: %w", context.Canceled), "canceled"},
		{"deadline", fmt.Errorf("private-error: %w", context.DeadlineExceeded), "timeout"},
		{"DNS timeout", &net.DNSError{Name: "private-host", Err: "private-error", IsTimeout: true}, "timeout"},
		{"HTTP failure", &url.Error{Op: "Get", URL: "https://private-host/private-path", Err: errors.New("private-error")}, "network"},
		{"unknown", errors.New("private-error"), "failed"},
		{"joined", errors.Join(errors.New("private-error"), context.Canceled), "canceled"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			require.Equal(t, scenario.want, LogErrorKind(scenario.err))
		})
	}
}

func TestPreviewFailureLogsExcludeSensitiveData(t *testing.T) {
	for _, level := range []log.Level{log.DebugLevel, log.WarnLevel} {
		for _, scenario := range []struct {
			name, target, message string
			socialImage           bool
		}{
			{"page", "https://private-user:private-password@preview.example/private-path?token=private-query", "Failed to fetch page", false},
			{"bluesky", "https://bsky.app/profile/private-user/post/private-path?token=private-query", "Failed to fetch Bluesky oEmbed metadata", false},
			{"mastodon", "https://preview.example/@private-user/123?token=private-query", "Failed to fetch Mastodon status metadata", false},
			{"social image", "https://private-user:private-password@preview.example/private-path?token=private-query", "Failed to persist social-post image", true},
		} {
			t.Run(fmt.Sprintf("%s/%s", level, scenario.name), func(t *testing.T) {
				var output bytes.Buffer
				logger := log.NewWithOptions(&output, log.Options{Level: level})
				client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("wrapped: %w", &url.Error{
						Op: "Get", URL: req.URL.String(),
						Err: errors.New("private-error from 10.23.45.67"),
					})
				})}
				fetcher := &Fetcher{logger: logger, httpClient: client, imageClient: client}
				if scenario.socialImage {
					budget := &socialPostImageBudget{bytesRemaining: MaxSocialPostImageBytes, fetchesRemaining: 1}
					require.Nil(t, fetcher.downloadSocialPostImage(context.Background(), scenario.target, budget))
				} else {
					_, err := fetcher.Fetch(context.Background(), scenario.target)
					require.Error(t, err)
				}
				require.Contains(t, output.String(), scenario.message)
				assertPreviewLogsPrivate(t, output.String())
			})
		}
	}
}

func TestPreviewMetadataAndImageLogsExcludeSensitiveData(t *testing.T) {
	for _, imageFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("image transport failure=%t", imageFailure), func(t *testing.T) {
			var output bytes.Buffer
			fetcher := &Fetcher{
				logger: log.NewWithOptions(&output, log.Options{Level: log.DebugLevel}),
				httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return response(http.StatusOK, "text/html", `<html><head>
<meta property="og:title" content="private-title">
<meta property="og:description" content="private-description">
<meta property="og:site_name" content="private-site">
<meta property="og:image" content="https://private-user:private-password@preview.example/private-path?token=private-query">
</head></html>`), nil
				})},
				imageClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					if imageFailure {
						return nil, fmt.Errorf("private-error fetching %s", req.URL)
					}
					result := response(http.StatusOK, "private-content-type", "")
					result.Header.Set("Content-Length", "private-content-length")
					return result, nil
				})},
			}
			result, err := fetcher.Fetch(context.Background(), "https://preview.example/private-path?token=private-query")
			require.NoError(t, err)
			require.Equal(t, "private-title", result.Title)
			require.Equal(t, "private-description", result.Description)
			require.Equal(t, "private-site", result.SiteName)
			require.Nil(t, result.ImageAsset)
			require.Contains(t, output.String(), "Fetched OG metadata")
			require.Contains(t, output.String(), "Failed to download preview image")
			if !imageFailure {
				require.Contains(t, output.String(), "Image fetch response")
			}
			assertPreviewLogsPrivate(t, output.String())
		})
	}
}

func assertPreviewLogsPrivate(t *testing.T, output string) {
	t.Helper()
	for _, secret := range []string{"private-", "preview.example", "bsky.app", "10.23.45.67"} {
		require.NotContains(t, output, secret)
	}
}
