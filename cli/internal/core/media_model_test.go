package core

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/signedurl"
)

func TestNewMediaModelWiresCore(t *testing.T) {
	core := &ChattoCore{}

	service := NewMediaModel(core)

	if service.ChattoCore != core {
		t.Fatal("core facade was not wired")
	}
}

func TestChattoCoreMediaLazilyInitializesModel(t *testing.T) {
	core := &ChattoCore{}

	first := core.media()
	second := core.media()

	if first == nil {
		t.Fatal("media model was not initialized")
	}
	if first != second {
		t.Fatal("media model was not reused")
	}
	if core.mediaModel != first {
		t.Fatal("media model was not stored on core")
	}
	if first.ChattoCore != core {
		t.Fatal("media model does not point at its core facade")
	}
}

func TestMediaModelUploadAttachmentStoresAndProjectsAsset(t *testing.T) {
	core, _ := setupTestCore(t)
	service := core.mediaModel
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-service", "Media service")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	attachment, err := service.UploadAttachment(ctx, SystemActorID, room.Id, "note.txt", "text/plain", bytes.NewReader([]byte("hello media model")))
	if err != nil {
		t.Fatalf("UploadAttachment returned error: %v", err)
	}

	declared, ok := core.Assets.AssetCreation(attachment.GetId())
	if !ok {
		t.Fatal("asset creation was not projected")
	}
	if declared.GetAsset().GetId() != attachment.GetId() {
		t.Fatalf("projected asset id = %q, want %q", declared.GetAsset().GetId(), attachment.GetId())
	}
	assetEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(attachment.GetId()).Subject(events.EventAssetCreated))
	if err != nil {
		t.Fatalf("SubjectEvents(asset_created asset aggregate): %v", err)
	}
	if len(assetEvents) != 1 {
		t.Fatalf("asset aggregate asset_created events = %d, want 1", len(assetEvents))
	}
	roomAssetEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.RoomAggregate(room.Id).Subject(events.EventAssetCreated))
	if err != nil {
		t.Fatalf("SubjectEvents(asset_created room aggregate): %v", err)
	}
	if len(roomAssetEvents) != 0 {
		t.Fatalf("room aggregate asset_created events = %d, want 0", len(roomAssetEvents))
	}

	reader, info, err := service.GetAttachmentReader(ctx, attachment)
	if err != nil {
		t.Fatalf("GetAttachmentReader returned error: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll attachment: %v", err)
	}
	if string(data) != "hello media model" {
		t.Fatalf("stored attachment = %q, want %q", string(data), "hello media model")
	}
	if info.ContentType != "text/plain" {
		t.Fatalf("content type = %q, want %q", info.ContentType, "text/plain")
	}
}

func TestMediaModelUploadAttachmentRequiresActor(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.mediaModel.UploadAttachment(ctx, "", "R-missing", "note.txt", "text/plain", bytes.NewReader([]byte("body")))
	if err == nil {
		t.Fatal("UploadAttachment with missing actor returned nil error")
	}
	if !strings.Contains(err.Error(), "upload missing actor id") {
		t.Fatalf("UploadAttachment error = %q, want missing actor message", err.Error())
	}
}

func TestMediaModelUploadDerivativeAttachmentProjectsParentage(t *testing.T) {
	core, _ := setupTestCore(t)
	service := core.mediaModel
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-derivative", "Media derivative")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	derivative, err := service.UploadDerivativeAttachment(
		ctx,
		"A-parent",
		corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL,
		room.Id,
		"thumb.png",
		"image/png",
		bytes.NewReader(createTestPNG(8, 8)),
	)
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment returned error: %v", err)
	}

	declared, ok := core.Assets.AssetCreation(derivative.GetId())
	if !ok {
		t.Fatal("derivative asset creation was not projected")
	}
	if declared.GetParentAssetId() != "A-parent" {
		t.Fatalf("ParentAssetId = %q, want %q", declared.GetParentAssetId(), "A-parent")
	}
	if declared.GetDerivativeRole() != corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL {
		t.Fatalf("DerivativeRole = %v, want thumbnail", declared.GetDerivativeRole())
	}
	if declared.GetUserId() != "" {
		t.Fatalf("UserId = %q, want empty for derivative", declared.GetUserId())
	}
}

