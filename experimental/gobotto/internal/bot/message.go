package bot

import chatto "github.com/chattocorp/chatto/experimental/chatto-go"

// modelMessage is Gobotto's explicit text-context contract, independent of SDK
// resource growth. Keep field order and JSON tags stable: existing fingerprints
// hash this shape. Attachments, signed URLs, and display metadata are not inputs
// to this text-only agent and must not enter prompts or edit comparisons.
type modelMessage struct {
	ID                string `json:"id"`
	RoomID            string `json:"roomId"`
	ActorID           string `json:"actorId"`
	Body              string `json:"body"`
	ThreadRootEventID string `json:"threadRootEventId"`
	DeletedAt         string `json:"deletedAt,omitempty"`
}

func messageForModel(m chatto.Message) modelMessage {
	return modelMessage{ID: m.ID, RoomID: m.RoomID, ActorID: m.ActorID, Body: m.Body,
		ThreadRootEventID: m.ThreadRootEventID, DeletedAt: m.DeletedAt}
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

// messageForInbox retains only the same stable text fields needed after restart.
// Construct a fresh SDK value so newly added SDK fields cannot leak into storage.
func messageForInbox(m chatto.Message) chatto.Message {
	return chatto.Message{ID: m.ID, RoomID: m.RoomID, ActorID: m.ActorID, Body: m.Body,
		ThreadRootEventID: m.ThreadRootEventID, DeletedAt: m.DeletedAt}
}
