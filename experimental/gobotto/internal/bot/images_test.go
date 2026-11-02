package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

type imageTestClient struct {
	*richThreadClient
	data      []byte
	downloads int
	err       error
}

func (f *imageTestClient) DownloadAsset(_ context.Context, room, id string, opts chatto.AssetDownloadOptions) ([]byte, error) {
	f.downloads++
	if room != "room" || id != "picture" || opts.MaxBytes != imageDownloadLimit || opts.Thumbnail == nil {
		return nil, errors.New("incorrect download scope")
	}
	return f.data, f.err
}

func imageFixture(t *testing.T) *imageTestClient {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 32, 16))); err != nil {
		t.Fatal(err)
	}
	m := input("photo", "What is in this image?", true).Message
	m.Attachments = []chatto.Attachment{{ID: "picture", ContentType: "image/png", Filename: "private-filename.png", AssetURL: chatto.AssetURL{URL: "https://example.test/?secret-capability"}}}
	return &imageTestClient{richThreadClient: &richThreadClient{fakeClient: newFake(), message: m}, data: data.Bytes()}
}

func TestImageInspectionSuppliesPixelsOnlyOnWire(t *testing.T) {
	f := imageFixture(t)
	c := &conversation{key: threadKey{"room", "root"}}
	calls := 0
	c.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
		calls++
		encoded, _ := json.Marshal(messages)
		text := string(encoded)
		if strings.Contains(text, "secret-capability") || strings.Contains(text, "private-filename") {
			t.Fatal("attachment metadata leaked")
		}
		if calls == 1 {
			if !strings.Contains(text, `assetId`) || !strings.Contains(text, "picture") {
				t.Fatal("missing image discovery")
			}
			var msg agent.Message
			if err := json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"view","type":"function","function":{"name":"inspect_image","arguments":"{\"message_id\":\"photo\",\"asset_id\":\"picture\"}"}}]}`), &msg); err != nil {
				t.Fatal(err)
			}
			return msg, nil
		}
		if strings.Contains(text, "data:image/jpeg;base64,") != (calls == 2) {
			t.Fatal("pixels missing or leaked across turns")
		}
		return answer(), nil
	}
	if err := respond(context.Background(), f, c, []chatto.MessageEvent{{Message: f.message, Addressed: true}}, nil); err != nil {
		t.Fatal(err)
	}
	if f.downloads != 1 || calls != 2 {
		t.Fatal("image was not inspected")
	}
	stored, _ := json.Marshal(c.model.Snapshot())
	if strings.Contains(string(stored), "base64") || strings.Contains(string(stored), "secret-capability") {
		t.Fatal("pixels or URL persisted")
	}
	if err := respond(context.Background(), f, c, []chatto.MessageEvent{input("next", "Thanks", false)}, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatal("follow-up missing")
	}
}

func TestImageInspectionRejectsUntrustedReferences(t *testing.T) {
	for _, scenario := range []string{"wrong asset", "wrong message", "other room", "other thread", "deleted", "unsupported", "download failure", "corrupt", "unknown argument", "limit"} {
		t.Run(scenario, func(t *testing.T) {
			f := imageFixture(t)
			v := &turnImages{}
			args := map[string]string{"message_id": "photo", "asset_id": "picture"}
			switch scenario {
			case "wrong asset":
				args["asset_id"] = "other"
			case "wrong message":
				args["message_id"] = "other"
			case "other room":
				f.message.RoomID = "other"
			case "other thread":
				f.message.ThreadRootEventID = "other"
			case "deleted":
				f.message.DeletedAt = "deleted"
			case "unsupported":
				f.message.Attachments[0].ContentType = "image/svg+xml"
			case "download failure":
				f.err = errors.New("secret-capability")
			case "corrupt":
				f.data = []byte("not an image")
			case "unknown argument":
				args["url"] = "https://example.test/private"
			case "limit":
				v.loaded = map[string]bool{"a": true, "b": true, "c": true, "d": true}
			}
			raw, _ := json.Marshal(args)
			result, err := v.tool(f, &conversation{key: threadKey{"room", "root"}}).Execute(context.Background(), raw)
			if err != nil || !strings.Contains(result, "rejected") || strings.Contains(result, "secret-capability") || len(v.parts) != 0 {
				t.Fatal("unsafe image rejection", err)
			}
			if scenario != "download failure" && scenario != "corrupt" && f.downloads != 0 {
				t.Fatal("unauthorized download")
			}
		})
	}
}

func TestImageReferencesSurviveInboxAndInvalidateEdits(t *testing.T) {
	f := imageFixture(t)
	m := f.message
	stored := messageForInbox(m)
	data, _ := json.Marshal(stored)
	if strings.Contains(string(data), "secret-capability") || strings.Contains(string(data), "private-filename") || len(stored.Attachments) != 1 {
		t.Fatal("unsafe inbox image reference")
	}
	if fingerprint(stored) != fingerprint(m) {
		t.Fatal("inbox changed fingerprint")
	}
	before := fingerprint(m)
	m.Attachments[0].AssetURL.URL = "refreshed"
	if fingerprint(m) != before {
		t.Fatal("URL refresh invalidates image")
	}
	m.Attachments[0].ID = "replacement"
	if fingerprint(m) == before {
		t.Fatal("image replacement ignored")
	}
	m.Attachments = nil
	if fingerprint(m) == before {
		t.Fatal("image removal ignored")
	}
}

func TestPrepareImageBoundsAndNormalizes(t *testing.T) {
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewRGBA(image.Rect(0, 0, 2000, 100))); err != nil {
		t.Fatal(err)
	}
	data, err := prepareImage(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" || cfg.Width != 1600 || cfg.Height != 80 {
		t.Fatal("normalization failed")
	}
	if _, err := prepareImage(make([]byte, imageDownloadLimit+1)); err == nil {
		t.Fatal("oversized input accepted")
	}
}
