package bot

import chatto "github.com/chattocorp/chatto/experimental/chatto-go"

// modelMessage is Gobotto's explicit text-context contract, independent of SDK
// resource growth. Keep field order and JSON tags stable: existing fingerprints
// hash this shape. Only stable image references enter this projection; signed
// URLs and display metadata never enter edit comparisons. Verified
// participant names are supplied separately through participantProfile.
type modelMessage struct {
	ID                string       `json:"id"`
	RoomID            string       `json:"roomId"`
	ActorID           string       `json:"actorId"`
	Body              string       `json:"body"`
	ThreadRootEventID string       `json:"threadRootEventId"`
	DeletedAt         string       `json:"deletedAt,omitempty"`
	Images            []modelImage `json:"images,omitempty"`
}

// modelImage identifies inspectable content without retaining access capabilities.
type modelImage struct {
	AssetID     string `json:"assetId"`
	ContentType string `json:"contentType"`
}

func messageForModel(m chatto.Message) modelMessage {
	return modelMessage{ID: m.ID, RoomID: m.RoomID, ActorID: m.ActorID, Body: m.Body,
		ThreadRootEventID: m.ThreadRootEventID, DeletedAt: m.DeletedAt, Images: imageReferences(m)}
}

func imageReferences(m chatto.Message) []modelImage {
	var images []modelImage
	for _, a := range m.Attachments {
		if a.ID != "" && supportedImage(a.ContentType) {
			images = append(images, modelImage{AssetID: a.ID, ContentType: a.ContentType})
		}
	}
	return images
}

func messagesForModel(messages []chatto.Message) []modelMessage {
	if messages == nil {
		return nil
	}
	result := make([]modelMessage, len(messages))
	for i, m := range messages {
		result[i] = messageForModel(m)
	}
	return result
}

// messageForInbox retains stable text and image references needed after restart.
// Construct a fresh SDK value so newly added SDK fields cannot leak into storage.
func messageForInbox(m chatto.Message) chatto.Message {
	result := chatto.Message{ID: m.ID, RoomID: m.RoomID, ActorID: m.ActorID, Body: m.Body,
		ThreadRootEventID: m.ThreadRootEventID, DeletedAt: m.DeletedAt}
	for _, ref := range imageReferences(m) {
		result.Attachments = append(result.Attachments, chatto.Attachment{ID: ref.AssetID, ContentType: ref.ContentType})
	}
	return result
}
