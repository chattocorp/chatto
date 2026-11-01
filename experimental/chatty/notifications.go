package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

type notificationJump struct {
	generation int
	target     chatto.NotificationMessageReference
	snapshot   snapshot
	err        error
}

// badgeAt uses the same right alignment, padding, and count widths as rendering.
// The separator and the gap between badges are not click targets.
func badgeAt(x, width, total, important int) (bool, bool) {
	important = max(0, min(total, important))
	ordinary := max(0, total-important)
	size := func(n int) int {
		if n > 99 {
			return 5
		}
		return len(strconv.Itoa(n)) + 2
	}
	end := width
	if important > 0 {
		start := end - size(important)
		if x >= start && x < end {
			return true, true
		}
		end = start - 1
	}
	if ordinary > 0 && x >= end-size(ordinary) && x < end {
		return false, true
	}
	return false, false
}

func (m *model) jumpNotification(room string, important bool) tea.Cmd {
	if m.busy {
		return nil
	}
	m.busy = true
	m.generation++
	generation := m.generation
	m.status = "Finding notification…"
	m.animationGeneration++
	return tea.Batch(animate(m.animationGeneration), func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.e.ctx, 30*time.Second)
		defer cancel()
		target, state, err := m.e.notificationDestination(ctx, room, important)
		return notificationJump{generation: generation, target: target, snapshot: state, err: err}
	})
}

// Find the newest unread message occurrence for the clicked attention class.
// The global list is paginated: a badge can refer to an older, unloaded page.
func (e *engine) notificationDestination(ctx context.Context, room string, important bool) (chatto.NotificationMessageReference, snapshot, error) {
	var target chatto.NotificationMessageReference
	s := e.connection()
	if err := s.WaitReady(ctx); err != nil {
		return target, snapshot{}, err
	}
	generation := s.ReadState().Generation
	e.mu.Lock()
	c, err := e.client()
	e.mu.Unlock()
	if err != nil {
		return target, snapshot{}, err
	}
	for offset := 0; ; {
		page, err := c.ListNotifications(ctx, offset, 100)
		if err != nil {
			return target, snapshot{}, err
		}
		for _, n := range page.Occurrences {
			if !n.Unread || (!n.ExpiresAt.IsZero() && !time.Now().Before(n.ExpiresAt)) || (n.AttentionLevel == "NOTIFICATION_ATTENTION_LEVEL_IMPORTANT") != important {
				continue
			}
			if candidate, ok := n.MessageTarget(); ok && candidate.Room.ID == room {
				target = candidate
				break
			}
		}
		if target.EventID != "" {
			break
		}
		if !page.Page.HasMore || len(page.Occurrences) == 0 {
			return target, snapshot{}, errors.New("no unread message notification remains for this badge")
		}
		offset += len(page.Occurrences)
	}
	if s.ReadState().Generation != generation {
		return target, snapshot{}, errors.New("notification access changed; try again")
	}
	page, err := s.LoadTimelineAround(ctx, room, target.ThreadRootEventID, target.EventID, 50)
	if err != nil {
		return target, snapshot{}, err
	}
	found := false
	for _, msg := range page.Messages {
		if msg.ID == target.EventID {
			found = true
			break
		}
	}
	if !found {
		return target, snapshot{}, errors.New("notification message is no longer available")
	}
	if target.ThreadRootEventID != "" {
		if _, err := s.LoadTimelineWithOptions(ctx, room, "", chatto.TimelineOptions{Limit: 50}); err != nil {
			return target, snapshot{}, err
		}
	}
	state := e.current(room, target.ThreadRootEventID)
	if state.ServerGeneration != generation || !state.Ready {
		return target, snapshot{}, errors.New("notification access changed; try again")
	}
	return target, state, nil
}

func (m *model) acceptNotificationJump(v notificationJump) tea.Cmd {
	m.busy = false
	if v.generation != m.generation {
		return m.reload("")
	}
	if v.err != nil {
		m.status = clean(v.err.Error())
		return nil
	}
	if v.snapshot.ServerGeneration != m.e.current(m.room, m.root).ServerGeneration {
		m.status = "Notification access changed; try again"
		return m.reload("")
	}
	m.saveDraft()
	m.room, m.root = v.target.Room.ID, v.target.ThreadRootEventID
	m.focus = 1
	if m.root != "" {
		m.focus = 2
	}
	m.restoreDraft()
	_, cmd := m.update(loaded{Generation: m.generation, Snapshot: v.snapshot})
	p := m.timeline(m.root != "")
	for i, msg := range p.page.Messages {
		if msg.ID == v.target.EventID {
			*p.selected = i
			break
		}
	}
	m.history = true
	m.renderMessages()
	// Center the exact source, rather than stopping at the first visible edge.
	if *p.selected < len(*p.rows) && *p.selected >= 0 {
		p.view.SetYOffset(max(0, (*p.rows)[*p.selected].Start-p.view.Height()/2))
	}
	m.status = "Notification source · Alt+R returns to latest messages"
	return cmd
}
