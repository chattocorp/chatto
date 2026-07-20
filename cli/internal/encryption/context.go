package encryption

import (
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MessageBodyAAD returns the authenticated context for a persisted v2 message
// body. Projections that decrypt EVT bodies must use this exact construction.
func MessageBodyAAD(eventID, bodyEventID, roomID, authorID string, epoch int32) []byte {
	if bodyEventID == "" {
		return []byte(fmt.Sprintf("chatto:message-body-context:v2\x00event_type=message_body\x00event_id=%s\x00room_id=%s\x00author_id=%s\x00content_key_epoch=%d", eventID, roomID, authorID, epoch))
	}
	return []byte(fmt.Sprintf("chatto:message-body-context:v2\x00event_type=message_body\x00event_id=%s\x00body_event_id=%s\x00room_id=%s\x00author_id=%s\x00content_key_epoch=%d", eventID, bodyEventID, roomID, authorID, epoch))
}

// UserDEKAAD returns the authenticated context used to wrap a persisted user
// data-encryption key, including compatibility with legacy unspecified-purpose
// content keys.
func UserDEKAAD(userID string, purpose corev1.UserDEKPurpose, epoch int32) []byte {
	if purpose == corev1.UserDEKPurpose_USER_DEK_PURPOSE_UNSPECIFIED {
		return []byte(fmt.Sprintf("chatto:content-key-context:v2\x00user_id=%s\x00epoch=%d", userID, epoch))
	}
	return []byte(fmt.Sprintf("chatto:user-dek-context:v1\x00user_id=%s\x00purpose=%d\x00epoch=%d", userID, purpose, epoch))
}
