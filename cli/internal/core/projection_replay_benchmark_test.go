package core

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	projectionBenchmarkRooms = 10
	projectionBenchmarkUsers = 100
	// Bump the fixture version when changing its event mix so benchmark
	// results from materially different workloads are not compared directly.
	projectionBenchmarkFixtureVersion = "mixed_v1"
)

type projectionBenchmarkWireEvent struct {
	subject string
	data    []byte
}

type projectionBenchmarkTarget struct {
	projection events.Projection
	subjects   []string
}

// BenchmarkProjectionReplay measures the complete decode-and-apply startup
// path. The fixture is serialized before timing so the benchmark includes the
// protobuf decode performed by the shared projector, but not fixture creation.
func BenchmarkProjectionReplay(b *testing.B) {
	for _, logicalMessages := range []int{1_000, 10_000} {
		fixture := newProjectionBenchmarkFixture(b, logicalMessages)
		for _, scope := range []string{"room_timeline", "threads", "timeline_and_threads"} {
			b.Run(fmt.Sprintf("%s/%s/messages_%d", projectionBenchmarkFixtureVersion, scope, logicalMessages), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(projectionBenchmarkWireBytes(fixture)))
				b.ResetTimer()

				for range b.N {
					targets, err := replayProjectionBenchmarkFixture(fixture, scope)
					if err != nil {
						b.Fatal(err)
					}
					runtime.KeepAlive(targets)
				}

				b.StopTimer()
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(fixture)), "ns/event")
				b.ReportMetric(float64(len(fixture)), "events/replay")
			})
		}
	}
}

// BenchmarkProjectionRetainedHeap reports live Go heap after a full replay,
// startup-only dedupe state release, and forced GC. Run it with
// -benchtime=1x: retaining more than one replay in a benchmark iteration would
// measure a workload that does not exist in a Chatto process.
func BenchmarkProjectionRetainedHeap(b *testing.B) {
	if b.N != 1 {
		b.Skip("run with -benchtime=1x")
	}
	if os.Getenv("CHATTO_BENCH_HEAP_PROFILE_DIR") != "" {
		runtime.MemProfileRate = 1
	}

	for _, logicalMessages := range []int{10_000, 50_000} {
		for _, scope := range []string{"room_timeline", "threads", "timeline_and_threads"} {
			b.Run(fmt.Sprintf("%s/%s/messages_%d", projectionBenchmarkFixtureVersion, scope, logicalMessages), func(b *testing.B) {
				if b.N != 1 {
					b.Skip("run with -benchtime=1x")
				}
				fixture := newProjectionBenchmarkFixture(b, logicalMessages)

				runtime.GC()
				var before runtime.MemStats
				runtime.ReadMemStats(&before)

				b.ReportAllocs()
				b.ResetTimer()
				targets, err := replayProjectionBenchmarkFixture(fixture, scope)
				b.StopTimer()
				if err != nil {
					b.Fatal(err)
				}

				runtime.GC()
				var after runtime.MemStats
				runtime.ReadMemStats(&after)
				retainedBytes := int64(after.HeapAlloc) - int64(before.HeapAlloc)
				b.ReportMetric(float64(retainedBytes)/float64(len(fixture)), "retained-heap-B/event")
				b.ReportMetric(float64(retainedBytes)/float64(logicalMessages), "retained-heap-B/message")
				b.ReportMetric(float64(len(fixture)), "events/replay")

				// Keep the serialized fixture present for both memory snapshots so
				// the delta contains projection state only. Release it before an
				// optional heap profile so pprof attribution is not obscured by the
				// benchmark input corpus.
				runtime.KeepAlive(fixture)
				fixture = nil
				runtime.GC()
				writeProjectionBenchmarkHeapProfile(b, scope, logicalMessages)
				runtime.KeepAlive(targets)
			})
		}
	}
}