func TestMediaModelUploadDerivativeAttachmentWithDimensionsProjectsAssetDimensions(t *testing.T) {
	core, _ := setupTestCore(t)
	service := core.mediaModel
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-derivative-dimensions", "Media derivative dimensions")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	derivative, err := service.UploadDerivativeAttachmentWithDimensions(
		ctx,
		"A-parent",
		corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT,
		room.Id,
		"clip-720p.mp4",
		"video/mp4",
		bytes.NewReader([]byte("video bytes")),
		1280,
		720,
	)
	if err != nil {
		t.Fatalf("UploadDerivativeAttachmentWithDimensions returned error: %v", err)
	}

	declared, ok := core.Assets.AssetCreation(derivative.GetId())
	if !ok {
		t.Fatal("derivative asset creation was not projected")
	}
	if declared.GetAsset().GetWidth() != 1280 || declared.GetAsset().GetHeight() != 720 {
		t.Fatalf("projected dimensions = %dx%d, want 1280x720", declared.GetAsset().GetWidth(), declared.GetAsset().GetHeight())
	}
}

func TestMediaModelCacheOperations(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	service := core.mediaModel
	ctx := testContext(t)
	key := ImageCacheKey(AttachmentSignResource, "A-cache", 64, 64, "cover")

	if !service.ImageCacheEnabled() {
		t.Fatal("ImageCacheEnabled returned false")
	}
	if got, err := service.GetCachedResize(ctx, key); err != nil || got != nil {
		t.Fatalf("initial GetCachedResize = %q, %v; want nil, nil", string(got), err)
	}
	if err := service.StoreCachedResize(ctx, key, []byte("webp")); err != nil {
		t.Fatalf("StoreCachedResize returned error: %v", err)
	}
	got, err := service.GetCachedResize(ctx, key)
	if err != nil {
		t.Fatalf("GetCachedResize returned error: %v", err)
	}
	if string(got) != "webp" {
		t.Fatalf("cached resize = %q, want %q", string(got), "webp")
	}
	deleted, err := service.DeleteCachedResizesForAttachment(ctx, "A-cache")
	if err != nil {
		t.Fatalf("DeleteCachedResizesForAttachment returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted count = %d, want 1", deleted)
	}
	got, err = service.GetCachedResize(ctx, key)
	if err != nil || got != nil {
		t.Fatalf("GetCachedResize after delete = %q, %v; want nil, nil", string(got), err)
	}
}

func TestMediaModelDeleteAttachmentFromStorageDeletesBinaryAndCache(t *testing.T) {
	core, _ := setupTestCoreWithCache(t)
	service := core.mediaModel
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-delete", "Media delete")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	attachment, err := service.UploadAttachment(ctx, SystemActorID, room.Id, "delete.txt", "text/plain", bytes.NewReader([]byte("delete me")))
	if err != nil {
		t.Fatalf("UploadAttachment returned error: %v", err)
	}
	cacheKey := ImageCacheKey(AttachmentSignResource, attachment.GetId(), 32, 32, "cover")
	if err := service.StoreCachedResize(ctx, cacheKey, []byte("cached")); err != nil {
		t.Fatalf("StoreCachedResize returned error: %v", err)
	}

	if err := service.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		t.Fatalf("DeleteAttachmentFromStorage returned error: %v", err)
	}
	if _, _, err := service.GetAttachmentReader(ctx, attachment); err == nil {
		t.Fatal("GetAttachmentReader after delete returned nil error")
	}
	if got, err := service.GetCachedResize(ctx, cacheKey); err != nil || got != nil {
		t.Fatalf("GetCachedResize after attachment delete = %q, %v; want nil, nil", string(got), err)
	}
	if err := service.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
		t.Fatalf("second DeleteAttachmentFromStorage returned error: %v", err)
	}
}

