package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// StreamRoomEventsLive creates a continuous stream of live events for a specific room.
// Only delivers new events that occur after subscription starts (no historical fetch).
// For historical events, use GetRoomEvents query instead.
// The returned channel will be closed when the context is cancelled or after unrecoverable errors.
//
// Reliability: Transient JetStream errors (heartbeat missed, leadership change) trigger automatic
// retry with backoff. Terminal errors (connection closed, consumer deleted) close the channel.
// Clients should handle channel closure by resubscribing if they want to continue receiving events.
func (c *ChattoCore) StreamRoomEventsLive(ctx context.Context, kind RoomKind, room_id string) (<-chan *corev1.Event, error) {
	// Post-#597 cutover: room events live on the EVT stream under
	// evt.room.{R}.>. We consume from there with DeliverNewPolicy so
	// only events arriving after subscription are surfaced; the
	// initial-load path is GetRoomEvents (projection-backed).
	stream := c.storage.serverEvtStream
	filterSubject := events.RoomAggregate(room_id).AllEventsFilter()
	cons, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects:    []string{filterSubject},
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ordered consumer: %w", err)
	}

	eventChan := make(chan *corev1.Event)

	// Track current iterator for cleanup
	var currentIter jetstream.MessagesContext
	var iterMu sync.Mutex

	go func() {
		c.logger.Debug("Starting live room event stream", "room_id", room_id)

		defer func() {
			c.logger.Debug("Live room event stream closed", "room_id", room_id)
			close(eventChan)
		}()

		const maxRetries = 3
		retryCount := 0

		for {
			// Get message iterator (retry on recoverable errors)
			iter, err := cons.Messages()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.logger.Error("Failed to get message iterator", "error", err)
				return
			}

			// Store iterator reference for external cleanup
			iterMu.Lock()
			currentIter = iter
			iterMu.Unlock()

			c.logger.Debug("Live subscription active", "room_id", room_id)

			// Read messages until error
			for {
				msg, err := iter.Next()
				if err != nil {
					iter.Stop()

					if ctx.Err() != nil {
						return
					}

					// Terminal errors - cannot recover
					if isTerminalIteratorError(err) {
						c.logger.Debug("Iterator terminated", "room_id", room_id, "error", err)
						return
					}

					// Recoverable error - retry with backoff
					retryCount++
					if retryCount > maxRetries {
						c.logger.Warn("Max retries exceeded for room event iterator", "room_id", room_id, "error", err, "retries", retryCount)
						return
					}

					c.logger.Debug("Iterator error, retrying", "room_id", room_id, "error", err, "retry", retryCount)
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Duration(retryCount) * 100 * time.Millisecond):
						// Continue to outer loop to create new iterator
					}
					break
				}

				// Success - reset retry count
				retryCount = 0

				var event corev1.Event
				if err := proto.Unmarshal(msg.Data(), &event); err != nil {
					c.logger.Warn("Failed to unmarshal live event", "error", err)
					continue
				}
				if event.GetMessageBody() != nil {
					continue
				}

				select {
				case <-ctx.Done():
					iter.Stop()
					return
				case eventChan <- &event:
					// Event delivered
				}
			}
		}
	}()

	// Goroutine to stop the iterator when context is cancelled
	go func() {
		<-ctx.Done()
		iterMu.Lock()
		if currentIter != nil {
			currentIter.Stop()
		}
		iterMu.Unlock()
	}()

	return eventChan, nil
}
