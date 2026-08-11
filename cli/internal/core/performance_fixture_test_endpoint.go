//go:build test_endpoints

package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/pkg/events"
)

const (
	PerformanceFixtureVersion       = "large-e2e-v1"
	defaultPerformanceFixtureBatch  = 100
	performanceFixtureRoomName      = "performance"
	performanceFixtureMessagePrefix = "Performance fixture message"
)

// PerformanceFixtureOptions controls the test-only large-server fixture.
// Synthetic users are OAuth-only because password hashing is irrelevant to
// directory and timeline performance; the ordinary bootstrap owner remains
// available for authenticating the browser test.
type PerformanceFixtureOptions struct {
	Users     int
	Messages  int
	BatchSize int
}

// PerformanceFixtureResult identifies and describes a generated fixture.
type PerformanceFixtureResult struct {
	Version         string
	SyntheticUsers  int
	Messages        int
	RoomID          string
	RoomName        string
	FirstUserLogin  string
	LastUserLogin   string
	LastMessageID   string
	LastMessageBody string
}

// SeedPerformanceFixture creates a large, logically deterministic dataset for
// black-box performance tests. It is compiled only into test-endpoint builds.
// Users go through the normal encrypted account creation service. Message
// bodies are encrypted normally and committed in room-scoped, OCC-guarded
// atomic batches; every serving projection is current on return.
func (c *ChattoCore) SeedPerformanceFixture(ctx context.Context, options PerformanceFixtureOptions) (*PerformanceFixtureResult, error) {
	if options.Users < 1 {
		return nil, fmt.Errorf("users must be positive")
	}
	if options.Messages < 1 {
		return nil, fmt.Errorf("messages must be positive")
	}
	if options.BatchSize == 0 {
		options.BatchSize = defaultPerformanceFixtureBatch
	}
	if options.BatchSize < 1 || options.BatchSize > 250 {
		return nil, fmt.Errorf("batch size must be between 1 and 250")
	}
	if _, err := c.GetUserByLogin(ctx, performanceFixtureUserLogin(1)); err == nil {
		return nil, fmt.Errorf("performance fixture already exists")
	} else if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("check existing performance fixture: %w", err)
	}

	room, err := c.createPerformanceFixtureRoom(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]*corev1.User, 0, options.Users)
	for index := 1; index <= options.Users; index++ {
		login := performanceFixtureUserLogin(index)
		user, err := c.CreateUser(ctx, SystemActorID, login, fmt.Sprintf("Performance User %04d", index), "")
		if err != nil {
			return nil, fmt.Errorf("create synthetic user %d: %w", index, err)
		}
		users = append(users, user)
	}

	result := &PerformanceFixtureResult{
		Version:        PerformanceFixtureVersion,
		SyntheticUsers: len(users),
		Messages:       options.Messages,
		RoomID:         room.GetId(),
		RoomName:       room.GetName(),
		FirstUserLogin: performanceFixtureUserLogin(1),
		LastUserLogin:  performanceFixtureUserLogin(options.Users),
	}
	baseTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	for first := 0; first < options.Messages; first += options.BatchSize {
		last := min(first+options.BatchSize, options.Messages)
		entries := make([]evtstream.MutationEntry, 0, (last-first)*2)
		for messageIndex := first; messageIndex < last; messageIndex++ {
			author := users[messageIndex%len(users)]
			bodyText := performanceFixtureMessageBody(messageIndex + 1)
			messageID := NewEventID()
			bodyEventID := NewEventID()
			createdAt := timestamppb.New(baseTime.Add(time.Duration(messageIndex) * time.Second))
			body := &corev1.MessageBody{
				CreatedAt: createdAt,
				AuthorId:  author.GetId(),
			}
			if err := c.encryptMessageBody(ctx, body, room.GetId(), messageID, bodyEventID, bodyText); err != nil {
				return nil, fmt.Errorf("encrypt synthetic message %d: %w", messageIndex+1, err)
			}

			bodyEvent := newEvent(author.GetId(), &corev1.Event{
				Id:        bodyEventID,
				CreatedAt: createdAt,
				Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{
					RoomId:  room.GetId(),
					EventId: messageID,
					Body:    body,
				}},
			})
			messageEvent := newEvent(author.GetId(), &corev1.Event{
				Id:        messageID,
				CreatedAt: createdAt,
				Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
					RoomId: room.GetId(),
				}},
			})
			aggregate := evtstream.RoomAggregate(room.GetId())
			entries = append(entries,
				evtstream.MutationEntry{Subject: aggregate.SubjectFor(bodyEvent), Event: bodyEvent},
				evtstream.MutationEntry{Subject: aggregate.SubjectFor(messageEvent), Event: messageEvent},
			)
			result.LastMessageID = messageID
			result.LastMessageBody = bodyText
		}

		aggregate := evtstream.RoomAggregate(room.GetId())
		mutation, err := c.EventPublisher.ExecuteMutation(ctx, events.AtSubject(aggregate.AllEventsFilter()), func(context.Context, events.MutationAttempt) ([]evtstream.MutationEntry, error) {
			return entries, nil
		})
		if err != nil {
			return nil, fmt.Errorf("commit synthetic messages %d-%d: %w", first+1, last, err)
		}
		if len(mutation.Sequences) == 0 {
			return nil, fmt.Errorf("commit synthetic messages %d-%d returned no stream positions", first+1, last)
		}
		position := events.SubjectPosition(aggregate.AllEventsFilter(), mutation.Sequences[len(mutation.Sequences)-1])
		if err := c.roomModel.waitForTimeline(ctx, position); err != nil {
			return nil, fmt.Errorf("wait for synthetic messages %d-%d: %w", first+1, last, err)
		}
	}
	if err := c.WaitForProjectionsCurrent(ctx); err != nil {
		return nil, fmt.Errorf("wait for performance fixture projections: %w", err)
	}

	return result, nil
}

func (c *ChattoCore) createPerformanceFixtureRoom(ctx context.Context) (*corev1.Room, error) {
	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return nil, fmt.Errorf("list rooms for performance fixture: %w", err)
	}
	for _, room := range rooms {
		if room.GetName() == performanceFixtureRoomName {
			return room, nil
		}
	}
	room, err := c.CreateRoom(
		ctx,
		SystemActorID,
		KindChannel,
		"",
		performanceFixtureRoomName,
		"Synthetic large-history performance fixture",
		WithUniversalRoom(true),
	)
	if err != nil {
		return nil, fmt.Errorf("create performance fixture room: %w", err)
	}
	return room, nil
}

func performanceFixtureUserLogin(index int) string {
	return fmt.Sprintf("perfuser%04d", index)
}

func performanceFixtureMessageBody(index int) string {
	return fmt.Sprintf("%s %06d searchable-perf-token", performanceFixtureMessagePrefix, index)
}
