package core

import corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"

type assetMessageRef struct {
	roomID         string
	messageEventID string
	authorID       string
}

// MessageAssetRef identifies the message, room, and asset in one projected
// message-to-asset ownership relationship.
type MessageAssetRef struct {
	RoomID         string
	MessageEventID string
	AssetID        string
}

// VideoAttachmentManifest is the projection's current processing state for one
// original video attachment. Started fires when processing is enqueued;
// Succeeded or Failed fires on terminal outcome.
type VideoAttachmentManifest struct {
	Started   *corev1.AssetProcessingStartedEvent
	Succeeded *corev1.AssetProcessingSucceededEvent
	Failed    *corev1.AssetProcessingFailedEvent
}

// VideoProcessingRequest describes an original video/GIF attachment embedded
// in a durable MessageBodyEvent that does not yet have a projected manifest.
type VideoProcessingRequest struct {
	RoomID         string
	MessageEventID string
	Attachment     *corev1.Attachment
}

// ownedAssetIDsFromBody returns the asset IDs a message body references,
// preferring the current asset_ids list and falling back to the legacy embedded
// attachments slice.
func ownedAssetIDsFromBody(body *corev1.MessageBody) []string {
	if body == nil {
		return nil
	}
	if ids := body.GetAssetIds(); len(ids) > 0 {
		return ids
	}
	atts := body.GetAttachments()
	out := make([]string, 0, len(atts))
	for _, att := range atts {
		if id := att.GetId(); id != "" {
			out = append(out, id)
		}
	}
	return out
}