func TestMediaModelStableAttachmentURLs(t *testing.T) {
	core, _ := setupTestCore(t)
	core.AssetBaseURL = "https://assets.example"
	service := core.mediaModel

	before := time.Now()
	stable := service.GetStableAttachmentAssetURL("A-url", "U-url")
	if stable.URL == "" {
		t.Fatal("GetStableAttachmentAssetURL returned empty URL")
	}
	if !strings.HasPrefix(stable.URL, "https://assets.example/assets/files/A-url?access=") {
		t.Fatalf("stable URL = %q, want asset base URL and stable path", stable.URL)
	}
	if stable.ExpiresAt.IsZero() {
		t.Fatal("stable URL expiry was zero")
	}
	assertStableAssetURLTicket(t, core, stable, nil, before)

	if got := service.GetStableAttachmentURL("", "U-url"); got != "" {
		t.Fatalf("GetStableAttachmentURL with empty asset id = %q, want empty", got)
	}
	if got := service.GetStableAttachmentURL("A-url", ""); got != "" {
		t.Fatalf("GetStableAttachmentURL with empty user id = %q, want empty", got)
	}

	transformed := service.GetStableTransformedAttachmentAssetURL("A-url", "U-url", 128, 96, "contain")
	if transformed.URL == "" {
		t.Fatal("GetStableTransformedAttachmentAssetURL returned empty URL")
	}
	if !strings.HasPrefix(transformed.URL, "https://assets.example/assets/files/A-url/image/128x96/contain?access=") {
		t.Fatalf("stable transformed URL = %q, want transformed asset path", transformed.URL)
	}
	if transformed.ExpiresAt.IsZero() {
		t.Fatal("stable transformed URL expiry was zero")
	}
	assertStableAssetURLTicket(t, core, transformed, &signedurl.TransformParams{
		Width:  128,
		Height: 96,
		Fit:    "contain",
	}, before)
	if got := service.GetStableTransformedAttachmentURL("", "U-url", 128, 96, "contain"); got != "" {
		t.Fatalf("GetStableTransformedAttachmentURL with empty asset id = %q, want empty", got)
	}

	hls := service.GetStableHLSMasterPlaylistAssetURL("A-url", "U-url")
	if !strings.HasPrefix(hls.URL, "https://assets.example/assets/hls/A-url/master.m3u8?access=") {
		t.Fatalf("stable HLS URL = %q, want HLS master path", hls.URL)
	}
	parsedHLSURL, err := url.Parse(hls.URL)
	if err != nil {
		t.Fatalf("stable HLS URL parse failed: %v", err)
	}
	hlsTicket, err := signedurl.ParseSignedHLSAccessTicket(core.config.Assets.SigningSecret, parsedHLSURL.Query().Get("access"))
	if err != nil {
		t.Fatalf("stable HLS ticket parse failed: %v", err)
	}
	if hlsTicket.AssetID != "A-url" || hlsTicket.UserID != "U-url" || hlsTicket.ExpiresAt != hls.ExpiresAt.Unix() {
		t.Fatalf("stable HLS ticket = %#v, URL expiry = %v", hlsTicket, hls.ExpiresAt)
	}
}

func TestMediaModelStableAttachmentURLIssuanceBuckets(t *testing.T) {
	core, _ := setupTestCore(t)
	service := core.mediaModel
	now := time.Date(2026, time.July, 19, 10, 15, 0, 0, time.FixedZone("test", 2*60*60))
	service.now = func() time.Time { return now }

	first := service.GetStableAttachmentAssetURL("A-url", "U-url")
	firstTransformed := service.GetStableTransformedAttachmentAssetURL("A-url", "U-url", 128, 96, "contain")
	firstHLS := service.GetStableHLSMasterPlaylistAssetURL("A-url", "U-url")
	now = now.Add(44*time.Minute + 59*time.Second)
	withinBucket := service.GetStableAttachmentAssetURL("A-url", "U-url")
	withinBucketTransformed := service.GetStableTransformedAttachmentAssetURL("A-url", "U-url", 128, 96, "contain")
	withinBucketHLS := service.GetStableHLSMasterPlaylistAssetURL("A-url", "U-url")

	if withinBucket != first {
		t.Fatalf("stable URL changed within issuance bucket: %#v != %#v", withinBucket, first)
	}
	if withinBucketTransformed != firstTransformed {
		t.Fatalf("stable transformed URL changed within issuance bucket: %#v != %#v", withinBucketTransformed, firstTransformed)
	}
	if withinBucketHLS != firstHLS {
		t.Fatalf("stable HLS URL changed within issuance bucket: %#v != %#v", withinBucketHLS, firstHLS)
	}
	if got, want := first.ExpiresAt, time.Date(2026, time.July, 20, 8, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("stable URL expiry = %v, want %v", got, want)
	}

	now = now.Add(time.Second)
	nextBucket := service.GetStableAttachmentAssetURL("A-url", "U-url")
	nextBucketHLS := service.GetStableHLSMasterPlaylistAssetURL("A-url", "U-url")
	if nextBucket.URL == first.URL {
		t.Fatal("stable URL did not rotate across issuance bucket boundary")
	}
	if nextBucketHLS.URL == firstHLS.URL {
		t.Fatal("stable HLS URL did not rotate across issuance bucket boundary")
	}
	if !nextBucketHLS.ExpiresAt.Equal(nextBucket.ExpiresAt) {
		t.Fatalf("next-bucket HLS expiry = %v, want %v", nextBucketHLS.ExpiresAt, nextBucket.ExpiresAt)
	}
	if got, want := nextBucket.ExpiresAt.Sub(now), AssetAccessTicketTTL; got != want {
		t.Fatalf("next-bucket URL validity = %v, want %v", got, want)
	}
}

