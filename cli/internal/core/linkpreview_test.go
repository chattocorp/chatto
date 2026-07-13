package core

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestGetLinkPreviewPromotesCachedLegacyNATSImage(t *testing.T) {
	ctx := context.Background()
	core, _ := setupTestCore(t)
	assetID := NewAssetID()
	url := "https://legacy-preview.example/article"
	data := []byte("legacy-preview-image")

	_, err := core.storage.serverAssets.Put(ctx, jetstream.ObjectMeta{
		Name:        assetID,
		Description: "legacy preview",
		Headers: map[string][]string{
			"Content-Type":  {"image/webp"},
			"X-Legacy-Test": {"preserved"},
		},
		Metadata: map[string]string{"source": "legacy-cache"},
	}, bytes.NewReader(data))
	require.NoError(t, err)
	require.NoError(t, core.linkPreviewCache.Set(ctx, url, &corev1.LinkPreview{
		Url:          url,
		ImageAssetId: &assetID,
		ImageAsset: &corev1.AssetRecord{
			Id:          assetID,
			ContentType: "image/webp",
			Storage:     &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: assetID}},
		},
	}))

	preview, err := core.GetLinkPreview(ctx, url)
	require.NoError(t, err)
	require.Equal(t, assetID, preview.GetImageAssetId())

	info, err := core.storage.serverAssets.GetInfo(ctx, assetID)
	require.NoError(t, err)
	require.Equal(t, ServerAssetVisibilityPublic, info.Headers.Get(ServerAssetVisibilityHeader))
	require.Equal(t, "image/webp", info.Headers.Get("Content-Type"))
	require.Equal(t, "preserved", info.Headers.Get("X-Legacy-Test"))
	require.Equal(t, "legacy preview", info.Description)
	require.Equal(t, "legacy-cache", info.Metadata["source"])
	require.True(t, core.IsPublicServerAsset(ctx, assetID))

	stored, err := core.storage.serverAssets.GetBytes(ctx, assetID)
	require.NoError(t, err)
	require.Equal(t, data, stored)
}

func TestGetLinkPreviewDoesNotPromotePrivateCachedNATSImage(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string][]string
		declare func(t *testing.T, core *ChattoCore, assetID string)
	}{
		{name: "room metadata", headers: map[string][]string{"Room-Id": {"Rprivatepreview"}}},
		{name: "upload metadata", headers: map[string][]string{"Upload-Id": {"Uprivatepreview"}}},
		{name: "durable declaration", declare: func(t *testing.T, core *ChattoCore, assetID string) {
			event := newEvent(SystemActorID, &corev1.Event{
				Event: &corev1.Event_AssetCreated{AssetCreated: &corev1.AssetCreatedEvent{
					Asset: &corev1.AssetRecord{Id: assetID},
				}},
			})
			_, err := core.AssetsProjector.AppendEventuallyAndWait(
				testContext(t), core.EventPublisher, events.AssetAggregate(assetID), event,
			)
			require.NoError(t, err)
		}},
		{name: "durable tombstone", declare: func(t *testing.T, core *ChattoCore, assetID string) {
			event := newEvent(SystemActorID, &corev1.Event{
				Event: &corev1.Event_AssetDeleted{AssetDeleted: &corev1.AssetDeletedEvent{
					AssetId: assetID,
				}},
			})
			_, err := core.AssetsProjector.AppendEventuallyAndWait(
				testContext(t), core.EventPublisher, events.AssetAggregate(assetID), event,
			)
			require.NoError(t, err)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			core, _ := setupTestCore(t)
			assetID := NewAssetID()
			url := "https://private-preview.example/" + assetID
			headers := map[string][]string{"Content-Type": {"image/webp"}}
			for name, values := range test.headers {
				headers[name] = values
			}

			_, err := core.storage.serverAssets.Put(ctx, jetstream.ObjectMeta{
				Name:    assetID,
				Headers: headers,
			}, bytes.NewReader([]byte("private-image")))
			require.NoError(t, err)
			if test.declare != nil {
				test.declare(t, core, assetID)
			}
			require.NoError(t, core.linkPreviewCache.Set(ctx, url, &corev1.LinkPreview{
				Url:          url,
				ImageAssetId: &assetID,
				ImageAsset: &corev1.AssetRecord{
					Id:      assetID,
					Storage: &corev1.AssetRecord_Nats{Nats: &corev1.NATSAsset{Key: assetID}},
				},
			}))

			preview, err := core.GetLinkPreview(ctx, url)
			require.NoError(t, err)
			require.Equal(t, assetID, preview.GetImageAssetId())
			info, err := core.storage.serverAssets.GetInfo(ctx, assetID)
			require.NoError(t, err)
			require.Empty(t, info.Headers.Get(ServerAssetVisibilityHeader))
			require.False(t, core.IsPublicServerAsset(ctx, assetID))
		})
	}
}

