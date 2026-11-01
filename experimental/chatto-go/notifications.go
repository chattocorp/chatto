package chatto

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/httpjson"
)

// NotificationMessageReference is an exact message destination supplied by Chatto.
type NotificationMessageReference struct {
	Room              Room   `json:"room"`
	EventID           string `json:"eventId"`
	ThreadRootEventID string `json:"threadRootEventId"`
}

// NotificationOccurrence retains unknown signal kinds without inventing a target.
type NotificationOccurrence struct {
	ID             string                     `json:"id"`
	Unread         bool                       `json:"unread"`
	AttentionLevel string                     `json:"attentionLevel"`
	ExpiresAt      time.Time                  `json:"expiresAt"`
	Signal         map[string]json.RawMessage `json:"signal"`
}

// MessageTarget returns the destination of a known message signal. Invitations
// and unknown future signals have no message destination.
func (n NotificationOccurrence) MessageTarget() (NotificationMessageReference, bool) {
	for _, kind := range []string{"directMessageReceived", "directMentionReceived", "replyReceived", "roleMentionReceived", "hereMentionReceived", "allMentionReceived", "followedThreadActivity", "followedRoomActivity", "reactionReceived", "roomMessageReceived"} {
		if raw, ok := n.Signal[kind]; ok {
			var payload struct {
				Message NotificationMessageReference `json:"message"`
			}
			if json.Unmarshal(raw, &payload) == nil && payload.Message.Room.ID != "" && payload.Message.EventID != "" {
				return payload.Message, true
			}
		}
	}
	return NotificationMessageReference{}, false
}

// NotificationPage preserves all returned occurrences, including unknown kinds.
// Use the number of returned occurrences to advance the live offset.
type NotificationPage struct {
	Occurrences []NotificationOccurrence `json:"occurrences"`
	Page        struct {
		HasMore bool `json:"hasMore"`
	} `json:"page"`
}

// ListNotifications reads a newest-first live offset page. Zero limit uses the
// server default. Counts and occurrences can change between successive reads.
func (c *Client) ListNotifications(ctx context.Context, offset, limit int) (NotificationPage, error) {
	var page NotificationPage
	if offset < 0 || limit < 0 || limit > 100 {
		return page, errors.New("invalid notification pagination")
	}
	err := c.call(ctx, "NotificationService/ListNotificationOccurrences", map[string]any{"page": map[string]int{"offset": offset, "limit": limit}}, &page)
	return page, err
}

// NotificationCount counts exact unread occurrences, not messages or rooms.
// Important is a subset of Unread; Unread-Important is ordinary attention.
type NotificationCount struct {
	RoomID    string `json:"roomId"`
	Unread    int    `json:"unreadCount"`
	Important int    `json:"importantUnreadCount"`
}

// NotificationCounts reads authoritative room totals independently of the
// occurrence page size. The expiry marks when these totals need another read.
func (c *Client) NotificationCounts(ctx context.Context) (map[string]NotificationCount, time.Time, error) {
	var out struct {
		Rooms      []NotificationCount `json:"roomUnreadCounts"`
		NextExpiry time.Time           `json:"nextExpiryAt"`
	}
	err := c.call(ctx, "NotificationService/ListNotificationOccurrences", map[string]any{"page": map[string]int{"limit": 1}}, &out)
	if err != nil {
		return nil, time.Time{}, err
	}
	counts := map[string]NotificationCount{}
	for _, count := range out.Rooms {
		count.Unread = max(0, count.Unread)
		count.Important = max(0, min(count.Unread, count.Important))
		if count.RoomID != "" && count.Unread > 0 {
			counts[count.RoomID] = count
		}
	}
	return counts, out.NextExpiry, nil
}

func (s *Server) refreshNotifications(ctx context.Context, c *Client) error {
	if !s.options.IncludeNotifications {
		return nil
	}
	counts, expiry, err := c.NotificationCounts(ctx)
	if err != nil {
		var status httpjson.StatusError
		if !errors.As(err, &status) || (status.Code != 404 && status.Code != 501) {
			return err
		}
		// Counts are optional on older servers; messaging remains available.
	}
	s.mu.Lock()
	s.state.Notifications, s.state.NotificationExpiry = counts, expiry
	s.mu.Unlock()
	return nil
}