func assertStableAssetURLTicket(t *testing.T, core *ChattoCore, stable StableAssetURL, params *signedurl.TransformParams, before time.Time) {
	t.Helper()

	parsedURL, err := url.Parse(stable.URL)
	if err != nil {
		t.Fatalf("stable URL parse failed: %v", err)
	}
	access := parsedURL.Query().Get("access")
	if access == "" {
		t.Fatalf("stable URL %q did not include access ticket", stable.URL)
	}
	ticket, err := signedurl.ParseSignedAssetAccessTicket(core.config.Assets.SigningSecret, access)
	if err != nil {
		t.Fatalf("stable URL access ticket parse failed: %v", err)
	}
	if ticket.UserID != "U-url" {
		t.Fatalf("stable URL ticket user = %q, want U-url", ticket.UserID)
	}
	if ticket.AssetID != "A-url" {
		t.Fatalf("stable URL ticket asset = %q, want A-url", ticket.AssetID)
	}
	if !ticket.MatchesTransform(params) {
		t.Fatalf("stable URL ticket transform = %#v, want %#v", ticket, params)
	}
	if ticket.ExpiresAt != stable.ExpiresAt.Unix() {
		t.Fatalf("stable URL ticket expiry = %d, want %d", ticket.ExpiresAt, stable.ExpiresAt.Unix())
	}
	ttl := stable.ExpiresAt.Sub(before)
	minimumTTL := AssetAccessTicketTTL - assetAccessTicketIssueBucket
	if ttl < minimumTTL || ttl > AssetAccessTicketTTL+2*time.Second {
		t.Fatalf("stable URL ticket TTL = %v, want between %v and %v", ttl, minimumTTL, AssetAccessTicketTTL)
	}
}

func TestMediaModelAssetURLs(t *testing.T) {
	core, _ := setupTestCore(t)
	core.AssetBaseURL = "https://assets.example"
	service := core.mediaModel

	rawURL := service.GetStableAttachmentURL("A-url", "U-url")
	if !strings.HasPrefix(rawURL, "https://assets.example/assets/files/A-url?access=") {
		t.Fatalf("GetStableAttachmentURL = %q, want stable asset URL", rawURL)
	}

	transformed := service.GetStableTransformedAttachmentURL("A-url", "U-url", 64, 48, "cover")
	if !strings.HasPrefix(transformed, "https://assets.example/assets/files/A-url/image/64x48/cover?access=") {
		t.Fatalf("GetStableTransformedAttachmentURL = %q, want stable transform URL", transformed)
	}
	serverAsset := service.GetTransformedServerAssetURL("server.logo", 80, 80, "cover")
	if !strings.HasPrefix(serverAsset, "https://assets.example/assets/server/server.logo/t/") {
		t.Fatalf("GetTransformedServerAssetURL = %q, want signed server asset URL", serverAsset)
	}
	if got := service.GetStableAttachmentURL("", "U-url"); got != "" {
		t.Fatalf("GetStableAttachmentURL with empty asset id = %q, want empty", got)
	}
}