func TestProjectionBenchmarkFixture(t *testing.T) {
	first := newProjectionBenchmarkFixture(t, 1_000)
	second := newProjectionBenchmarkFixture(t, 1_000)
	if len(first) <= 2_000 {
		t.Fatalf("fixture contains %d events, want more than two events per logical message", len(first))
	}
	eventKinds := make(map[string]int)
	var echoes, attachments int
	timelineSubjects := NewRoomTimelineProjection().Subjects()
	if len(first) != len(second) {
		t.Fatalf("fixture event count changed between identical runs: %d != %d", len(first), len(second))
	}
	for i := range first {
		if first[i].subject != second[i].subject || !bytes.Equal(first[i].data, second[i].data) {
			t.Fatalf("fixture is not deterministic at event %d", i)
		}
		var event corev1.Event
		if err := proto.Unmarshal(first[i].data, &event); err != nil {
			t.Fatalf("decode fixture event %d: %v", i, err)
		}
		eventKinds[events.EventTypeOf(&event)]++
		if posted := event.GetMessagePosted(); posted != nil && posted.GetEchoOfEventId() != "" {
			echoes++
		}
		if body := event.GetMessageBody(); body != nil && len(body.GetBody().GetAssetIds()) != 0 {
			attachments++
		}
		if !projectionBenchmarkMatchesAnySubject(timelineSubjects, first[i].subject) {
			t.Fatalf("fixture event %d on %q is not consumed by the room timeline", i, first[i].subject)
		}
	}
	for _, kind := range []string{
		events.EventMessagePosted,
		events.EventMessageBody,
		events.EventMessageEdited,
		events.EventMessageRetracted,
		events.EventThreadCreated,
		events.EventThreadFollowed,
		events.EventUserKeyShredded,
	} {
		if eventKinds[kind] == 0 {
			t.Errorf("fixture contains no %s events", kind)
		}
	}
	if echoes == 0 {
		t.Error("fixture contains no message echoes")
	}
	if attachments == 0 {
		t.Error("fixture contains no message attachments")
	}

	targets, err := replayProjectionBenchmarkFixture(first, "timeline_and_threads")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		switch projection := target.projection.(type) {
		case *RoomTimelineProjection:
			if got := len(projection.replayGuard.retainedEventIDs()); got != 0 {
				t.Fatalf("room timeline retained %d startup replay IDs", got)
			}
		case *ThreadProjection:
			if got := len(projection.replayGuard.retainedEventIDs()); got != 0 {
				t.Fatalf("threads retained %d startup replay IDs", got)
			}
		}
	}
}

