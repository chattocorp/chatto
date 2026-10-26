package core

import (
	"context"
	"sort"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/interchange"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type importedMessage struct {
	author    string
	created   *timestamppb.Timestamp
	reactions map[string]map[string]bool
}

// history builds native facts in source order. Intermediate bodies use normal
// message encryption. Final snapshots reconcile state without timeline activity.
// Its in-memory state is reconstructed from the immutable archive on each retry.
func (m *archiveImport) history(ctx context.Context) ([][]evtstream.BatchEntry, error) {
	var batches [][]evtstream.BatchEntry
	messages := map[string]*importedMessage{}
	threads := map[string]bool{}
	members := map[string]map[string]bool{}
	sourceForAccount := map[string]string{}
	for _, id := range sortedImportKeys(m.users) {
		if sourceForAccount[m.users[id]] == "" {
			sourceForAccount[m.users[id]] = id
		}
	}
	metadata := map[string]interchange.Room{}
	for _, room := range m.archive.Rooms {
		metadata[room.ID] = room
		members[room.ID] = map[string]bool{}
	}
	for _, source := range m.archive.Events {
		roomID := m.rooms[source.RoomID]
		d := source.Data
		when := importTime(source.OccurredAt)
		var batch []evtstream.BatchEntry
		add := func(e *evtv1.Event, hidden bool, at *timestamppb.Timestamp, unknown bool) {
			e.CreatedAt = at
			e.ImportContext = m.context(hidden, unknown)
			if source.ActorID != nil {
				actor := m.users[*source.ActorID]
				e.ImportContext.SourceActorId = &actor
				e.ActorId = actor
			}
			e = newEvent(SystemActorID, e)
			batch = append(batch, evtstream.BatchEntry{Subject: evtstream.RoomAggregate(roomID).SubjectFor(e), Event: e})
		}
		join := func(user string, present, hidden bool) {
			e := &evtv1.Event{ActorId: m.users[user]}
			if present {
				e.Event = &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID}}
			} else {
				e.Event = &evtv1.Event_UserLeftRoom{UserLeftRoom: &evtv1.UserLeftRoomEvent{RoomId: roomID}}
			}
			add(e, hidden, when, source.OccurredAt == nil)
			// newEvent uses the operation actor; native membership identifies the
			// member in the envelope, with source attribution in ImportContext.
			batch[len(batch)-1].Event.ActorId = m.users[user]
			members[source.RoomID][m.users[user]] = present
		}
		react := func(messageID, user, emoji string, present bool, unknown bool) {
			nativeEmoji, _ := importEmoji(emoji)
			e := &evtv1.Event{}
			if present {
				e.Event = &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: roomID, MessageEventId: m.nativeID("message", messageID), UserId: m.users[user], Emoji: nativeEmoji, HistoricalImport: true}}
			} else {
				e.Event = &evtv1.Event_ReactionRemoved{ReactionRemoved: &evtv1.ReactionRemovedEvent{RoomId: roomID, MessageEventId: m.nativeID("message", messageID), Emoji: nativeEmoji}}
			}
			add(e, true, when, unknown)
			if !present {
				batch[len(batch)-1].Event.ActorId = m.users[user]
			}
			state := messages[messageID]
			if state.reactions[emoji] == nil {
				state.reactions[emoji] = map[string]bool{}
			}
			state.reactions[emoji][m.users[user]] = present
		}
		createThread := func(root string, unknown bool) {
			if threads[root] {
				return
			}
			threads[root] = true
			add(&evtv1.Event{Event: &evtv1.Event_ThreadCreated{ThreadCreated: &evtv1.ThreadCreatedEvent{RoomId: roomID, ThreadRootEventId: m.nativeID("message", root), HistoricalImport: true}}}, true, when, unknown)
		}
		switch source.Type {
		case "message.posted", "message.snapshot", "message.edited":
			messageID := m.nativeID("message", d.MessageID)
			state := messages[d.MessageID]
			introduced := state == nil
			if introduced {
				created := when
				if source.Type == "message.snapshot" {
					created = importTime(d.CreatedAt)
				}
				state = &importedMessage{author: m.users[d.AuthorID], created: created, reactions: map[string]map[string]bool{}}
				messages[d.MessageID] = state
				posted := &evtv1.MessagePostedEvent{RoomId: roomID, AuthorId: state.author, HistoricalImport: true}
				if d.ThreadID != nil {
					createThread(*d.ThreadID, true)
					posted.InThread = m.nativeID("message", *d.ThreadID)
				}
				if d.ReplyTo != nil {
					posted.InReplyTo = m.nativeID("message", *d.ReplyTo)
				}
				add(&evtv1.Event{Id: messageID, Event: &evtv1.Event_MessagePosted{MessagePosted: posted}}, false, created, false)
			}
			if d.Content.Status == "available" {
				bodyID := NewEventID()
				body := &evtv1.MessageBody{CreatedAt: state.created, AuthorId: state.author}
				if source.Type == "message.edited" {
					body.UpdatedAt = when
				} else if d.EditedAt != nil {
					body.UpdatedAt = importTime(d.EditedAt)
				}
				for _, asset := range d.Content.AssetIDs {
					body.AssetIds = append(body.AssetIds, m.nativeID("asset", asset))
				}
				if err := m.core.encryptMessageContent(ctx, body, roomID, messageID, messageID, bodyID, d.Content.Body.Text, nil); err != nil {
					return nil, err
				}
				add(&evtv1.Event{Id: bodyID, Event: &evtv1.Event_MessageBody{MessageBody: &evtv1.MessageBodyEvent{RoomId: roomID, EventId: messageID, Body: body}}}, true, when, source.OccurredAt == nil)
				if !introduced {
					add(&evtv1.Event{Event: &evtv1.Event_MessageEdited{MessageEdited: &evtv1.MessageEditedEvent{RoomId: roomID, EventId: messageID}}}, true, when, source.OccurredAt == nil)
				}
			} else if d.Content.Status == "deleted" {
				add(&evtv1.Event{Event: &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: roomID, EventId: messageID}}}, true, importTime(d.DeletedAt), d.DeletedAt == nil)
			}
			if source.Type == "message.snapshot" {
				final := map[string]map[string]bool{}
				for _, r := range d.Reactions {
					final[r.Emoji] = map[string]bool{}
					for _, user := range r.UserIDs {
						final[r.Emoji][m.users[user]] = true
					}
				}
				for _, emoji := range sortedImportKeys(state.reactions) {
					for _, user := range sortedImportKeys(state.reactions[emoji]) {
						if state.reactions[emoji][user] && !final[emoji][user] {
							react(d.MessageID, sourceForAccount[user], emoji, false, true)
						}
					}
				}
				for _, emoji := range sortedImportKeys(final) {
					for _, user := range sortedImportKeys(final[emoji]) {
						if !state.reactions[emoji][user] {
							react(d.MessageID, sourceForAccount[user], emoji, true, true)
						}
					}
				}
			}
		case "message.deleted":
			add(&evtv1.Event{Event: &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: roomID, EventId: m.nativeID("message", d.MessageID)}}}, true, when, false)
		case "reaction.added", "reaction.removed":
			react(d.MessageID, d.UserID, d.Emoji, source.Type == "reaction.added", false)
		case "thread.created":
			createThread(d.RootID, false)
		case "room.member_joined", "room.member_left":
			join(d.UserID, source.Type == "room.member_joined", false)
		case "room.membership_snapshot":
			final := map[string]bool{}
			for _, id := range d.UserIDs {
				final[m.users[id]] = true
			}
			for _, id := range sortedImportKeys(members[source.RoomID]) {
				if members[source.RoomID][id] && !final[id] {
					join(sourceForAccount[id], false, true)
				}
			}
			for _, id := range sortedImportKeys(final) {
				if !members[source.RoomID][id] {
					join(sourceForAccount[id], true, true)
				}
			}
		case "room.name_changed", "room.topic_changed", "room.purpose_changed":
			r := metadata[source.RoomID]
			switch source.Type {
			case "room.name_changed":
				r.Name = d.Value
			case "room.topic_changed":
				r.Topic = d.Value
			case "room.purpose_changed":
				r.Purpose = d.Value
			}
			metadata[source.RoomID] = r
			add(&evtv1.Event{Event: &evtv1.Event_RoomUpdated{RoomUpdated: &evtv1.RoomUpdatedEvent{RoomId: roomID, Name: m.channel(r.Name), Description: r.Topic, Purpose: &r.Purpose}}}, false, when, false)
		case "room.archived":
			add(&evtv1.Event{Event: &evtv1.Event_RoomArchived{RoomArchived: &evtv1.RoomArchivedEvent{RoomId: roomID}}}, false, when, false)
		case "room.unarchived":
			add(&evtv1.Event{Event: &evtv1.Event_RoomUnarchived{RoomUnarchived: &evtv1.RoomUnarchivedEvent{RoomId: roomID}}}, false, when, false)
		}
		batches = append(batches, batch)
	}
	for _, room := range m.archive.Rooms {
		id := m.rooms[room.ID]
		updated := newEvent(SystemActorID, &evtv1.Event{ImportContext: m.context(true, true), Event: &evtv1.Event_RoomUpdated{RoomUpdated: &evtv1.RoomUpdatedEvent{RoomId: id, Name: m.channel(room.Name), Description: room.Topic, Purpose: &room.Purpose}}})
		archived := newEvent(SystemActorID, &evtv1.Event{ImportContext: m.context(true, true)})
		if room.Archived {
			archived.Event = &evtv1.Event_RoomArchived{RoomArchived: &evtv1.RoomArchivedEvent{RoomId: id}}
		} else {
			archived.Event = &evtv1.Event_RoomUnarchived{RoomUnarchived: &evtv1.RoomUnarchivedEvent{RoomId: id}}
		}
		batches = append(batches, []evtstream.BatchEntry{{Subject: evtstream.RoomAggregate(id).SubjectFor(updated), Event: updated}, {Subject: evtstream.RoomAggregate(id).SubjectFor(archived), Event: archived}})
	}
	return batches, nil
}

func sortedImportKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// importEmoji selects a stable bundled shortcode without changing the Unicode
// sequence. Unknown sequences fail preflight rather than lose a reaction.
func importEmoji(unicode string) (string, error) {
	name := ""
	for candidate, value := range emojiNameToUnicode {
		if value == unicode && (name == "" || len(candidate) < len(name) || (len(candidate) == len(name) && candidate < name)) {
			name = candidate
		}
	}
	if name == "" {
		return "", invalidArgument("unsupported Unicode emoji")
	}
	return name, nil
}