func TestMediaModelAttachmentNeedsVideoProcessing(t *testing.T) {
	tests := []struct {
		name        string
		attachment  *corev1.Attachment
		animatedGIF bool
		want        bool
	}{
		{name: "nil", want: false},
		{name: "video", attachment: &corev1.Attachment{ContentType: "video/mp4"}, want: true},
		{name: "animated gif", attachment: &corev1.Attachment{ContentType: "image/gif"}, animatedGIF: true, want: true},
		{name: "static image", attachment: &corev1.Attachment{ContentType: "image/png"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AttachmentNeedsVideoProcessing(tt.attachment, tt.animatedGIF); got != tt.want {
				t.Fatalf("AttachmentNeedsVideoProcessing = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAssetModelVideoProcessingLifecycle(t *testing.T) {
	core, _ := setupTestCore(t)
	media := core.mediaModel
	service := core.assetLifecycle()
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-video", "Media video")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	original, err := media.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment returned error: %v", err)
	}

	requested := false
	core.OnVideoProcessingRequested = func(_ context.Context, assetID, messageEventID string) error {
		requested = true
		if assetID != original.GetId() {
			t.Fatalf("requested asset id = %q, want %q", assetID, original.GetId())
		}
		if messageEventID != "E-message" {
			t.Fatalf("requested message event id = %q, want E-message", messageEventID)
		}
		return nil
	}
	if err := service.ScheduleVideoProcessingForMessageAttachment(ctx, SystemActorID, room.Id, "E-message", original); err != nil {
		t.Fatalf("ScheduleVideoProcessingForMessageAttachment returned error: %v", err)
	}
	if !requested {
		t.Fatal("video processing callback was not invoked")
	}
	manifest, ok := core.Assets.VideoAttachmentManifest(original.GetId())
	if !ok || manifest.Started == nil {
		t.Fatalf("manifest after schedule = %#v, %v; want started", manifest, ok)
	}

	thumbnail := &corev1.Attachment{Id: "A-thumb"}
	variants := []*corev1.VideoVariant{
		{Quality: "720p", Attachment: &corev1.Attachment{Id: "A-variant"}},
	}
	hls := &corev1.AssetProcessedHLS{
		Renditions: []*corev1.AssetHLSRendition{{
			Width: 1280, Height: 720, Bandwidth: 1_500_000,
			Segments: []*corev1.AssetHLSSegment{{AssetId: "A-hls-segment-1", DurationMs: 1000}, {AssetId: "A-hls-segment-2", DurationMs: 200}},
		}},
	}
	if err := service.RecordAssetProcessedWithHLS(ctx, SystemActorID, room.Id, "E-message", original.GetId(), 1200, 640, 360, thumbnail, variants, hls); err != nil {
		t.Fatalf("RecordAssetProcessedWithHLS returned error: %v", err)
	}
	manifest, ok = core.Assets.VideoAttachmentManifest(original.GetId())
	if !ok || manifest.Succeeded == nil {
		t.Fatalf("manifest after processed = %#v, %v; want succeeded", manifest, ok)
	}
	if manifest.Succeeded.GetVideo().GetThumbnailAssetId() != "A-thumb" {
		t.Fatalf("thumbnail asset id = %q, want A-thumb", manifest.Succeeded.GetVideo().GetThumbnailAssetId())
	}
	if got := manifest.Succeeded.GetVideo().GetVariants()[0].GetAssetId(); got != "A-variant" {
		t.Fatalf("variant asset id = %q, want A-variant", got)
	}
	if got := manifest.Succeeded.GetVideo().GetHls().GetRenditions()[0].GetSegments(); len(got) != 2 || got[0].GetAssetId() != "A-hls-segment-1" || got[1].GetDurationMs() != 200 {
		t.Fatalf("HLS segments = %v", got)
	}

	if err := service.RecordAssetProcessingFailed(ctx, SystemActorID, room.Id, "E-message", original.GetId(), corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING); err != nil {
		t.Fatalf("RecordAssetProcessingFailed returned error: %v", err)
	}
	manifest, ok = core.Assets.VideoAttachmentManifest(original.GetId())
	if !ok || manifest.Succeeded == nil || manifest.Failed != nil {
		t.Fatalf("manifest after terminal failure race = %#v, %v; want original success preserved", manifest, ok)
	}

	if err := service.RecordAssetDeleted(ctx, SystemActorID, room.Id, original.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted returned error: %v", err)
	}
	if _, ok := core.Assets.AssetCreation(original.GetId()); ok {
		t.Fatal("asset creation still projected after RecordAssetDeleted")
	}
}

func TestAssetModelProcessedCommitSurvivesProjectionWaitFailure(t *testing.T) {
	core, _ := setupTestCore(t)
	service := core.assetLifecycle()
	ctx := testContext(t)
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-video-ambiguous", "Media video ambiguous")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	original, err := core.media().UploadAttachment(ctx, SystemActorID, room.GetId(), "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	segment, err := core.media().UploadDerivativeAttachment(ctx, original.GetId(), corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_HLS_MEDIA_SEGMENT, room.GetId(), "480p-00000.ts", "video/mp2t", bytes.NewReader([]byte("segment")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment: %v", err)
	}

	service.waitForAssetsOverride = func(waitCtx context.Context, pos events.StreamPosition) error {
		if strings.HasSuffix(pos.SubjectFilter, "."+events.EventAssetProcessingSucceeded) {
			return context.DeadlineExceeded
		}
		return core.AssetsProjector.WaitFor(waitCtx, pos)
	}
	t.Cleanup(func() { service.waitForAssetsOverride = nil })

	hls := &corev1.AssetProcessedHLS{Renditions: []*corev1.AssetHLSRendition{{
		Segments: []*corev1.AssetHLSSegment{{AssetId: segment.GetId(), DurationMs: 1000}},
	}}}
	if err := service.RecordAssetProcessedWithHLS(ctx, SystemActorID, room.GetId(), "E-message", original.GetId(), 1000, 640, 360, nil, nil, hls); err != nil {
		t.Fatalf("RecordAssetProcessedWithHLS after committed append: %v", err)
	}

	processed, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(original.GetId()).Subject(events.EventAssetProcessingSucceeded))
	if err != nil {
		t.Fatalf("read processing success: %v", err)
	}
	if len(processed) != 1 {
		t.Fatalf("processing success events = %d, want 1", len(processed))
	}
	failed, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(original.GetId()).Subject(events.EventAssetProcessingFailed))
	if err != nil {
		t.Fatalf("read processing failures: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("processing failure events = %d, want 0", len(failed))
	}
	deleted, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(segment.GetId()).Subject(events.EventAssetDeleted))
	if err != nil {
		t.Fatalf("read segment deletions: %v", err)
	}
	if len(deleted) != 0 {
		t.Fatalf("segment deletion events = %d, want 0", len(deleted))
	}
	reader, _, err := core.media().GetAttachmentReader(ctx, segment)
	if err != nil {
		t.Fatalf("committed segment became unreadable: %v", err)
	}
	if closer, ok := reader.(io.Closer); ok {
		closer.Close()
	}
}

func TestDerivativeCreationCommitSurvivesProjectionWaitFailure(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "derivative-ambiguous", "Derivative ambiguous")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	original, err := core.media().UploadAttachment(ctx, SystemActorID, room.GetId(), "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	service := core.assetLifecycle()
	service.waitForAssetsOverride = func(waitCtx context.Context, pos events.StreamPosition) error {
		if strings.HasSuffix(pos.SubjectFilter, "."+events.EventAssetCreated) {
			return context.DeadlineExceeded
		}
		return core.AssetsProjector.WaitFor(waitCtx, pos)
	}
	t.Cleanup(func() { service.waitForAssetsOverride = nil })

	segment, err := core.media().UploadDerivativeAttachment(ctx, original.GetId(), corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_HLS_MEDIA_SEGMENT, room.GetId(), "480p-00000.ts", "video/mp2t", bytes.NewReader([]byte("segment")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment after committed append: %v", err)
	}
	created, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(segment.GetId()).Subject(events.EventAssetCreated))
	if err != nil {
		t.Fatalf("read derivative creation: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("derivative creation events = %d, want 1", len(created))
	}
	reader, _, err := core.media().GetAttachmentReader(ctx, segment)
	if err != nil {
		t.Fatalf("committed derivative became unreadable: %v", err)
	}
	if closer, ok := reader.(io.Closer); ok {
		closer.Close()
	}
}

func TestLosingVideoGenerationCleansDerivativesPromptly(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "video-losing-generation", "Video losing generation")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	original, err := core.media().UploadAttachment(ctx, SystemActorID, room.GetId(), "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment: %v", err)
	}
	if err := core.assetLifecycle().RecordAssetProcessedWithHLS(ctx, SystemActorID, room.GetId(), "E-message", original.GetId(), 1000, 640, 360, nil, nil, &corev1.AssetProcessedHLS{}); err != nil {
		t.Fatalf("record winning generation: %v", err)
	}
	loser, err := core.media().UploadDerivativeAttachment(ctx, original.GetId(), corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_HLS_MEDIA_SEGMENT, room.GetId(), "loser.ts", "video/mp2t", bytes.NewReader([]byte("loser")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment: %v", err)
	}
	losingHLS := &corev1.AssetProcessedHLS{Renditions: []*corev1.AssetHLSRendition{{Segments: []*corev1.AssetHLSSegment{{AssetId: loser.GetId(), DurationMs: 1000}}}}}
	if err := core.assetLifecycle().RecordAssetProcessedWithHLS(ctx, SystemActorID, room.GetId(), "E-message", original.GetId(), 1000, 640, 360, nil, nil, losingHLS); err != nil {
		t.Fatalf("record losing generation: %v", err)
	}
	deleted, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(loser.GetId()).Subject(events.EventAssetDeleted))
	if err != nil {
		t.Fatalf("read derivative deletion events: %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("derivative deletion events = %d, want 1", len(deleted))
	}
	if _, _, err := core.media().GetAttachmentReader(ctx, loser); err == nil {
		t.Fatal("losing derivative remained readable after prompt cleanup")
	}
}

func TestAssetModelRecordAssetDeletedRequiresActor(t *testing.T) {
	core, _ := setupTestCore(t)
	service := core.assetLifecycle()
	ctx := testContext(t)

	err := service.RecordAssetDeleted(ctx, "", "R-missing-actor", "A-missing-actor")
	if err == nil {
		t.Fatal("RecordAssetDeleted with missing actor returned nil error")
	}
	if !strings.Contains(err.Error(), "asset deletion missing actor id") {
		t.Fatalf("RecordAssetDeleted error = %q, want missing actor message", err.Error())
	}
}

func TestAssetModelProcessingDoesNotAppendAfterAssetDeleted(t *testing.T) {
	core, _ := setupTestCore(t)
	media := core.mediaModel
	service := core.assetLifecycle()
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-video-deleted", "Media video deleted")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	original, err := media.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment returned error: %v", err)
	}
	if err := service.RecordAssetDeleted(ctx, SystemActorID, room.Id, original.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted returned error: %v", err)
	}

	if err := service.RecordAssetProcessed(ctx, SystemActorID, room.Id, "E-message", original.GetId(), 1200, 640, 360, nil, nil); err != nil {
		t.Fatalf("RecordAssetProcessed after deletion returned error: %v", err)
	}
	processedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(original.GetId()).Subject(events.EventAssetProcessingSucceeded))
	if err != nil {
		t.Fatalf("SubjectEvents(asset_processing_succeeded): %v", err)
	}
	if len(processedEvents) != 0 {
		t.Fatalf("asset_processing_succeeded events after deletion = %d, want 0", len(processedEvents))
	}
	if manifest, ok := core.Assets.VideoAttachmentManifest(original.GetId()); ok || manifest != nil {
		t.Fatalf("VideoAttachmentManifest after deleted processing = %#v, %v; want none", manifest, ok)
	}
}

