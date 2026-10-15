package core

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/core/linkpreview"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

type previewLoggingFailureKV struct {
	jetstream.KeyValue
	err error
}

func (kv previewLoggingFailureKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	return nil, kv.err
}

func (kv previewLoggingFailureKV) Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, kv.err
}

func TestLinkPreviewCacheLogsExcludeSensitiveData(t *testing.T) {
	t.Parallel()
	const target = "https://www.youtube.com/watch?v=dQw4w9WgXcQ&token=private-query"
	var output bytes.Buffer
	c := &ChattoCore{
		logger: log.NewWithOptions(&output, log.Options{Level: log.DebugLevel}),
		linkPreviewCache: linkpreview.NewCache(previewLoggingFailureKV{
			err: fmt.Errorf("private-error for %s from 10.23.45.67", target),
		}),
		linkPreviewFetcher: linkpreview.NewFetcher(nil, nil, nil),
	}
	preview, err := c.GetLinkPreview(context.Background(), target)
	require.NoError(t, err)
	require.Equal(t, "youtube", preview.GetEmbedType())
	require.Contains(t, output.String(), "Failed to get cached link preview")
	require.Contains(t, output.String(), "Failed to cache link preview")

	// An oversized URL still reaches fetched-preview validation through the
	// local YouTube shortcut; no network access is needed.
	preview, err = c.GetLinkPreview(context.Background(), target+strings.Repeat("x", MaxLinkPreviewURLLength))
	require.NoError(t, err)
	require.Nil(t, preview)
	require.Contains(t, output.String(), "Discarding invalid fetched link preview")

	// An invalid asset key stops backend lookup after the cache warning.
	err = c.HydrateLinkPreviewImageAsset(context.Background(), &evtv1.LinkPreview{
		Url: target, ImageAssetId: new("/"),
	})
	require.Error(t, err)
	require.Contains(t, output.String(), "Failed to hydrate link preview image asset from cache")

	for _, secret := range []string{"private-", "youtube.com", "10.23.45.67"} {
		require.NotContains(t, output.String(), secret)
	}
}
