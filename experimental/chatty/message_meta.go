package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
	"github.com/yuin/goldmark-emoji/definition"
)

type metaHit struct {
	x, y, width int
	root        string
}

var reactionEmoji = definition.Github()

func displayReaction(key string) string {
	if emoji, ok := reactionEmoji.Get(key); ok && emoji.IsUnicode() {
		return string(emoji.Unicode)
	}
	return ":" + oneLine(key) + ":"
}

func (m *model) participantAvatar(id string) string {
	u := m.users[id]
	if entry := m.images.entries[avatarKey(id, u.AvatarURL)]; m.images.enabled && entry != nil && !entry.loading && !entry.failed {
		return m.images.imageCells(entry, 2, 1)[0]
	}
	initial := "?"
	if name := []rune(strings.TrimPrefix(m.userName(id), "@")); len(name) > 0 {
		initial = strings.ToUpper(string(name[0]))
	}
	return nickColor(id).Background(lipgloss.Color("#30434B")).Render(ansi.Truncate(initial, 1, "") + " ")
}

// messageMeta packs complete badges beneath the body. Thread hit targets cover
// only the thread badge; reactions and status chips do not imply mutation tools.
func (m *model) messageMeta(msg chatto.Message, width, gutter int, selected bool) ([]string, []metaHit) {
	type badge struct {
		text, root string
		own        bool
	}
	var badges []badge
	indent := min(gutter, max(0, width-20))
	available := max(1, width-indent)
	if msg.Thread != nil || msg.EchoFromThreadRootEventID != "" {
		label := "↳ Thread"
		if thread := msg.Thread; thread != nil {
			if thread.ReplyCount == 1 {
				label = "↳ 1 reply"
			} else if thread.ReplyCount > 1 {
				label = fmt.Sprintf("↳ %d replies", thread.ReplyCount)
			}
			shown := 0
			for _, id := range thread.ParticipantPreviewUserIDs[:min(3, len(thread.ParticipantPreviewUserIDs))] {
				if ansi.StringWidth(label)+5 > available-2 {
					break
				}
				label += " " + m.participantAvatar(id)
				shown++
			}
			if extra := thread.ParticipantCount - shown; extra > 0 {
				suffix := fmt.Sprintf(" +%d", extra)
				if ansi.StringWidth(label+suffix) <= available-2 {
					label += suffix
				}
			}
			if unread := thread.ViewerState.HasUnreadReplies; unread != nil && *unread {
				label += " ●"
			}
		}
		badges = append(badges, badge{text: label, root: msg.ThreadRootID()})
	}
	if msg.Pinned {
		badges = append(badges, badge{text: "◆ pinned"})
	}
	if msg.UpdatedAt != "" {
		badges = append(badges, badge{text: "✎ edited"})
	}
	for _, r := range msg.Reactions {
		if r.Count > 0 {
			badges = append(badges, badge{text: fmt.Sprintf("%s %d", displayReaction(r.Emoji), r.Count), own: r.HasReacted})
		}
	}
	if len(badges) == 0 {
		return nil, nil
	}
	var lines []string
	var hits []metaHit
	line := strings.Repeat(" ", indent)
	used := 0
	for _, b := range badges {
		text := " " + ansi.Truncate(b.text, max(1, available-2), "…") + " "
		size := ansi.StringWidth(text)
		if used > 0 && used+1+size > available {
			lines = append(lines, line)
			line = strings.Repeat(" ", indent)
			used = 0
		}
		if used > 0 {
			line += " "
			used++
		}
		if b.root != "" {
			hits = append(hits, metaHit{x: indent + used, y: len(lines), width: size, root: b.root})
		}
		bg, fg := "#203340", "#94A3B8"
		if b.root != "" {
			fg = "#7DD3FC"
		}
		if b.own {
			bg, fg = "#285465", "#E0F2FE"
		}
		line += lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg)).Render(text)
		used += size
	}
	lines = append(lines, line)
	if selected {
		for i := range lines {
			lines[i] = lipgloss.NewStyle().Width(width).Background(lipgloss.Color("#193D48")).Render(lines[i])
		}
	}
	return lines, hits
}
