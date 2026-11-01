package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func metadataMessage() chatto.Message {
	return chatto.Message{ID: "m", ActorID: "alice", Body: "A threaded message", UpdatedAt: "2026-11-01T12:00:00Z", Pinned: true,
		Thread:    &chatto.ThreadSummary{ThreadRootEventID: "m", ReplyCount: 3, ParticipantPreviewUserIDs: []string{"alice", "bob"}, ParticipantCount: 5},
		Reactions: []chatto.MessageReaction{{Emoji: "thumbsup", Count: 2, HasReacted: true}, {Emoji: "custom_party", Count: 1}}}
}

func TestMessageMetadataRowsWrapAndThreadBadgeIsClickable(t *testing.T) {
	m := exampleModel()
	m.users["bob"] = chatto.Profile{ID: "bob", Login: "bob"}
	m.page.Messages = []chatto.Message{metadataMessage()}
	m.renderMessages()
	text := ansi.Strip(m.main.View())
	for _, label := range []string{"3 replies", "edited", "pinned", "👍 2", ":custom_party: 1"} {
		if !strings.Contains(text, label) {
			t.Fatalf("missing metadata %q", label)
		}
	}
	for _, width := range []int{20, 35, 70} {
		lines, hits := m.messageMeta(metadataMessage(), width, 23, true)
		if len(lines) == 0 || len(hits) != 1 {
			t.Fatal("missing metadata or thread hit")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("metadata overflow at width %d", width)
			}
		}
	}
	left, _, _ := m.columns()
	hit := m.mainRows[0].Meta[0]
	m.compose.SetValue("keep draft")
	m.busy = true // Do not issue requests from a synthetic input test.
	m.Update(tea.MouseClickMsg{X: left + 1 + hit.x, Y: 2 + hit.y - m.main.YOffset(), Button: tea.MouseLeft})
	if m.root != "m" || m.focus != 2 || m.drafts["room/"] != "keep draft" {
		t.Fatal("thread badge did not open its thread safely")
	}
}

func TestEmptyThreadsEchoesAndPlainMessages(t *testing.T) {
	m := exampleModel()
	lines, hits := m.messageMeta(chatto.Message{ID: "empty", Thread: &chatto.ThreadSummary{}}, 50, 23, false)
	if len(hits) != 1 || hits[0].root != "empty" || !strings.Contains(ansi.Strip(strings.Join(lines, "")), "Thread") {
		t.Fatal("empty established thread not marked")
	}
	_, hits = m.messageMeta(chatto.Message{ID: "echo", EchoFromThreadRootEventID: "source"}, 50, 23, false)
	if len(hits) != 1 || hits[0].root != "source" {
		t.Fatal("echo badge points to wrong thread")
	}
	lines, _ = m.messageMeta(chatto.Message{ID: "plain"}, 50, 23, false)
	if len(lines) != 0 {
		t.Fatal("plain message gained empty metadata line")
	}
	if strings.Contains(displayReaction("bad\x1b[31m"), "\x1b") {
		t.Fatal("reaction injected terminal control")
	}
}

func TestTinyAvatarsUseKittyCellsWithTextFallback(t *testing.T) {
	m := exampleModel()
	m.users["alice"] = chatto.Profile{ID: "alice", Login: "alice", AvatarURL: "/assets/avatar"}
	m.images.enabled = true
	m.images.entries = map[string]*terminalImage{avatarKey("alice", "/assets/avatar"): {id: 1234, width: 64, height: 64, placements: map[int]bool{}}}
	avatar := m.participantAvatar("alice")
	if !strings.ContainsRune(avatar, kitty.Placeholder) || ansi.StringWidth(avatar) != 2 {
		t.Fatal("avatar is not two terminal cells")
	}
	lines, _ := m.messageMeta(metadataMessage(), 70, 23, true)
	if !strings.ContainsRune(strings.Join(lines, ""), kitty.Placeholder) {
		t.Fatal("selection stripped avatar cells")
	}
	m.images.enabled = false
	if got := ansi.Strip(m.participantAvatar("alice")); got != "A " {
		t.Fatalf("unexpected avatar fallback %q", got)
	}
}
