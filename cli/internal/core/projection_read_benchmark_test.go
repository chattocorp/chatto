package core

import (
	"runtime"
	"testing"

	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// projectionReadBenchmarkQueries holds request-shaped inputs sampled from a
// real EVT stream.
type projectionReadBenchmarkQueries struct {
	interactions []projectionReadBenchmarkInteraction
	messages     []projectionReadBenchmarkMessage
	threadRoots  []string
	reacted      []string
	reactions    []projectionReadBenchmarkReaction
}

type projectionReadBenchmarkInteraction struct {
	userID, roomID, rootID string
}

type projectionReadBenchmarkMessage struct {
	roomID, eventID string
}

type projectionReadBenchmarkReaction struct {
	roomID, messageID, emoji, userID string
}

// BenchmarkProjectionReadsFromStore measures request-path reads of the Threads
// and Reactions components against a replayed real EVT stream. Set
// CHATTO_BENCH_EVT_STORE_DIR to a copied NATS data directory.
func BenchmarkProjectionReadsFromStore(b *testing.B) {
	storeDir := projectionBenchmarkStoreDir(b)
	fixture := loadProjectionBenchmarkStoreFixture(b, storeDir)
	threads := NewThreadProjection()
	reactions := NewReactionProjection()
	queries := projectionReadBenchmarkQueries{}
	userIDs := make(map[string]struct{})
	for i, wireEvent := range fixture {
		var event evtv1.Event
		if err := proto.Unmarshal(wireEvent.data, &event); err != nil {
			b.Fatal(err)
		}
		for _, projection := range []projectionReadBenchmarkApplier{threads, reactions} {
			if projectionBenchmarkMatchesAnySubject(projection.Subjects(), wireEvent.subject) {
				if err := projection.Apply(&event, uint64(i+1)); err != nil {
					b.Fatal(err)
				}
			}
		}
		if event.GetActorId() != "" {
			userIDs[event.GetActorId()] = struct{}{}
		}
		switch e := event.GetEvent().(type) {
		case *evtv1.Event_MessagePosted:
			root := e.MessagePosted.GetInThread()
			if root == "" {
				root = event.GetId()
			} else {
				queries.threadRoots = append(queries.threadRoots, root)
			}
			queries.messages = append(queries.messages, projectionReadBenchmarkMessage{roomID: e.MessagePosted.GetRoomId(), eventID: event.GetId()})
		case *evtv1.Event_ReactionAdded:
			queries.reacted = append(queries.reacted, e.ReactionAdded.GetMessageEventId())
			queries.reactions = append(queries.reactions, projectionReadBenchmarkReaction{
				roomID: e.ReactionAdded.GetRoomId(), messageID: e.ReactionAdded.GetMessageEventId(),
				emoji: e.ReactionAdded.GetEmoji(), userID: event.GetActorId(),
			})
		}
	}
	threads.CompleteStartupReplay()
	reactions.CompleteStartupReplay()
	benchmarkProjectionStoreReplay(b, fixture, "replay/threads", func() projectionReadBenchmarkApplier { return NewThreadProjection() })
	benchmarkProjectionStoreReplay(b, fixture, "replay/reactions", func() projectionReadBenchmarkApplier { return NewReactionProjection() })
	// Pair every sampled message with several users so both hits and misses
	// are exercised, as authorization checks do for timeline pages.
	users := make([]string, 0, len(userIDs))
	for userID := range userIDs {
		users = append(users, userID)
	}
	for i, message := range queries.messages {
		root, _ := threads.ThreadRootForMessage(message.roomID, message.eventID)
		queries.interactions = append(queries.interactions, projectionReadBenchmarkInteraction{
			userID: users[i%len(users)], roomID: message.roomID, rootID: root,
		})
	}
	if len(queries.messages) == 0 || len(queries.threadRoots) == 0 || len(queries.reacted) == 0 {
		b.Fatal("store fixture lacks messages, thread replies, or reactions")
	}
	runtime.GC()

	b.Run("threads/HasInteraction", func(b *testing.B) {
		b.ReportAllocs()
		for i := range b.N {
			q := queries.interactions[i%len(queries.interactions)]
			threads.HasInteraction(q.userID, q.roomID, q.rootID)
		}
	})
	b.Run("threads/ThreadRootForMessage", func(b *testing.B) {
		b.ReportAllocs()
		for i := range b.N {
			q := queries.messages[i%len(queries.messages)]
			threads.ThreadRootForMessage(q.roomID, q.eventID)
		}
	})
	b.Run("threads/ThreadEvents", func(b *testing.B) {
		b.ReportAllocs()
		for i := range b.N {
			threads.ThreadEvents(queries.threadRoots[i%len(queries.threadRoots)])
		}
	})
	b.Run("threads/ThreadMetadata", func(b *testing.B) {
		b.ReportAllocs()
		for i := range b.N {
			threads.ThreadMetadata(queries.threadRoots[i%len(queries.threadRoots)])
		}
	})
	b.Run("reactions/Reactions", func(b *testing.B) {
		b.ReportAllocs()
		for i := range b.N {
			reactions.Reactions(queries.reacted[i%len(queries.reacted)])
		}
	})
	b.Run("reactions/ReactionsBatch50", func(b *testing.B) {
		// A timeline page asks for reactions on a window of messages.
		page := min(50, len(queries.messages))
		b.ReportAllocs()
		for i := range b.N {
			start := (i * page) % max(1, len(queries.messages)-page)
			ids := make([]string, 0, page)
			for _, message := range queries.messages[start : start+page] {
				ids = append(ids, message.eventID)
			}
			reactions.ReactionsBatch(ids)
		}
	})
	b.Run("reactions/ReactionMutationSnapshot", func(b *testing.B) {
		b.ReportAllocs()
		for i := range b.N {
			q := queries.reactions[i%len(queries.reactions)]
			reactions.ReactionMutationSnapshot(q.roomID, q.messageID, q.emoji, q.userID)
		}
	})
	b.Logf("queries: %d messages, %d thread replies, %d reactions", len(queries.messages), len(queries.threadRoots), len(queries.reactions))
}

type projectionReadBenchmarkApplier interface {
	Subjects() []string
	Apply(*evtv1.Event, uint64) error
}

// benchmarkProjectionStoreReplay measures a complete decode-and-apply replay
// of the real fixture into one fresh projection.
func benchmarkProjectionStoreReplay(b *testing.B, fixture []projectionBenchmarkWireEvent, name string, newProjection func() projectionReadBenchmarkApplier) {
	subjects := newProjection().Subjects()
	b.Run(name, func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			projection := newProjection()
			for i, wireEvent := range fixture {
				if !projectionBenchmarkMatchesAnySubject(subjects, wireEvent.subject) {
					continue
				}
				var event evtv1.Event
				if err := proto.Unmarshal(wireEvent.data, &event); err != nil {
					b.Fatal(err)
				}
				if err := projection.Apply(&event, uint64(i+1)); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}
