//go:build bootstrap || test_endpoints

package core

import (
	"context"
	"time"

	"github.com/brianvoe/gofakeit/v7"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
)

// seedNotifications adds historical targets through the import service, then
// creates explicit synthetic occurrences through the notification command model.
// Imports do not produce alerts. The fixtures use in-app delivery only, and
// alternate read states without changing the recipient's real read cursors.
func (c *ChattoCore) seedNotifications(ctx context.Context, o SeedOptions, recipientID string, result *SeedResult, faker *gofakeit.Faker) error {
	for i := range result.Rooms {
		room := &result.Rooms[i]
		if _, err := c.JoinRoom(ctx, recipientID, KindChannel, recipientID, room.ID); err != nil {
			return err
		}
		room.MemberIDs = append(room.MemberIDs, recipientID)
	}
	now := time.Now().UTC().Add(-5 * time.Minute)
	for i := range o.Notifications {
		room := result.Rooms[i%len(result.Rooms)]
		actor := result.Users[i%len(result.Users)]
		created := now.AddDate(0, 0, -(i % o.NotificationDays)).Add(-time.Duration(i) * time.Minute)
		kind := (i / o.NotificationDays) % 3
		authorID := actor.ID
		if kind != 0 {
			authorID = recipientID
		}
		body := faker.Sentence()
		rootCreated := created
		if kind != 0 {
			rootCreated = created.Add(-time.Hour)
		}
		root, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{
			RoomID: room.ID, AuthorID: authorID, CreatedAt: rootCreated, Body: body,
			CreateThread: kind == 1,
		})
		if err != nil {
			return err
		}
		result.Messages = append(result.Messages, SeedMessage{ID: root.Id, RoomID: room.ID, AuthorID: authorID, Body: body})
		ref := newNotificationMessageReference(room.ID, root.Id)
		sourceID := root.Id
		var sourceSequence uint64
		signal := &notificationv1.NotificationSignal{}
		attention := notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		switch kind {
		case 0:
			signal.Kind = &notificationv1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &notificationv1.DirectMentionReceived{Message: ref}}
		case 1:
			body = faker.Sentence()
			reply, err := c.ImportHistoricalMessage(ctx, HistoricalMessageInput{RoomID: room.ID, AuthorID: actor.ID,
				CreatedAt: created, Body: body, InReplyTo: root.Id, ThreadRootEventID: root.Id})
			if err != nil {
				return err
			}
			if err := c.FollowThread(ctx, KindChannel, recipientID, room.ID, root.Id); err != nil {
				return err
			}
			result.Messages = append(result.Messages, SeedMessage{ID: reply.Id, RoomID: room.ID, AuthorID: actor.ID, Body: body, ThreadRootID: root.Id})
			ref = newNotificationMessageReference(room.ID, reply.Id)
			ref.ThreadRootEventId = new(root.Id)
			sourceID = reply.Id
			signal.Kind = &notificationv1.NotificationSignal_ReplyReceived{ReplyReceived: &notificationv1.ReplyReceived{Message: ref}}
		case 2:
			emoji := []string{"thumbsup", "heart", "tada", "fire"}[i%4]
			if _, err := c.ImportHistoricalReaction(ctx, HistoricalReactionInput{RoomID: room.ID, MessageEventID: root.Id, UserID: actor.ID, Emoji: emoji}); err != nil {
				return err
			}
			// Reaction visibility requires the identity of the current add event,
			// not the reacted-to message. Re-adding a reaction changes this identity.
			reaction := c.roomModel.reactionMutationSnapshot(room.ID, root.Id, emoji, actor.ID)
			sourceID, sourceSequence = reaction.SourceEventID, reaction.Seq
			signal.Kind = &notificationv1.NotificationSignal_ReactionReceived{ReactionReceived: &notificationv1.ReactionReceived{Message: ref, Emoji: emoji}}
			attention = notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
		}
		_, createdOccurrence, err := c.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
			RecipientID: recipientID, SourceEventID: sourceID, SourceCreated: created, ActorID: actor.ID,
			SourceStreamSequence: sourceSequence,
			Signal:               signal, AttentionLevel: attention, Mode: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
			InitiallyRead: i%4 == 0,
		})
		if err != nil {
			return err
		}
		if createdOccurrence {
			result.NotificationCount++
		}
	}
	return nil
}
