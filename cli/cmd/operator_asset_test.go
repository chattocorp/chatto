package cmd

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

func TestOperatorAssetUploadLocalImage(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-image-author", "Image Author", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "operator-image-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	filePath := filepath.Join(t.TempDir(), "source.png")
	var content bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&content, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	if err := os.WriteFile(filePath, content.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	args := []string{
		"operator", "asset", "upload", "--room-id", room.GetId(), "--author-id", author.GetId(),
		"--file", filePath, "--filename", "exported.png", "--content-type", "image/png", "--json",
	}
	output := env.run(t, args...)
	var result operatorv1.CompleteUploadResponse
	if err := protojson.Unmarshal([]byte(output), &result); err != nil || result.GetAssetId() == "" {
		t.Fatalf("upload JSON = %q, err = %v", output, err)
	}
	events, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.AssetAggregate(result.GetAssetId()).Subject(evtstream.EventAssetCreated))
	if err != nil || len(events) != 1 {
		t.Fatalf("asset creation events = %+v, err = %v", events, err)
	}
	created := events[0].GetAssetCreated()
	if events[0].GetActorId() != core.SystemActorID || created.GetUserId() != author.GetId() || created.GetRoomId() != room.GetId() || created.GetAsset().GetFilename() != "exported.png" || created.GetAsset().GetContentType() != "image/png" {
		t.Fatalf("created asset metadata = %+v, actor = %q", created, events[0].GetActorId())
	}
	human := env.run(t, "operator", "asset", "upload", "--room-id", room.GetId(), "--author-id", author.GetId(), "--file", filePath)
	if strings.TrimSpace(human) == "" || strings.Contains(human, "{") {
		t.Fatalf("human output = %q", human)
	}
	defaultEvents, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.AssetAggregate(strings.TrimSpace(human)).Subject(evtstream.EventAssetCreated))
	if err != nil || len(defaultEvents) != 1 {
		t.Fatalf("default upload events = %+v, err = %v", defaultEvents, err)
	}
	defaultAsset := defaultEvents[0].GetAssetCreated().GetAsset()
	if defaultAsset.GetFilename() != "source.png" || defaultAsset.GetContentType() != "image/png" {
		t.Fatalf("default asset filename = %q, type = %q", defaultAsset.GetFilename(), defaultAsset.GetContentType())
	}
}

func TestOperatorAssetUploadRejectsInvalidFileAndFlags(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	for _, args := range [][]string{
		{"operator", "asset", "upload"},
		{"operator", "asset", "upload", "--room-id", "room", "--author-id", "author"},
		{"operator", "asset", "upload", "--room-id", "room", "--author-id", "author", "--file", t.TempDir()},
	} {
		if _, err := env.execute(t, args...); err == nil {
			t.Fatalf("upload accepted invalid input: %v", args)
		}
	}
}

func TestOperatorAssetUploadMultipleChunks(t *testing.T) {
	env := newOperatorCLITestEnv(t)
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "operator-large-author", "Large Author", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "operator-large-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	content := bytes.Repeat([]byte("a"), 1024*1024+13)
	filePath := filepath.Join(t.TempDir(), "large.txt")
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	assetID := strings.TrimSpace(env.run(t, "operator", "asset", "upload", "--room-id", room.GetId(), "--author-id", author.GetId(), "--file", filePath))
	events, _, err := env.core.EventPublisher.SubjectEvents(env.ctx, evtstream.AssetAggregate(assetID).Subject(evtstream.EventAssetCreated))
	if err != nil || len(events) != 1 || events[0].GetAssetCreated().GetAsset().GetSize() != int64(len(content)) {
		t.Fatalf("large asset events = %+v, err = %v", events, err)
	}
}
