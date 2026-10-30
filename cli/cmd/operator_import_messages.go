package cmd

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	op "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
)

// importMessageBatch owns source references. Only mapped resources and earlier
// operation keys cross the Operator API. Each message and reaction retains the
// same journal key and server receipt arguments as the individual-call path.
func (m *cliArchiveImport) importMessageBatch(ctx context.Context, source []interchange.Event) error {
	request := &op.ImportMessagesRequest{}
	local := map[string]string{}
	postKeys := make([]string, len(source))
	reactionKeys := make([][]string, len(source))
	for i, e := range source {
		d := e.Data
		key := "event/" + importRef(e.ID)
		postKeys[i] = key + "/post"
		at := archiveTime(e.OccurredAt)
		if e.Type == "message.snapshot" {
			at = archiveTime(d.CreatedAt)
		}
		message := &op.ImportMessageRequest{RoomId: m.rooms[e.RoomID], AuthorId: m.users[d.AuthorID], CreatedAt: at, EditedAt: archiveTime(d.EditedAt), Body: d.Content.Body.Text, Deleted: d.Content.Status == "deleted", DeletedAt: archiveTime(d.DeletedAt), OperationKey: m.progress.operation(postKeys[i])}
		if message.Deleted {
			message.EditedAt = nil
		}
		if e.ActorID != nil {
			message.SourceActorId = m.users[*e.ActorID]
		}
		item := &op.HistoricalMessageOperation{Message: message}
		if d.ThreadID != nil {
			message.ThreadRootEventId = m.messages[*d.ThreadID]
			if message.ThreadRootEventId == "" {
				item.ThreadRootOperationKey = local[*d.ThreadID]
				if item.ThreadRootOperationKey == "" {
					return errors.New("import: unresolved batch thread root")
				}
			}
		}
		if d.ReplyTo != nil {
			message.InReplyTo = m.messages[*d.ReplyTo]
			if message.InReplyTo == "" {
				item.InReplyToOperationKey = local[*d.ReplyTo]
				if item.InReplyToOperationKey == "" {
					return errors.New("import: unresolved batch reply target")
				}
			}
		}
		local[d.MessageID] = message.OperationKey
		final := map[string]map[string]bool{}
		for _, reaction := range d.Reactions {
			final[reaction.Emoji] = map[string]bool{}
			for _, user := range reaction.UserIDs {
				final[reaction.Emoji][m.users[user]] = true
			}
		}
		for _, emoji := range sortedImportKeys(final) {
			for _, user := range sortedImportKeys(final[emoji]) {
				rkey := key + "/reaction/" + emoji + "/" + user
				reactionKeys[i] = append(reactionKeys[i], rkey)
				item.Reactions = append(item.Reactions, &op.HistoricalMessageReaction{OperationKey: m.progress.operation(rkey), ActorId: message.SourceActorId, Change: &op.HistoricalReactionChange{UserId: user, Emoji: emoji, Present: true}})
			}
		}
		request.Operations = append(request.Operations, item)
	}
	if !m.legacyMessageBatches {
		response, err := m.messageAPI.ImportMessages(ctx, operatorRequest(request))
		if connect.CodeOf(err) == connect.CodeUnimplemented {
			m.legacyMessageBatches = true
		} else if err != nil {
			return err
		} else {
			if len(response.Msg.Messages) != len(source) {
				return errors.New("import: incomplete message batch response")
			}
			for _, message := range response.Msg.Messages {
				if message.GetMessageId() == "" {
					return errors.New("import: missing message batch identity")
				}
			}
			for i, message := range response.Msg.Messages {
				m.messages[source[i].Data.MessageID] = message.MessageId
				if err := m.progress.putReplayable(postKeys[i], message.MessageId); err != nil {
					return err
				}
				for _, key := range reactionKeys[i] {
					if err := m.progress.putReplayable(key, "done"); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	// Resolve local references after each successful individual call. A partial
	// fallback failure resumes through the same per-operation receipt identities.
	byKey := map[string]string{}
	for i, item := range request.Operations {
		message := item.Message
		if item.ThreadRootOperationKey != "" {
			message.ThreadRootEventId = byKey[item.ThreadRootOperationKey]
		}
		if item.InReplyToOperationKey != "" {
			message.InReplyTo = byKey[item.InReplyToOperationKey]
		}
		response, err := m.messageAPI.ImportMessage(ctx, operatorRequest(message))
		if err != nil {
			return err
		}
		id := response.Msg.MessageId
		if id == "" {
			return errors.New("import: missing message identity")
		}
		byKey[message.OperationKey], m.messages[source[i].Data.MessageID] = id, id
		if err := m.progress.putReplayable(postKeys[i], id); err != nil {
			return err
		}
		for j, r := range item.Reactions {
			change := &op.ImportMessageChangeRequest{RoomId: message.RoomId, MessageId: id, ActorId: r.ActorId, OccurredAt: r.OccurredAt, Change: &op.ImportMessageChangeRequest_Reaction{Reaction: r.Change}}
			if err := m.messageChange(ctx, reactionKeys[i][j], change); err != nil {
				return err
			}
		}
	}
	return nil
}