func newProjectionBenchmarkFixture(tb testing.TB, logicalMessages int) []projectionBenchmarkWireEvent {
	tb.Helper()
	// mixed_v1 uses 10 rooms and 100 users, with 20% replies, 4% edits,
	// 1% retractions, 5% echoes, 2% attachments, one follow per created
	// thread, and one key-shred fact in fixtures of at least 1,000 messages.
	fixture := make([]projectionBenchmarkWireEvent, 0, logicalMessages*2+logicalMessages/4)
	latestRootByRoom := make([]string, projectionBenchmarkRooms)
	createdThreads := make(map[string]struct{}, logicalMessages/5)
	serial := 0

	nextID := func(prefix string) string {
		serial++
		return fmt.Sprintf("%s%026d", prefix, serial)
	}
	createdAt := func() *timestamppb.Timestamp {
		return timestamppb.New(time.Unix(1_700_000_000+int64(serial), 0).UTC())
	}
	appendEvent := func(aggregate events.Aggregate, event *corev1.Event) {
		data, err := proto.Marshal(event)
		if err != nil {
			tb.Fatalf("marshal benchmark event: %v", err)
		}
		fixture = append(fixture, projectionBenchmarkWireEvent{
			subject: aggregate.SubjectFor(event),
			data:    data,
		})
	}

	for messageIndex := range logicalMessages {
		roomIndex := messageIndex % projectionBenchmarkRooms
		ordinalInRoom := messageIndex / projectionBenchmarkRooms
		roomID := fmt.Sprintf("R%025d", roomIndex)
		actorID := fmt.Sprintf("U%025d", messageIndex%projectionBenchmarkUsers)
		roomAggregate := events.RoomAggregate(roomID)
		previousRoot := latestRootByRoom[roomIndex]
		messageID := nextID("E")
		threadRoot := ""
		if ordinalInRoom > 0 && ordinalInRoom%5 == 0 {
			threadRoot = previousRoot
			if _, exists := createdThreads[threadRoot]; !exists {
				createdThreads[threadRoot] = struct{}{}
				appendEvent(roomAggregate, &corev1.Event{
					Id:        nextID("T"),
					ActorId:   actorID,
					CreatedAt: createdAt(),
					Event: &corev1.Event_ThreadCreated{ThreadCreated: &corev1.ThreadCreatedEvent{
						RoomId:            roomID,
						ThreadRootEventId: threadRoot,
					}},
				})
				appendEvent(roomAggregate, &corev1.Event{
					Id:        nextID("F"),
					ActorId:   actorID,
					CreatedAt: createdAt(),
					Event: &corev1.Event_ThreadFollowed{ThreadFollowed: &corev1.ThreadFollowedEvent{
						RoomId:            roomID,
						ThreadRootEventId: threadRoot,
						UserId:            actorID,
						Source:            corev1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_MANUAL,
					}},
				})
			}
		} else {
			latestRootByRoom[roomIndex] = messageID
		}

		echoOfEventID := ""
		if threadRoot == "" && previousRoot != "" && ordinalInRoom%20 == 11 {
			echoOfEventID = previousRoot
		}
		appendEvent(roomAggregate, &corev1.Event{
			Id:        messageID,
			ActorId:   actorID,
			CreatedAt: createdAt(),
			Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{
				RoomId:        roomID,
				InReplyTo:     threadRoot,
				InThread:      threadRoot,
				EchoOfEventId: echoOfEventID,
			}},
		})

		assetIDs := []string(nil)
		if messageIndex%50 == 0 {
			assetIDs = []string{fmt.Sprintf("A%025d", messageIndex)}
		}
		bodyID := nextID("B")
		appendEvent(roomAggregate, projectionBenchmarkBodyEvent(bodyID, messageID, roomID, actorID, assetIDs, createdAt()))

		if messageIndex > 0 && messageIndex%25 == 0 {
			appendEvent(roomAggregate, &corev1.Event{
				Id:        nextID("X"),
				ActorId:   actorID,
				CreatedAt: createdAt(),
				Event: &corev1.Event_MessageEdited{MessageEdited: &corev1.MessageEditedEvent{
					RoomId:  roomID,
					EventId: messageID,
				}},
			})
			bodyID = nextID("B")
			appendEvent(roomAggregate, projectionBenchmarkBodyEvent(bodyID, messageID, roomID, actorID, assetIDs, createdAt()))
		}
		if messageIndex > 0 && messageIndex%100 == 0 {
			appendEvent(roomAggregate, &corev1.Event{
				Id:        nextID("D"),
				ActorId:   actorID,
				CreatedAt: createdAt(),
				Event: &corev1.Event_MessageRetracted{MessageRetracted: &corev1.MessageRetractedEvent{
					RoomId:  roomID,
					EventId: messageID,
				}},
			})
		}
	}

	if logicalMessages >= 1_000 {
		userID := fmt.Sprintf("U%025d", 0)
		appendEvent(events.UserAggregate(userID), &corev1.Event{
			Id:        nextID("S"),
			CreatedAt: createdAt(),
			Event: &corev1.Event_UserKeyShredded{UserKeyShredded: &corev1.UserKeyShreddedEvent{
				UserId: userID,
			}},
		})
	}

	return fixture
}

func projectionBenchmarkBodyEvent(id, messageID, roomID, actorID string, assetIDs []string, createdAt *timestamppb.Timestamp) *corev1.Event {
	return &corev1.Event{
		Id:        id,
		ActorId:   actorID,
		CreatedAt: createdAt,
		Event: &corev1.Event_MessageBody{MessageBody: &corev1.MessageBodyEvent{
			RoomId:  roomID,
			EventId: messageID,
			Body: &corev1.MessageBody{
				AuthorId:      actorID,
				BodyEventId:   id,
				EncryptedBody: []byte(strings.Repeat("benchmark message body ", 8)),
				AssetIds:      assetIDs,
			},
		}},
	}
}

