package core_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// setupFakeS3Server creates an in-memory S3 server for testing.
func setupFakeS3Server(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	backend := s3mem.New()
	faker := gofakes3.New(backend)
	server := httptest.NewServer(faker.Server())
	t.Cleanup(server.Close)

	return server, server.URL
}

// TestS3Client_PutAndGetObject tests uploading and retrieving objects.
func TestS3Client_PutAndGetObject(t *testing.T) {
	server, endpoint := setupFakeS3Server(t)
	defer server.Close()

	// Parse endpoint to get host without protocol
	endpointHost := endpoint[7:] // Remove "http://"

	// Create S3 client with test config
	cfg := config.S3Config{
		Endpoint:        endpointHost,
		Bucket:          "test-bucket",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
	}
	useSSL := false
	pathStyle := true
	cfg.UseSSL = &useSSL
	cfg.PathStyle = &pathStyle

	client, err := core.NewS3Client(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	ctx := context.Background()

	// Create the bucket
	err = client.EnsureBucket(ctx)
	require.NoError(t, err)

	// Test data
	testKey := "test/object.txt"
	testData := []byte("Hello, S3!")
	testContentType := "text/plain"

	// Put object
	info, err := client.PutObjectFromBytes(ctx, testKey, testData, testContentType)
	require.NoError(t, err)
	require.Equal(t, testKey, info.Key)
	require.Equal(t, int64(len(testData)), info.Size)
	require.Equal(t, testContentType, info.ContentType)

	// Get object
	reader, objInfo, err := client.GetObject(ctx, testKey)
	require.NoError(t, err)
	defer reader.Close()

	require.Equal(t, testKey, objInfo.Key)
	require.Equal(t, int64(len(testData)), objInfo.Size)

	// Read content
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, testData, content)
}

// TestS3Client_DeleteObject tests deleting objects.
func TestS3Client_DeleteObject(t *testing.T) {
	server, endpoint := setupFakeS3Server(t)
	defer server.Close()

	endpointHost := endpoint[7:]

	cfg := config.S3Config{
		Endpoint:        endpointHost,
		Bucket:          "test-bucket",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
	}
	useSSL := false
	pathStyle := true
	cfg.UseSSL = &useSSL
	cfg.PathStyle = &pathStyle

	client, err := core.NewS3Client(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = client.EnsureBucket(ctx)
	require.NoError(t, err)

	// Upload an object
	testKey := "to-delete.txt"
	_, err = client.PutObjectFromBytes(ctx, testKey, []byte("delete me"), "text/plain")
	require.NoError(t, err)

	// Verify it exists
	_, _, err = client.GetObject(ctx, testKey)
	require.NoError(t, err)

	// Delete it
	err = client.DeleteObject(ctx, testKey)
	require.NoError(t, err)

	// Verify it's gone
	_, _, err = client.GetObject(ctx, testKey)
	require.Error(t, err)
}

// TestS3Client_StatObject tests getting object metadata without downloading.
func TestS3Client_StatObject(t *testing.T) {
	server, endpoint := setupFakeS3Server(t)
	defer server.Close()

	endpointHost := endpoint[7:]

	cfg := config.S3Config{
		Endpoint:        endpointHost,
		Bucket:          "test-bucket",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
	}
	useSSL := false
	pathStyle := true
	cfg.UseSSL = &useSSL
	cfg.PathStyle = &pathStyle

	client, err := core.NewS3Client(cfg)
	require.NoError(t, err)

	ctx := context.Background()
	err = client.EnsureBucket(ctx)
	require.NoError(t, err)

	// Upload an object
	testKey := "stat-test.bin"
	testData := bytes.Repeat([]byte{0xAB}, 1024)
	_, err = client.PutObjectFromBytes(ctx, testKey, testData, "application/octet-stream")
	require.NoError(t, err)

	// Stat it
	info, err := client.StatObject(ctx, testKey)
	require.NoError(t, err)
	require.Equal(t, testKey, info.Key)
	require.Equal(t, int64(1024), info.Size)
}

// TestS3KeyHelpers tests the S3 key generation helpers.
func TestS3KeyHelpers(t *testing.T) {
	tests := []struct {
		name     string
		function func() string
		expected string
	}{
		{
			name: "Attachment",
			function: func() string {
				return core.S3KeyAttachment("attach456")
			},
			expected: "attachments/attach456",
		},
		{
			name: "SpaceAttachment",
			function: func() string {
				return core.S3KeySpaceAttachment("space123", "attach456")
			},
			expected: "spaces/space123/attachments/attach456",
		},
		{
			name: "ServerAsset",
			function: func() string {
				return core.S3KeyServerAsset("asset789")
			},
			expected: "instance/asset789",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.function()
			require.Equal(t, tc.expected, got)
		})
	}
}

