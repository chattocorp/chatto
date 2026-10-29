package cmd

import (
	"context"
	"fmt"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	op "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func archiveTime(s *string) *timestamppb.Timestamp {
	if s == nil {
		return nil
	}
	t, _ := time.Parse(time.RFC3339Nano, *s)
	return timestamppb.New(t)
}

func (m *cliArchiveImport) roomChange(ctx context.Context, key string, r *op.ImportRoomEventRequest) error {
	if m.progress.values[key] != "" {
		return nil
	}
	r.OperationKey = m.progress.operation(key)
	res, err := m.roomAPI.ImportRoomEvent(ctx, operatorRequest(r))
	if err != nil {
		return err
	}
	id := res.Msg.EventId
	if id == "" {
		id = "done" // Older servers and receipts still support reaction-free imports.
	}
	return m.progress.put(key, id)
}
func (m *cliArchiveImport) messageChange(ctx context.Context, key string, r *op.ImportMessageChangeRequest) error {
	if m.progress.values[key] != "" {
		return nil
	}
	r.OperationKey = m.progress.operation(key)
	if _, err := m.messageAPI.ImportMessageChange(ctx, operatorRequest(r)); err != nil {
		return err
	}
	return m.progress.put(key, "done")
}

// history rebuilds reference and membership indexes while replaying the source.
// Completed operations are skipped, but still contribute to reconciliation.
func (m *cliArchiveImport) history(ctx context.Context) error {
	members := map[string]map[string]bool{}
	reactions := map[string]map[string]map[string]bool{}
	err := m.archive.VisitEvents(func(e interchange.Event) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := "event/" + importRef(e.ID)
		room := m.rooms[e.RoomID]
		d := e.Data
		at := archiveTime(e.OccurredAt)
		actor := ""
		if e.ActorID != nil {
			actor = m.users[*e.ActorID]
		}
		if members[room] == nil {
			members[room] = map[string]bool{}
		}
		join := func(user string, present, hidden bool) error {
			r := &op.ImportRoomEventRequest{RoomId: room, ActorId: actor, OccurredAt: at, Hidden: hidden}
			if present {
				r.Change = &op.ImportRoomEventRequest_JoinedUserId{JoinedUserId: user}
			} else {
				r.Change = &op.ImportRoomEventRequest_LeftUserId{LeftUserId: user}
			}
			if err := m.roomChange(ctx, key+"/member/"+user, r); err != nil {
				return err
			}
			members[room][user] = present
			return nil
		}
		react := func(user, emoji string, present bool, when *timestamppb.Timestamp) error {
			if reactions[d.MessageID] == nil {
				reactions[d.MessageID] = map[string]map[string]bool{}
			}
			if reactions[d.MessageID][emoji] == nil {
				reactions[d.MessageID][emoji] = map[string]bool{}
			}
			r := &op.ImportMessageChangeRequest{RoomId: room, MessageId: m.messages[d.MessageID], ActorId: actor, OccurredAt: when, Change: &op.ImportMessageChangeRequest_Reaction{Reaction: &op.HistoricalReactionChange{UserId: user, Emoji: emoji, Present: present}}}
			if err := m.messageChange(ctx, key+"/reaction/"+emoji+"/"+user, r); err != nil {
				return err
			}
			reactions[d.MessageID][emoji][user] = present
			return nil
		}
		switch e.Type {
		case "message.posted", "message.snapshot", "message.edited":
			id := m.messages[d.MessageID]
			if id == "" {
				id = m.progress.values[key+"/post"]
				if id == "" {
					assets, err := m.messageAssets(ctx, d.MessageID, room, d.Content.AssetIDs)
					if err != nil {
						return err
					}
					created := at
					if e.Type == "message.snapshot" {
						created = archiveTime(d.CreatedAt)
					}
					r := &op.ImportMessageRequest{RoomId: room, AuthorId: m.users[d.AuthorID], CreatedAt: created, Body: d.Content.Body.Text, AttachmentAssetIds: assets, Deleted: d.Content.Status == "deleted", EditedAt: archiveTime(d.EditedAt), OperationKey: m.progress.operation(key + "/post")}
					r.SourceActorId = actor
					r.DeletedAt = archiveTime(d.DeletedAt)
					if r.Deleted {
						r.EditedAt = nil
					}
					if d.ThreadID != nil {
						r.ThreadRootEventId = m.messages[*d.ThreadID]
					}
					if d.ReplyTo != nil {
						r.InReplyTo = m.messages[*d.ReplyTo]
					}
					res, err := m.messageAPI.ImportMessage(ctx, operatorRequest(r))
					if err != nil {
						return err
					}
					id = res.Msg.MessageId
					if err = m.progress.put(key+"/post", id); err != nil {
						return err
					}
				}
				m.messages[d.MessageID] = id
			} else {
				r := &op.ImportMessageChangeRequest{RoomId: room, MessageId: id, ActorId: actor, OccurredAt: at}
				if d.Content.Status == "deleted" {
					r.Change = &op.ImportMessageChangeRequest_Deleted{Deleted: true}
					r.OccurredAt = archiveTime(d.DeletedAt)
				} else {
					assets, err := m.messageAssets(ctx, d.MessageID, room, d.Content.AssetIDs)
					if err != nil {
						return err
					}
					r.Change = &op.ImportMessageChangeRequest_Edit{Edit: &op.HistoricalMessageEdit{Body: d.Content.Body.Text, AttachmentAssetIds: assets}}
					if e.Type == "message.snapshot" {
						r.OccurredAt = archiveTime(d.EditedAt)
					}
				}
				if err := m.messageChange(ctx, key+"/content", r); err != nil {
					return err
				}
			}
			if e.Type == "message.snapshot" {
				final := map[string]map[string]bool{}
				for _, r := range d.Reactions {
					final[r.Emoji] = map[string]bool{}
					for _, u := range r.UserIDs {
						final[r.Emoji][m.users[u]] = true
					}
				}
				for _, emoji := range sortedImportKeys(reactions[d.MessageID]) {
					for _, user := range sortedImportKeys(reactions[d.MessageID][emoji]) {
						if reactions[d.MessageID][emoji][user] && !final[emoji][user] {
							if err := react(user, emoji, false, nil); err != nil {
								return err
							}
						}
					}
				}
				for _, emoji := range sortedImportKeys(final) {
					for _, user := range sortedImportKeys(final[emoji]) {
						if !reactions[d.MessageID][emoji][user] {
							if err := react(user, emoji, true, nil); err != nil {
								return err
							}
						}
					}
				}
			}
		case "message.deleted":
			return m.messageChange(ctx, key, &op.ImportMessageChangeRequest{RoomId: room, MessageId: m.messages[d.MessageID], ActorId: actor, OccurredAt: at, Change: &op.ImportMessageChangeRequest_Deleted{Deleted: true}})
		case "reaction.added", "reaction.removed":
			return react(m.users[d.UserID], d.Emoji, e.Type == "reaction.added", at)
		case "thread.created":
			return m.messageChange(ctx, key, &op.ImportMessageChangeRequest{RoomId: room, MessageId: m.messages[d.RootID], ActorId: actor, OccurredAt: at, Change: &op.ImportMessageChangeRequest_CreateThread{CreateThread: true}})
		case "room.member_joined", "room.member_left":
			if err := join(m.users[d.UserID], e.Type == "room.member_joined", false); err != nil {
				return err
			}
			id := m.progress.values[key+"/member/"+m.users[d.UserID]]
			if len(d.Reactions) > 0 && (id == "" || id == "done") {
				return fmt.Errorf("membership reactions require an event ID from ImportRoomEvent; upgrade the destination server; older receipts cannot supply this ID")
			}
			for _, reaction := range d.Reactions {
				// ImportReaction accepts names; interchange also permits Unicode.
				emoji, err := core.HistoricalReactionName(reaction.Emoji)
				if err != nil {
					return err
				}
				for _, user := range reaction.UserIDs {
					progressKey := key + "/reaction/" + reaction.Emoji + "/" + m.users[user]
					if m.progress.values[progressKey] != "" {
						continue
					}
					_, err := m.messageAPI.ImportReaction(ctx, operatorRequest(&op.ImportReactionRequest{RoomId: room, MessageEventId: id, UserId: m.users[user], Emoji: emoji}))
					if err != nil {
						return err
					}
					if err = m.progress.put(progressKey, "done"); err != nil {
						return err
					}
				}
			}
		case "room.membership_snapshot":
			final := map[string]bool{}
			for _, id := range d.UserIDs {
				final[m.users[id]] = true
			}
			for _, id := range sortedImportKeys(members[room]) {
				if members[room][id] && !final[id] {
					if err := join(id, false, true); err != nil {
						return err
					}
				}
			}
			for _, id := range sortedImportKeys(final) {
				if !members[room][id] {
					if err := join(id, true, true); err != nil {
						return err
					}
				}
			}
		default:
			r := &op.ImportRoomEventRequest{RoomId: room, ActorId: actor, OccurredAt: at}
			switch e.Type {
			case "room.name_changed":
				r.Change = &op.ImportRoomEventRequest_Name{Name: m.channel(d.Value)}
			case "room.topic_changed":
				r.Change = &op.ImportRoomEventRequest_Topic{Topic: d.Value}
			case "room.purpose_changed":
				r.Change = &op.ImportRoomEventRequest_Purpose{Purpose: d.Value}
			case "room.archived", "room.unarchived":
				r.Change = &op.ImportRoomEventRequest_Archived{Archived: e.Type == "room.archived"}
			}
			return m.roomChange(ctx, key, r)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, r := range m.archive.Rooms {
		for i, change := range []*op.ImportRoomEventRequest{
			{Change: &op.ImportRoomEventRequest_Name{Name: m.channel(r.Name)}},
			{Change: &op.ImportRoomEventRequest_Topic{Topic: r.Topic}},
			{Change: &op.ImportRoomEventRequest_Purpose{Purpose: r.Purpose}},
			{Change: &op.ImportRoomEventRequest_Archived{Archived: r.Archived}},
		} {
			change.RoomId = m.rooms[r.ID]
			change.Hidden = true
			if err := m.roomChange(ctx, "final/"+importRef(r.ID)+"/"+string(rune('0'+i)), change); err != nil {
				return err
			}
		}
	}
	return nil
}
