package core

import (
	"context"
	"time"
)

const notificationTTL = 90 * 24 * time.Hour

func (c *ChattoCore) suppressesNotificationAlertsForPresence(ctx context.Context, userID string) bool {
	status, err := c.GetUserPresence(ctx, userID)
	if err != nil {
		c.logger.Warn("Failed to get presence for notification suppression",
			"user_id", userID, "error", err)
		return false
	}
	return status == PresenceStatusDoNotDisturb
}
