package model

import (
	"fmt"
	"io"
	"strconv"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// NotificationLevel controls how a user receives notifications for a space or room.
type NotificationLevel string

const (
	NotificationLevelDefault     NotificationLevel = "DEFAULT"
	NotificationLevelMuted       NotificationLevel = "MUTED"
	NotificationLevelNormal      NotificationLevel = "NORMAL"
	NotificationLevelAllMessages NotificationLevel = "ALL_MESSAGES"
)

var AllNotificationLevel = []NotificationLevel{
	NotificationLevelDefault,
	NotificationLevelMuted,
	NotificationLevelNormal,
	NotificationLevelAllMessages,
}

func (e NotificationLevel) IsValid() bool {
	switch e {
	case NotificationLevelDefault, NotificationLevelMuted, NotificationLevelNormal, NotificationLevelAllMessages:
		return true
	}
	return false
}

func (e NotificationLevel) String() string {
	return string(e)
}

func (e *NotificationLevel) UnmarshalGQL(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = NotificationLevel(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid NotificationLevel", str)
	}
	return nil
}

func (e NotificationLevel) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// DMMessageNotificationItem represents a DM message notification.
type DMMessageNotificationItem struct {
	ID        string                 `json:"id"`
	CreatedAt *timestamppb.Timestamp `json:"createdAt"`
	Actor     *corev1.User           `json:"actor"`
	Summary   string                 `json:"summary"`
	Room      *corev1.Room           `json:"room"`

	// Internal fields for resolvers (not exposed in GraphQL)
	ActorID string `json:"-"`
	RoomID  string `json:"-"`
}

func (DMMessageNotificationItem) IsNotificationItem() {}

// MentionNotificationItem represents a mention notification.
type MentionNotificationItem struct {
	ID        string                 `json:"id"`
	CreatedAt *timestamppb.Timestamp `json:"createdAt"`
	Actor     *corev1.User           `json:"actor"`
	Summary   string                 `json:"summary"`
	Room      *corev1.Room           `json:"room"`
	EventID            string                 `json:"eventId"`
	ThreadRootEventID  *string                `json:"threadRootEventId,omitempty"`

	// Internal fields for resolvers (not exposed in GraphQL)
	ActorID string `json:"-"`
	SpaceID string `json:"-"` // Persisted on the proto; carried through for room-kind disambiguation.
	RoomID  string `json:"-"`
}

func (MentionNotificationItem) IsNotificationItem() {}

// ReplyNotificationItem represents a reply notification.
type ReplyNotificationItem struct {
	ID          string                 `json:"id"`
	CreatedAt   *timestamppb.Timestamp `json:"createdAt"`
	Actor       *corev1.User           `json:"actor"`
	Summary     string                 `json:"summary"`
	Room        *corev1.Room           `json:"room"`
	EventID            string                 `json:"eventId"`
	InReplyToID        string                 `json:"inReplyToId"`
	ThreadRootEventID  *string                `json:"threadRootEventId,omitempty"`

	// Internal fields for resolvers (not exposed in GraphQL)
	ActorID string `json:"-"`
	SpaceID string `json:"-"` // Persisted on the proto; carried through for room-kind disambiguation.
	RoomID  string `json:"-"`
}

func (ReplyNotificationItem) IsNotificationItem() {}

// RoomMessageNotificationItem represents a room message notification (ALL_MESSAGES level).
type RoomMessageNotificationItem struct {
	ID        string                 `json:"id"`
	CreatedAt *timestamppb.Timestamp `json:"createdAt"`
	Actor     *corev1.User           `json:"actor"`
	Summary   string                 `json:"summary"`
	Room      *corev1.Room           `json:"room"`
	EventID   string                 `json:"eventId"`

	// Internal fields for resolvers (not exposed in GraphQL)
	ActorID string `json:"-"`
	RoomID  string `json:"-"`
}

func (RoomMessageNotificationItem) IsNotificationItem() {}