func TestAssetModelSkippedVideoManifestCleansUpDerivativeOutputs(t *testing.T) {
	core, _ := setupTestCore(t)
	media := core.mediaModel
	service := core.assetLifecycle()
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-video-skipped", "Media video skipped")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	original, err := media.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment returned error: %v", err)
	}
	thumbnail, err := media.UploadDerivativeAttachment(ctx, original.GetId(), corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumb.png", "image/png", bytes.NewReader(createTestPNG(16, 16)))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment thumbnail: %v", err)
	}
	variantAttachment, err := media.UploadDerivativeAttachment(ctx, original.GetId(), corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT, room.Id, "clip_480p.mp4", "video/mp4", bytes.NewReader([]byte("variant")))
	if err != nil {
		t.Fatalf("UploadDerivativeAttachment variant: %v", err)
	}
	variants := []*corev1.VideoVariant{
		{Quality: "480p", Attachment: variantAttachment},
	}

	if err := service.RecordAssetDeleted(ctx, SystemActorID, room.Id, original.GetId()); err != nil {
		t.Fatalf("RecordAssetDeleted original: %v", err)
	}
	if err := service.RecordAssetProcessed(ctx, SystemActorID, room.Id, "E-message", original.GetId(), 1200, 640, 360, thumbnail, variants); err != nil {
		t.Fatalf("RecordAssetProcessed after original deletion returned error: %v", err)
	}

	processedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(original.GetId()).Subject(events.EventAssetProcessingSucceeded))
	if err != nil {
		t.Fatalf("SubjectEvents(asset_processing_succeeded): %v", err)
	}
	if len(processedEvents) != 0 {
		t.Fatalf("asset_processing_succeeded events after deletion = %d, want 0", len(processedEvents))
	}
	for _, derivative := range []*corev1.Attachment{thumbnail, variantAttachment} {
		deletedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(derivative.GetId()).Subject(events.EventAssetDeleted))
		if err != nil {
			t.Fatalf("SubjectEvents(asset_deleted %s): %v", derivative.GetId(), err)
		}
		if len(deletedEvents) != 1 {
			t.Fatalf("asset_deleted events for %s = %d, want 1", derivative.GetId(), len(deletedEvents))
		}
		if _, ok := core.Assets.AssetCreation(derivative.GetId()); ok {
			t.Fatalf("derivative %s still projected after skipped manifest cleanup", derivative.GetId())
		}
		if _, _, err := media.GetAttachmentReader(ctx, derivative); err == nil {
			t.Fatalf("derivative %s backing bytes still readable after skipped manifest cleanup", derivative.GetId())
		}
	}
}

