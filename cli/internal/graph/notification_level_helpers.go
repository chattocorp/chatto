package graph

import (
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// protoNotificationLevelToGQL converts a proto NotificationLevel to the GraphQL enum.
func protoNotificationLevelToGQL(level corev1.NotificationLevel) model.NotificationLevel {
	switch level {
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return model.NotificationLevelMuted
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return model.NotificationLevelNormal
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		return model.NotificationLevelAllMessages
	default:
		return model.NotificationLevelDefault
	}
}

// gqlNotificationLevelToProto converts a GraphQL NotificationLevel to the proto enum.
func gqlNotificationLevelToProto(level model.NotificationLevel) corev1.NotificationLevel {
	switch level {
	case model.NotificationLevelMuted:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED
	case model.NotificationLevelNormal:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	case model.NotificationLevelAllMessages:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES
	default:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED
	}
}
