package bot

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func richMessage(id string) chatto.Message {
	m := input(id, "ordinary text https://example.test/user-written-link", true).Message
	m.CreatedAt = "2026-11-01T12:00:00Z"
	m.UpdatedAt = "2026-11-01T12:01:00Z"
	m.Pinned = true
	m.Thread = &chatto.ThreadSummary{ReplyCount: 2, ParticipantPreviewUserIDs: []string{"metadata-only-participant"}}
	m.Reactions = []chatto.MessageReaction{{Emoji: "heart", Count: 2}}
	m.Attachments = []chatto.Attachment{{ID: "asset", Filename: "metadata-only.png",
		AssetURL:          chatto.AssetURL{URL: "https://example.test/asset?access=secret-capability"},
		ThumbnailAssetURL: chatto.AssetURL{URL: "https://example.test/thumb?access=secret-capability"}}}
	return m
}

type richThreadClient struct {
	*fakeClient
	message chatto.Message
}

func (f *richThreadClient) Thread(context.Context, string, string, string) (chatto.Page, error) {
	return chatto.Page{Messages: []chatto.Message{f.message}}, nil
}

func assertNoResourceMetadata(t *testing.T, data []byte) {
	t.Helper()
	for _, forbidden := range []string{"secret-capability", "metadata-only.png", "attachments", "createdAt", "updatedAt", "pinned", "reactions", "metadata-only-participant", "replyCount"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("resource metadata escaped the bot projection: %s", forbidden)
		}
	}
}

func TestModelAndStoredHistoryExcludeSDKResourceMetadata(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f := &richThreadClient{fakeClient: newFake(), message: richMessage("first")}
	c := &conversation{id: 1, provider: "test-provider", key: threadKey{"room", "root"}, store: s}
	calls := 0
	c.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
		calls++
		data, err := json.Marshal(messages)
		if err != nil {
			t.Fatal(err)
		}
		assertNoResourceMetadata(t, data)
		if !strings.Contains(string(data), "user-written-link") {
			t.Fatal("projection removed message text")
		}
		return answer(), nil
	}
	for _, id := range []string{"second", "third"} {
		e := chatto.MessageEvent{ID: id, Message: richMessage(id), Addressed: true}
		if err := s.checkpoint("cursor", &e); err != nil {
			t.Fatal(err)
		}
		if err := respond(context.Background(), f, c, []chatto.MessageEvent{e}, nil); err != nil {
			t.Fatal(err)
		}
		if len(e.Message.Attachments) != 1 {
			t.Fatal("projection mutated caller's message")
		}
	}
	if calls != 2 {
		t.Fatal("expected initial and incremental model requests")
	}
	if err := s.save(c, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{s.path(c), filepath.Join(s.dir, ".realtime")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		assertNoResourceMetadata(t, data)
	}
}

func TestEditComparisonIgnoresSDKMetadataButRetainsContentAndIdentity(t *testing.T) {
	base := input("first", "unchanged", true).Message
	// Preserve hashes from sessions created before the SDK acquired richer fields.
	legacy := `{"id":"first","roomId":"room","actorId":"human","body":"unchanged","threadRootEventId":"root"}`
	if fingerprint(base) != fmt.Sprintf("%x", sha256.Sum256([]byte(legacy))) {
		t.Fatal("legacy fingerprint changed")
	}
	for _, field := range []string{"metadata", "body", "actor", "room", "thread", "deleted"} {
		t.Run(field, func(t *testing.T) {
			fresh := richMessage("first")
			fresh.Body = base.Body
			switch field {
			case "body":
				fresh.Body = "edited"
			case "actor":
				fresh.ActorID = "someone-else"
			case "room":
				fresh.RoomID = "elsewhere"
			case "thread":
				fresh.ThreadRootEventID = "other-thread"
			case "deleted":
				fresh.DeletedAt = "now"
			}
			c := &conversation{key: threadKey{"room", "root"}}
			c.contextIDs.Store("first", fingerprint(base))
			f := &richThreadClient{fakeClient: newFake(), message: fresh}
			e := chatto.MessageEvent{InvalidatedRoom: "room", InvalidatedMessage: "first", InvalidationReason: chatto.MessageEdited}
			if got := c.unchangedEdit(context.Background(), f, e, nil); got != (field == "metadata") {
				t.Fatal("incorrect edit classification")
			}
		})
	}
}