func TestAssetModelPublishAssetProcessingRejectsRoomMismatch(t *testing.T) {
	core, _ := setupTestCore(t)
	media := core.mediaModel
	service := core.assetLifecycle()
	ctx := testContext(t)

	sourceRoom, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-source-room", "Media source")
	if err != nil {
		t.Fatalf("CreateRoom source: %v", err)
	}
	otherRoom, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-other-room", "Media other")
	if err != nil {
		t.Fatalf("CreateRoom other: %v", err)
	}
	original, err := media.UploadAttachment(ctx, SystemActorID, sourceRoom.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment returned error: %v", err)
	}

	err = service.RecordAssetProcessingStarted(ctx, SystemActorID, otherRoom.Id, "E-message", original.GetId())
	if err == nil {
		t.Fatal("RecordAssetProcessingStarted with mismatched room returned nil error")
	}
	if !strings.Contains(err.Error(), "asset processing event room mismatch") {
		t.Fatalf("RecordAssetProcessingStarted error = %q, want room mismatch", err.Error())
	}
	startedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(original.GetId()).Subject(events.EventAssetProcessingStarted))
	if err != nil {
		t.Fatalf("SubjectEvents(asset_processing_started): %v", err)
	}
	if len(startedEvents) != 0 {
		t.Fatalf("asset_processing_started events after room mismatch = %d, want 0", len(startedEvents))
	}
}