func TestLinkPreviewImageStorageAndRetrieval(t *testing.T) {
	ctx := context.Background()
	core, _ := setupTestCore(t)

	restoreLocalhost := linkpreview.AllowLocalhostForTesting()
	defer restoreLocalhost()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/article":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<html>
<head>
<meta property="og:title" content="Local Link Preview">
<meta property="og:description" content="A hermetic preview fixture">
<meta property="og:image" content="` + serverURL + `/preview.png">
</head>
<body>hello</body>
</html>`))
		case "/preview.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(createTestPNG(64, 64))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	url := server.URL + "/article"

	preview, err := core.GetLinkPreview(ctx, url)
	require.NoError(t, err, "GetLinkPreview should succeed")
	require.NotNil(t, preview, "Preview should not be nil")

	t.Logf("Title: %s", preview.Title)
	t.Logf("Description: %s", preview.Description)
	t.Logf("ImageAssetId: %s", preview.GetImageAssetId())

	require.Equal(t, "Local Link Preview", preview.Title)
	require.NotEmpty(t, preview.GetImageAssetId(), "ImageAssetId should not be empty")
	require.NotNil(t, preview.GetImageAsset(), "ImageAsset should be populated")
	require.Equal(t, preview.GetImageAssetId(), preview.GetImageAsset().GetId())
	require.Equal(t, "image/webp", preview.GetImageAsset().GetContentType())
	require.NotNil(t, preview.GetImageAsset().GetNats(), "NATS-backed preview should carry NATS storage pointer")
	objectKey := PublicServerAssetObjectKey(preview.GetImageAssetId())
	require.Equal(t, objectKey, preview.GetImageAsset().GetNats().GetKey())
	storedInfo, err := core.storage.serverAssets.GetInfo(ctx, objectKey)
	require.NoError(t, err)
	require.Empty(t, storedInfo.Headers.Get(ServerAssetVisibilityHeader), "new public namespace should not need a visibility marker")
	require.True(t, core.IsPublicServerAsset(ctx, preview.GetImageAssetId()))

	idOnlyPreview := &corev1.LinkPreview{Url: url, ImageAssetId: preview.ImageAssetId}
	require.NoError(t, core.HydrateLinkPreviewImageAsset(ctx, idOnlyPreview))
	require.NotNil(t, idOnlyPreview.GetImageAsset(), "ID-only preview should be hydrated with ImageAsset")
	require.Equal(t, preview.GetImageAsset().GetId(), idOnlyPreview.GetImageAsset().GetId())
	require.NotNil(t, idOnlyPreview.GetImageAsset().GetNats(), "hydrated preview should carry NATS storage pointer")

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "Previews", "Preview discussion")
	require.NoError(t, err)
	user, err := core.CreateUser(ctx, "system", "previewposter", "previewposter", "password123")
	require.NoError(t, err)
	_, err = core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)

	postPreview := &corev1.LinkPreview{
		Url:          url,
		Title:        preview.GetTitle(),
		ImageAssetId: preview.ImageAssetId,
	}
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "preview", nil, "", "", postPreview, false)
	require.NoError(t, err)
	body, err := core.GetFullMessageBodyByEventID(ctx, event.GetId())
	require.NoError(t, err)
	require.NotNil(t, body)
	require.NotNil(t, body.LinkPreview.GetImageAsset(), "posted ID-only preview should be stored with ImageAsset")
	require.Equal(t, preview.GetImageAssetId(), body.LinkPreview.GetImageAsset().GetId())
	require.NotNil(t, body.LinkPreview.GetImageAsset().GetNats())

	// Now try to retrieve the stored image
	reader, info, err := core.GetServerAssetFromAnyBackend(ctx, preview.GetImageAssetId())
	require.NoError(t, err, "GetServerAssetFromAnyBackend should succeed")
	require.NotNil(t, reader, "Reader should not be nil")

	t.Logf("Content-Type: %s", info.ContentType)
	t.Logf("Size: %d", info.Size)

	require.Equal(t, "image/webp", info.ContentType, "Content type should be image/webp")
	require.Greater(t, info.Size, int64(0), "Size should be greater than 0")

	// Read the data to verify it's valid
	data, err := io.ReadAll(reader)
	require.NoError(t, err, "Reading asset data should succeed")
	require.Greater(t, len(data), 0, "Data should not be empty")

	t.Logf("Read %d bytes of image data", len(data))

	// Verify it starts with WebP signature (RIFF....WEBP)
	require.True(t, len(data) >= 12, "Data should be at least 12 bytes")
	require.Equal(t, "RIFF", string(data[0:4]), "Should start with RIFF")
	require.Equal(t, "WEBP", string(data[8:12]), "Should have WEBP magic number")
}

func TestLinkPreviewImageUsesS3WhenConfigured(t *testing.T) {
	ctx := context.Background()
	core, _, s3Client, rawS3Client, _ := setupTestCoreWithS3PathPrefix(t, "tenant-a/chatto")

	restoreLocalhost := linkpreview.AllowLocalhostForTesting()
	defer restoreLocalhost()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/article":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<html>
<head>
<meta property="og:title" content="S3 Link Preview">
<meta property="og:description" content="A hermetic S3 preview fixture">
<meta property="og:image" content="` + serverURL + `/preview.png">
</head>
<body>hello</body>
</html>`))
		case "/preview.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(createTestPNG(64, 64))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	preview, err := core.GetLinkPreview(ctx, server.URL+"/article")
	require.NoError(t, err)
	require.NotNil(t, preview)
	require.Equal(t, "S3 Link Preview", preview.Title)
	require.NotEmpty(t, preview.GetImageAssetId())
	require.NotNil(t, preview.GetImageAsset(), "ImageAsset should be populated")
	require.Equal(t, preview.GetImageAssetId(), preview.GetImageAsset().GetId())
	require.Equal(t, "image/webp", preview.GetImageAsset().GetContentType())
	require.NotNil(t, preview.GetImageAsset().GetS3(), "S3-backed preview should carry S3 storage pointer")
	require.Equal(t, preview.GetImageAssetId(), preview.GetImageAsset().GetS3().GetKey())

	_, err = core.storage.serverAssets.Get(ctx, preview.GetImageAssetId())
	require.Error(t, err, "link preview image should not be stored in SERVER_ASSETS when S3 is configured")

	logicalS3Key := S3KeyServerAsset(preview.GetImageAssetId())
	if _, err := s3Client.StatObject(ctx, logicalS3Key); err != nil {
		t.Fatalf("logical S3 stat failed: %v", err)
	}
	if _, err := rawS3Client.StatObject(ctx, "tenant-a/chatto/"+logicalS3Key); err != nil {
		t.Fatalf("raw physical S3 stat failed: %v", err)
	}

	reader, info, err := core.GetServerAssetFromAnyBackend(ctx, preview.GetImageAssetId())
	require.NoError(t, err)
	require.NotNil(t, reader)
	require.Equal(t, "image/webp", info.ContentType)
}