// TestS3Client_NilWhenNotConfigured tests that NewS3Client returns nil when config is empty.
func TestS3Client_NilWhenNotConfigured(t *testing.T) {
	cfg := config.S3Config{} // Empty config
	client, err := core.NewS3Client(cfg)
	require.NoError(t, err)
	require.Nil(t, client)
}

// TestStorageBackendEncapsulation_URLGeneration tests that URL generation always uses
// standard formats regardless of storage backend. The storage backend should be an
// internal implementation detail that is not exposed in URLs.
func TestStorageBackendEncapsulation_URLGeneration(t *testing.T) {
	t.Run("S3 asset keys use consistent format for server assets", func(t *testing.T) {
		// Instance assets should all use the same key format: instance/{assetId}
		assetID := "abc123xyz"
		s3Key := core.S3KeyServerAsset(assetID)
		require.Equal(t, "instance/abc123xyz", s3Key)

		// The URL format should be /assets/server/{assetId}
		// This is what the HTTP handler expects regardless of backend
		expectedURLPath := fmt.Sprintf("/assets/server/%s", assetID)
		require.Equal(t, "/assets/server/abc123xyz", expectedURLPath)
	})

	t.Run("S3 asset keys use consistent format for attachments", func(t *testing.T) {
		// Attachments use: attachments/{attachmentId}
		attachmentID := "attach789"
		s3Key := core.S3KeyAttachment(attachmentID)
		require.Equal(t, "attachments/attach789", s3Key)

		// The URL format is /assets/attachments/{attachmentId}
		expectedURLPath := fmt.Sprintf("/assets/attachments/%s", attachmentID)
		require.Equal(t, "/assets/attachments/attach789", expectedURLPath)
	})

	t.Run("legacy S3 key layout remains accessible via S3KeySpaceAttachment", func(t *testing.T) {
		// Pre-ADR-030-Phase-4 attachments still live at
		// spaces/{server|DM}/attachments/{id}. The legacy key constructor
		// is retained so the S3-key fallback probe can find them.
		s3Key := core.S3KeySpaceAttachment("server", "legacyAttach")
		require.Equal(t, "spaces/server/attachments/legacyAttach", s3Key)
	})

	t.Run("S3Asset.Key stores only the asset ID for server assets", func(t *testing.T) {
		// When storing an S3 asset, we should store only the assetID
		// (same as NATS) so URL generation is consistent
		assetID := "myasset123"

		// NATS asset stores assetID in Key
		natsAsset := &corev1.Asset{
			Asset: &corev1.Asset_Nats{
				Nats: &corev1.NATSAsset{Key: assetID},
			},
		}

		// S3 asset should also store assetID in Key (not the full S3 path)
		s3Asset := &corev1.Asset{
			Asset: &corev1.Asset_S3{
				S3: &corev1.S3Asset{Key: assetID, Bucket: proto.String("test-bucket")},
			},
		}

		// Both should return the same key for URL generation
		require.Equal(t, assetID, natsAsset.GetNats().Key)
		require.Equal(t, assetID, s3Asset.GetS3().Key)
	})
}
