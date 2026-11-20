package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/pkg/events"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
	"github.com/nats-io/nats.go/jetstream"
)

const messageBodyErasureConsumerName = "chatto-message-body-erasure-v1"

// Existing edit and retraction facts are the durable handoff. The boot sweep
// covers history from before this consumer existed.
func (c *ChattoCore) configureMessageBodyErasure(ctx context.Context) error {
	stream := c.storage.serverEvtStream
	consumer, err := evtstream.CreateEffectConsumer(ctx, stream, evtstream.EffectConsumerConfig{
		Name:        messageBodyErasureConsumerName,
		Description: "Hand off obsolete message body erasure",
		FilterSubjects: []string{
			evtstream.RoomEventTypeFilter(evtstream.EventMessageEdited),
			evtstream.RoomEventTypeFilter(evtstream.EventMessageRetracted),
		},
		AckWait:       assetCleanupAckWait,
		MaxAckPending: assetCleanupMaxPending,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		return err
	}
	_, err = c.newEffectJob(jobTypeMessageBodyErasure, stream, consumer, c.eraseMessageBodies, jobqueue.EffectOptions{
		MaxAttempts: jobqueue.UnlimitedAttempts,
		RetryDelay:  assetCleanupRetryDelay,
	})
	return err
}

func (c *ChattoCore) eraseMessageBodies(ctx context.Context, delivery events.DurableDelivery) error {
	event, err := decodeDurableCoreDelivery(delivery)
	if err != nil {
		return err
	}
	var roomID, messageID string
	retraction := event.GetMessageRetracted()
	switch {
	case retraction != nil:
		roomID, messageID = retraction.GetRoomId(), retraction.GetEventId()
	case event.GetMessageEdited() != nil:
		edit := event.GetMessageEdited()
		roomID, messageID = edit.GetRoomId(), edit.GetEventId()
	}
	if roomID == "" || messageID == "" || delivery.Subject != evtstream.RoomAggregate(roomID).SubjectFor(event) {
		return events.TerminateDelivery("invalid message change", errors.New("room subject and message change do not match"))
	}
	if err := c.roomModel.waitForTimeline(ctx, events.SubjectPosition(delivery.Subject, delivery.StreamSequence)); err != nil {
		return err
	}
	if entry, ok := c.roomModel.timelineEntry(messageID); ok && entry.RoomID != roomID {
		return events.TerminateDelivery("invalid message change", errors.New("message belongs to a different room"))
	}
	if retraction != nil {
		return c.secureDeleteMessageBodyEvents(ctx, c.retractedMessageBodySeqs(messageID))
	}
	return c.secureDeleteMessageBodyEvents(ctx, c.roomModel.obsoleteBodyEventSeqs(messageID))
}

// Retracting an original releases any historical copies on linked echoes.
// Retracting an echo releases only that echo's own body history.
func (c *ChattoCore) retractedMessageBodySeqs(messageID string) []uint64 {
	seqs := c.roomModel.obsoleteBodyEventSeqs(messageID)
	if !c.roomModel.isEcho(messageID) {
		for _, id := range c.roomModel.linkedEventIDs(messageID) {
			linked, _, _ := c.roomModel.bodyEventSeqs(id)
			seqs = append(seqs, linked...)
		}
	}
	return seqs
}

func (c *ChattoCore) secureDeleteMessageBodyEvents(ctx context.Context, seqs []uint64) error {
	var result error
	for _, seq := range seqs {
		if seq == 0 {
			continue
		}
		if err := secureDeleteStreamMessage(ctx, c.storage.serverEvtStream, seq); err != nil {
			result = errors.Join(result, fmt.Errorf("erase message body at sequence %d: %w", seq, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		c.eventReader.Forget(seq)
	}
	return result
}

func (c *ChattoCore) secureDeleteObsoleteProjectedMessageBodyEvents(ctx context.Context) {
	if err := c.secureDeleteMessageBodyEvents(ctx, c.roomModel.allObsoleteBodyEventSeqs()); err != nil {
		c.logger.Warn("Message body erasure during startup failed", "error", err)
	}
}
