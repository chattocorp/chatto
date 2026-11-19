package core

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

func TestChattoCore_ServerBrandingUsesConfigEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	assetID := NewAssetID()
	logo := &evtv1.AssetRecord{
		Id:          assetID,
		Filename:    "logo.webp",
		ContentType: "image/webp",
		Storage:     &evtv1.AssetRecord_Nats{Nats: &evtv1.NATSAsset{Key: assetID}},
	}
	if err := core.SetServerLogo(ctx, "admin", logo); err != nil {
		t.Fatalf("SetServerLogo failed: %v", err)
	}

	got, err := core.GetServerLogo(ctx)
	if err != nil {
		t.Fatalf("GetServerLogo failed: %v", err)
	}
	if !proto.Equal(logo, got) {
		t.Fatalf("GetServerLogo = %+v, want %+v", got, logo)
	}
	cfg := core.ConfigModel().GetServerConfig()
	if cfg != nil {
		t.Fatalf("logo-only update wrote server config: cfg=%+v", cfg)
	}
	blocked := core.ConfigModel().GetEffectiveBlockedUsernames()
	if blocked != DefaultBlockedUsernames {
		t.Fatalf("logo-only update changed effective blocked usernames: got %q", blocked)
	}

	msgs := eventStreamMsgCount(t, core)
	if err := core.SetServerLogo(ctx, "admin", logo); err != nil {
		t.Fatalf("SetServerLogo same value failed: %v", err)
	}
	if after := eventStreamMsgCount(t, core); after != msgs {
		t.Fatalf("same logo write published events: before %d after %d", msgs, after)
	}

	if err := core.DeleteServerLogo(ctx, "admin"); err != nil {
		t.Fatalf("DeleteServerLogo failed: %v", err)
	}
	got, err = core.GetServerLogo(ctx)
	if err != nil {
		t.Fatalf("GetServerLogo after delete failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected logo to be cleared, got %+v", got)
	}
}

// Stop the writing replica after the config transaction commits and before
// its source can hand off to JOBS. The survivor runs normal core services.
func TestServerBrandingCleanupAfterWriterCrash(t *testing.T) {
	ns, conn := testutil.StartNATS(t)
	ctx := testContext(t)
	cfg := testCoreConfig()
	first, err := NewChattoCore(ctx, conn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	projector := first.configModel.config.Projector()
	stop := testutil.StartService(t, "config projection", projector.Run, projector.WaitForStartup)
	upload := func(kind string) *evtv1.AssetRecord {
		t.Helper()
		asset, err := first.uploadServerAsset(ctx, []byte(kind), kind)
		if err != nil {
			t.Fatal(err)
		}
		require.Equal(t, PublicServerAssetObjectKey(asset.GetId()), asset.GetNats().GetKey())
		for _, key := range []string{asset.GetId(), asset.GetNats().GetKey()} {
			if err := first.StoreCachedResize(ctx, ImageCacheKey(ServerAssetSignResource, key, 64, 64, "cover"), []byte(kind)); err != nil {
				t.Fatal(err)
			}
		}
		return asset
	}
	old, current, banner := upload("logo"), upload("logo"), upload("banner")
	if err := first.SetServerLogo(ctx, "admin", old); err != nil {
		t.Fatal(err)
	}
	if err := first.SetServerBanner(ctx, "admin", banner); err != nil {
		t.Fatal(err)
	}
	if err := first.SetServerLogo(ctx, "admin", current); err != nil {
		t.Fatal(err)
	}
	if err := first.DeleteServerBanner(ctx, "admin"); err != nil {
		t.Fatal(err)
	}
	for _, asset := range []*evtv1.AssetRecord{old, banner} {
		if _, err := first.storage.serverAssets.GetInfo(ctx, asset.GetNats().GetKey()); err != nil {
			t.Fatalf("writer removed retired object before handoff: %v", err)
		}
	}
	// For the banner, leave the storage state of an interrupted cleanup
	// that deleted the binary before it could delete cached sizes.
	require.NoError(t, first.storage.serverAssets.Delete(ctx, banner.GetNats().GetKey()))
	stop()
	conn.Close()

	second := startReplica(t, ns, cfg)
	require.Eventually(t, func() bool {
		jobs := jobsOfType(t, ctx, second, jobTypeBrandingCleanup)
		return len(jobs) == 2 && jobs[0].Status == jobqueue.Succeeded && jobs[1].Status == jobqueue.Succeeded
	}, 5*time.Second, 10*time.Millisecond)
	for _, asset := range []*evtv1.AssetRecord{old, banner} {
		waitForAssetObjectDeleted(t, ctx, second.storage.serverAssets, asset.GetNats().GetKey())
	}
	got, err := second.GetServerLogo(ctx)
	if err != nil || !proto.Equal(got, current) {
		t.Fatalf("current logo = %v, %v", got, err)
	}
	if data, err := second.storage.serverAssets.GetBytes(ctx, current.GetNats().GetKey()); err != nil || !bytes.Equal(data, []byte("logo")) {
		t.Fatalf("surviving logo = %q, %v", data, err)
	}
	for _, asset := range []*evtv1.AssetRecord{old, current, banner} {
		for _, key := range []string{asset.GetId(), asset.GetNats().GetKey()} {
			data, err := second.GetCachedResize(ctx, ImageCacheKey(ServerAssetSignResource, key, 64, 64, "cover"))
			if err != nil || (data != nil) != (asset == current) {
				t.Fatalf("cache for %s = %q, %v", key, data, err)
			}
		}
	}
}

func TestServerBrandingCleanupDeletesS3Object(t *testing.T) {
	core, _, storage := setupTestCoreWithS3(t)
	ctx := testContext(t)
	logo, err := core.uploadServerAsset(ctx, []byte("logo"), "logo")
	require.NoError(t, err)
	key := S3KeyServerAsset(logo.GetId())
	_, err = storage.StatObject(ctx, key)
	require.NoError(t, err)
	require.NoError(t, core.SetServerLogo(ctx, "admin", logo))
	require.NoError(t, core.DeleteServerLogo(ctx, "admin"))
	require.Eventually(t, func() bool {
		jobs := jobsOfType(t, ctx, core, jobTypeBrandingCleanup)
		return len(jobs) == 1 && jobs[0].Status == jobqueue.Succeeded
	}, 5*time.Second, 10*time.Millisecond)
	_, err = storage.StatObject(ctx, key)
	require.True(t, IsNoSuchKeyError(err), "expected deleted S3 object, got %v", err)
}

func TestBrandingCleanupRejectsMismatchedStorageKey(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	logo, err := core.uploadServerAsset(ctx, []byte("keep"), "logo")
	require.NoError(t, err)
	// Store an inconsistent historical reference. Its key identifies the
	// live object but its asset ID identifies a different upload.
	invalid := cloneAssetRecord(logo)
	invalid.Id = NewAssetID()
	require.NoError(t, core.SetServerLogo(ctx, "admin", logo))
	require.NoError(t, core.SetServerBanner(ctx, "admin", invalid))
	require.NoError(t, core.DeleteServerBanner(ctx, "admin"))
	require.Eventually(t, func() bool {
		jobs := jobsOfType(t, ctx, core, jobTypeBrandingCleanup)
		return len(jobs) == 1 && jobs[0].Status == jobqueue.Failed && jobs[0].Attempts == 1
	}, 5*time.Second, 10*time.Millisecond)
	data, err := core.storage.serverAssets.GetBytes(ctx, logo.GetNats().GetKey())
	require.NoError(t, err)
	require.Equal(t, []byte("keep"), data)
}
