package chatto

import (
	"context"
	"errors"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/httpjson"
)

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
