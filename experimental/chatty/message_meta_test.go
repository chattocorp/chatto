package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestDoubleClickThreadedMessage(t *testing.T) {
	for _, scenario := range []string{"thread", "echo", "plain", "slow", "other-message", "intervening-click", "key", "thread-pane"} {
		t.Run(scenario, func(t *testing.T) {
			m := exampleModel()
			message := metadataMessage()
			if scenario == "plain" {
				message.Thread = nil
			}
			if scenario == "echo" {
				message.Thread = nil
				message.EchoFromThreadRootEventID = "m"
			}
			m.page.Messages = []chatto.Message{message}
			if scenario == "thread-pane" {
				m.root = "existing"
				m.thread.Messages = []chatto.Message{{ID: "first", Body: "First"}, message}
			}
			m.layout()
			left, center, _ := m.columns()
			click := tea.MouseClickMsg{X: left + 5, Y: 2 + m.mainRows[0].Start, Button: tea.MouseLeft}
			if scenario == "thread-pane" {
				click.X = left + center + 5
				click.Y = 2 + m.sideRows[1].Start
			}
			m.compose.SetValue("preserve draft")
			m.busy = true // Do not execute network commands in this input test.
			initialRoot := m.root
			m.Update(click)
			if m.root != initialRoot {
				t.Fatal("single click opened thread")
			}
			m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft})
			switch scenario {
			case "slow":
				m.lastMessageClick.at = time.Now().Add(-time.Second)
			case "other-message":
				m.page.Messages[0].ID = "replacement"
			case "intervening-click":
				m.Update(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
			case "key":
				m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
			}
			m.Update(click)
			wantOpen := scenario == "thread" || scenario == "echo" || scenario == "thread-pane"
			if wantOpen {
				if m.root != "m" || m.focus != 2 || m.drafts["room/"] != "preserve draft" {
					t.Fatal("double-click failed to open thread or preserve draft")
				}
			} else if m.root != initialRoot {
				t.Fatal("unrelated clicks opened thread")
			}
		})
	}
}

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
