package connectapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/assets"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestGIFVideoThumbnailURLs(t *testing.T) {
	for _, tt := range []struct {
		name          string
		frames        int
		videoEnabled  bool
		terminal      string
		wantImage     bool
		wantThumbnail bool
	}{
		{name: "processing", frames: assets.MaxAnimatedImageFrames + 1, videoEnabled: true},
		{name: "completed", frames: assets.MaxAnimatedImageFrames + 1, videoEnabled: true, terminal: "completed", wantThumbnail: true},
		{name: "completed without thumbnail", frames: assets.MaxAnimatedImageFrames + 1, videoEnabled: true, terminal: "no thumbnail"},
		{name: "failed", frames: assets.MaxAnimatedImageFrames + 1, videoEnabled: true, terminal: "failed"},
		{name: "static", frames: 1, videoEnabled: true, wantImage: true, wantThumbnail: true},
		{name: "image only", frames: 2, wantImage: true, wantThumbnail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := newGIFThumbnailTestEnv(t, tt.videoEnabled)
			ctx := withCaller(env.ctx, env.viewer)
			room := env.createJoinedRoom("gif-thumbnails")
			animation := &gif.GIF{
				Image: make([]*image.Paletted, tt.frames),
				Delay: make([]int, tt.frames),
			}
			for i := range tt.frames {
				frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
				frame.SetColorIndex(1, 1, uint8(i%2))
				animation.Image[i] = frame
				animation.Delay[i] = 10
			}
			var content bytes.Buffer
			require.NoError(t, gif.EncodeAll(&content, animation))
			sum := sha256.Sum256(content.Bytes())
			hash := hex.EncodeToString(sum[:])
			upload, err := env.assetUploads.CreateUpload(ctx, connect.NewRequest(&apiv1.CreateUploadRequest{
				RoomId: room.Id, Filename: "animation.gif", ContentType: "image/gif", Size: int64(content.Len()), Sha256: hash,
			}))
			require.NoError(t, err)
			_, err = env.assetUploads.UploadChunk(ctx, connect.NewRequest(&apiv1.UploadChunkRequest{
				UploadId: upload.Msg.Upload.UploadId, Content: content.Bytes(), ChunkSha256: hash,
			}))
			require.NoError(t, err)
			completed, err := env.assetUploads.CompleteUpload(ctx, connect.NewRequest(&apiv1.CompleteUploadRequest{
				UploadId: upload.Msg.Upload.UploadId,
			}))
			require.NoError(t, err)
			pending := completed.Msg.GetAsset()
			assetID := pending.GetId()
			require.NotEmpty(t, assetID)
			if tt.wantImage {
				require.NotNil(t, pending.GetThumbnailAssetUrl())
			} else {
				require.Nil(t, pending.GetThumbnailAssetUrl(), "pending GIF must not advertise an image transform")
			}
			message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "GIF", []string{assetID}, "", "", nil, false)
			require.NoError(t, err)

			var thumbnail *evtv1.Attachment
			switch tt.terminal {
			case "completed", "no thumbnail":
				if tt.wantThumbnail {
					thumbnail, err = env.core.UploadDerivativeAttachment(env.ctx, assetID, evtv1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumbnail.png", "image/png", bytes.NewReader(connectAPITestPNG()))
					require.NoError(t, err)
				}
				require.NoError(t, env.core.RecordAssetProcessedWithHLS(env.ctx, core.SystemActorID, room.Id, message.Id, assetID, 1000, 2, 2, thumbnail, nil, nil))
			case "failed":
				require.NoError(t, env.core.RecordAssetProcessingFailed(env.ctx, core.SystemActorID, room.Id, message.Id, assetID, evtv1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED))
			}

			get, err := env.assets.GetAsset(ctx, connect.NewRequest(&apiv1.GetAssetRequest{RoomId: room.Id, AssetId: assetID}))
			require.NoError(t, err)
			batch, err := env.assets.BatchGetAssets(ctx, connect.NewRequest(&apiv1.BatchGetAssetsRequest{RoomId: room.Id, AssetIds: []string{assetID}}))
			require.NoError(t, err)
			require.Len(t, batch.Msg.Assets, 1)
			files, err := env.rooms.ListRoomAttachments(ctx, connect.NewRequest(&apiv1.ListRoomAttachmentsRequest{RoomId: room.Id}))
			require.NoError(t, err)
			require.Len(t, files.Msg.Attachments, 1)
			preview, err := env.messages.GetMessage(ctx, connect.NewRequest(&apiv1.GetMessageRequest{RoomId: room.Id, EventId: message.Id}))
			require.NoError(t, err)
			require.Len(t, preview.Msg.Message.Attachments, 1)
			timeline, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id, Limit: 10}))
			require.NoError(t, err)
			timelineMessage := timelinePageEvent(timeline.Msg.Page, message.Id).GetMessagePosted().GetMessage()
			require.Len(t, timelineMessage.GetAttachments(), 1)

			for _, value := range []*apiv1.MessageAssetUrl{
				get.Msg.Asset.GetThumbnailAssetUrl(),
				batch.Msg.Assets[0].GetThumbnailAssetUrl(),
				files.Msg.Attachments[0].Attachment.GetThumbnailAssetUrl(),
				preview.Msg.Message.Attachments[0].GetThumbnailAssetUrl(),
				timelineMessage.Attachments[0].GetThumbnailAssetUrl(),
			} {
				if !tt.wantThumbnail {
					require.Nil(t, value, "GIF without a video thumbnail must not advertise an image transform")
					continue
				}
				require.NotNil(t, value)
				if tt.wantImage {
					require.Contains(t, value.GetUrl(), "/assets/files/"+assetID+"/")
				} else {
					require.Contains(t, value.GetUrl(), "/assets/files/"+thumbnail.GetId()+"?")
					require.NotContains(t, value.GetUrl(), "/assets/files/"+assetID+"/")
				}
			}
		})
	}
}