func replayProjectionBenchmarkFixture(fixture []projectionBenchmarkWireEvent, scope string) ([]projectionBenchmarkTarget, error) {
	targets, err := newProjectionBenchmarkTargets(scope)
	if err != nil {
		return nil, err
	}
	for i, wireEvent := range fixture {
		var event corev1.Event
		if err := proto.Unmarshal(wireEvent.data, &event); err != nil {
			return nil, fmt.Errorf("decode event %d: %w", i, err)
		}
		for _, target := range targets {
			if !projectionBenchmarkMatchesAnySubject(target.subjects, wireEvent.subject) {
				continue
			}
			if err := target.projection.Apply(&event, uint64(i+1)); err != nil {
				return nil, fmt.Errorf("apply event %d to %T: %w", i, target.projection, err)
			}
		}
	}
	for _, target := range targets {
		if projection, ok := target.projection.(events.StartupReplayCompleter); ok {
			projection.CompleteStartupReplay()
		}
	}
	return targets, nil
}

func newProjectionBenchmarkTargets(scope string) ([]projectionBenchmarkTarget, error) {
	newTarget := func(projection events.Projection) projectionBenchmarkTarget {
		return projectionBenchmarkTarget{projection: projection, subjects: projection.Subjects()}
	}
	switch scope {
	case "room_timeline":
		return []projectionBenchmarkTarget{newTarget(NewRoomTimelineProjection())}, nil
	case "threads":
		return []projectionBenchmarkTarget{newTarget(NewThreadProjection())}, nil
	case "timeline_and_threads":
		return []projectionBenchmarkTarget{
			newTarget(NewRoomTimelineProjection()),
			newTarget(NewThreadProjection()),
		}, nil
	default:
		return nil, fmt.Errorf("unknown projection benchmark scope %q", scope)
	}
}

func projectionBenchmarkMatchesAnySubject(filters []string, subject string) bool {
	for _, filter := range filters {
		if projectionBenchmarkSubjectMatches(filter, subject) {
			return true
		}
	}
	return false
}

func projectionBenchmarkSubjectMatches(filter, subject string) bool {
	filterParts := strings.Split(filter, ".")
	subjectParts := strings.Split(subject, ".")
	for i, part := range filterParts {
		if part == ">" {
			return true
		}
		if i >= len(subjectParts) || (part != "*" && part != subjectParts[i]) {
			return false
		}
	}
	return len(filterParts) == len(subjectParts)
}

func projectionBenchmarkWireBytes(fixture []projectionBenchmarkWireEvent) int {
	total := 0
	for _, event := range fixture {
		total += len(event.data)
	}
	return total
}

func writeProjectionBenchmarkHeapProfile(b *testing.B, scope string, logicalMessages int) {
	b.Helper()
	configuredDirectory := os.Getenv("CHATTO_BENCH_HEAP_PROFILE_DIR")
	if configuredDirectory == "" {
		return
	}
	directory := projectionBenchmarkOutputDirectory(b, configuredDirectory)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		b.Fatalf("create heap profile directory: %v", err)
	}
	path := filepath.Join(directory, fmt.Sprintf("%s-%s-messages-%d.pb.gz", projectionBenchmarkFixtureVersion, scope, logicalMessages))
	file, err := os.Create(path)
	if err != nil {
		b.Fatalf("create heap profile: %v", err)
	}
	defer file.Close()
	if err := pprof.WriteHeapProfile(file); err != nil {
		b.Fatalf("write heap profile: %v", err)
	}
}

// Go test binaries run from the package directory, while benchmark paths are
// configured relative to the cli module. Resolve through go.mod so the same
// task works from the repository root, cli, or an IDE.
func projectionBenchmarkOutputDirectory(tb testing.TB, configuredDirectory string) string {
	tb.Helper()
	if filepath.IsAbs(configuredDirectory) {
		return configuredDirectory
	}
	directory, err := os.Getwd()
	if err != nil {
		tb.Fatalf("get benchmark working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return filepath.Clean(filepath.Join(directory, configuredDirectory))
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			tb.Fatalf("resolve benchmark output %q: no go.mod above %q", configuredDirectory, directory)
		}
		directory = parent
	}
}
