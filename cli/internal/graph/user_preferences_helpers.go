package graph

import (
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// protoSettingsToGQL converts a proto UserSettings to the GraphQL model.
func protoSettingsToGQL(s *corev1.ServerUserPreferences) *model.UserSettings {
	result := &model.UserSettings{
		TimeFormat: protoTimeFormatToGQL(s.TimeFormat),
	}
	if s.Timezone != nil {
		tz := *s.Timezone
		result.Timezone = &tz
	}
	return result
}

// protoTimeFormatToGQL converts a proto TimeFormat to the GraphQL TimeFormat.
func protoTimeFormatToGQL(tf corev1.TimeFormat) model.TimeFormat {
	switch tf {
	case corev1.TimeFormat_TIME_FORMAT_12H:
		return model.TimeFormatTwelveHour
	case corev1.TimeFormat_TIME_FORMAT_24H:
		return model.TimeFormatTwentyFourHour
	default:
		return model.TimeFormatUnspecified
	}
}

// gqlTimeFormatToProto converts a GraphQL TimeFormat to the proto TimeFormat.
func gqlTimeFormatToProto(tf model.TimeFormat) corev1.TimeFormat {
	switch tf {
	case model.TimeFormatTwelveHour:
		return corev1.TimeFormat_TIME_FORMAT_12H
	case model.TimeFormatTwentyFourHour:
		return corev1.TimeFormat_TIME_FORMAT_24H
	default:
		return corev1.TimeFormat_TIME_FORMAT_UNSPECIFIED
	}
}