func TestAssetModelDeleteVideoDerivativesUsesInheritedAssetRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	media := core.mediaModel
	service := core.assetLifecycle()
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "media-video-inherited", "Media video inherited")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	original, err := media.UploadAttachment(ctx, SystemActorID, room.Id, "clip.mp4", "video/mp4", bytes.NewReader([]byte("video")))
	if err != nil {
		t.Fatalf("UploadAttachment returned error: %v", err)
	}

	thumbnail := &corev1.Attachment{Id: "A-inherited-thumb"}
	inheritedCreated := &corev1.Event{
		Id: "E-inherited-thumb-created",
		Event: &corev1.Event_AssetCreated{
			AssetCreated: &corev1.AssetCreatedEvent{
				OriginalBinaryAvailable: true,
				Asset: &corev1.AssetRecord{
					Id:          thumbnail.GetId(),
					Filename:    "thumb.png",
					ContentType: "image/png",
				},
				ParentAssetId:  original.GetId(),
				DerivativeRole: corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL,
			},
		},
	}
	inheritedSubject := events.AssetAggregate(thumbnail.GetId()).SubjectFor(inheritedCreated)
	inheritedSeq, err := core.EventPublisher.Append(ctx, inheritedSubject, inheritedCreated)
	if err != nil {
		t.Fatalf("Append inherited thumbnail creation: %v", err)
	}
	if err := core.AssetsProjector.WaitFor(ctx, events.SubjectPosition(inheritedSubject, inheritedSeq)); err != nil {
		t.Fatalf("Wait for inherited thumbnail creation: %v", err)
	}

	if err := service.RecordAssetProcessed(ctx, SystemActorID, room.Id, "E-message", original.GetId(), 1200, 640, 360, thumbnail, nil); err != nil {
		t.Fatalf("RecordAssetProcessed returned error: %v", err)
	}
	service.DeleteVideoDerivativesForAttachment(ctx, SystemActorID, original.GetId())

	deletedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, events.AssetAggregate(thumbnail.GetId()).Subject(events.EventAssetDeleted))
	if err != nil {
		t.Fatalf("SubjectEvents(asset_deleted): %v", err)
	}
	if len(deletedEvents) != 1 {
		t.Fatalf("thumbnail asset_deleted events = %d, want 1", len(deletedEvents))
	}
	if roomID, ok := core.Assets.AssetRoomID(thumbnail.GetId()); !ok || roomID != room.Id {
		t.Fatalf("deleted thumbnail room = %q, %v; want %q, true", roomID, ok, room.Id)
	}
}

func TestMediaModelMessageBodyAttachmentLookups(t *testing.T) {
	core, _ := setupTestCore(t)
	service := core.mediaModel
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "media-lookup-user", "Media Lookup", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "media-lookup", "Media lookup")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	attachment, err := service.UploadAttachment(ctx, user.Id, room.Id, "lookup.txt", "text/plain", bytes.NewReader([]byte("lookup")))
	if err != nil {
		t.Fatalf("UploadAttachment returned error: %v", err)
	}
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "with asset", []string{attachment.GetId()}, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	body, retracted, ok := core.RoomTimeline.LatestBody(event.GetId())
	if !ok || retracted {
		t.Fatalf("LatestBody ok=%v retracted=%v, want ok true retracted false", ok, retracted)
	}

	attachments := service.MessageBodyAttachments(body)
	if len(attachments) != 1 || attachments[0].GetId() != attachment.GetId() {
		t.Fatalf("MessageBodyAttachments = %#v, want attachment %s", attachments, attachment.GetId())
	}
	declared, ok := core.Assets.AssetCreation(attachment.GetId())
	if !ok || declared == nil {
		t.Fatalf("AssetCreation missing for %s", attachment.GetId())
	}
	if declared.GetAsset().GetId() != attachment.GetId() {
		t.Fatalf("AssetCreation id = %q, want %q", declared.GetAsset().GetId(), attachment.GetId())
	}
}