func TestGIFVideoThumbnailFromHistoricalManifest(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("historical-gif")
	var content bytes.Buffer
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	require.NoError(t, gif.EncodeAll(&content, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{10, 10}}))
	original, err := env.core.UploadAttachment(env.ctx, env.viewer.Id, room.Id, "historical.gif", "image/gif", &content)
	require.NoError(t, err)
	require.False(t, env.core.GetAssetState(original.Id).Creation.GetNeedsVideoProcessing())
	message, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "historical GIF", []string{original.Id}, "", "", nil, false)
	require.NoError(t, err)
	thumbnail, err := env.core.UploadDerivativeAttachment(env.ctx, original.Id, evtv1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, room.Id, "thumbnail.png", "image/png", bytes.NewReader(connectAPITestPNG()))
	require.NoError(t, err)
	require.NoError(t, env.core.RecordAssetProcessedWithHLS(env.ctx, core.SystemActorID, room.Id, message.Id, original.Id, 1000, 2, 2, thumbnail, nil, nil))
	ctx := withCaller(env.ctx, env.viewer)
	asset, err := env.assets.GetAsset(ctx, connect.NewRequest(&apiv1.GetAssetRequest{RoomId: room.Id, AssetId: original.Id}))
	require.NoError(t, err)
	require.Contains(t, asset.Msg.Asset.GetThumbnailAssetUrl().GetUrl(), "/assets/files/"+thumbnail.Id+"?")
}

// newGIFThumbnailTestEnv sets video admission before starting core workers.
func newGIFThumbnailTestEnv(t *testing.T, videoEnabled bool) *connectAPITestEnv {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	c, err := core.NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	require.NoError(t, err)
	c.VideoUploadsEnabled = videoEnabled
	startConnectAPITestCore(t, c)
	viewer, err := c.CreateUser(ctx, core.SystemActorID, "gif-viewer", "GIF Viewer", "password")
	require.NoError(t, err)
	api := New(c, config.ChattoConfig{Video: config.VideoConfig{Enabled: videoEnabled}}, "test")
	return &connectAPITestEnv{
		ctx: ctx, core: c, api: api, viewer: viewer,
		assets:       &assetService{api: api},
		assetUploads: &assetUploadService{api: api},
		rooms:        &roomService{api: api},
		messages:     &messageService{api: api},
	}
}
