package core

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/testutil"
)

type testEventHarness struct {
	nc        *nats.Conn
	js        jetstream.JetStream
	stream    jetstream.Stream
	publisher *evtstream.Publisher
}

func newTestEventHarness(t *testing.T) *testEventHarness {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New: %v", err)
	}
	stream, err := js.CreateOrUpdateStream(testContext(t), jetstream.StreamConfig{
		Name:               "EVT",
		Subjects:           []string{"evt.>"},
		Storage:            jetstream.MemoryStorage,
		AllowAtomicPublish: true,
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateStream: %v", err)
	}
	return &testEventHarness{
		nc:        nc,
		js:        js,
		stream:    stream,
		publisher: evtstream.NewPublisher(js, stream, testCoreLogger()),
	}
}

func testEventPublisher(t *testing.T) *evtstream.Publisher {
	t.Helper()
	return newTestEventHarness(t).publisher
}

func (h *testEventHarness) projector(proj evtstream.Projection) *events.Projector {
	return evtstream.NewProjector(h.js, h.stream, proj, testCoreLogger())
}

func testProjectionHandle[T any, P evtstream.ProjectionPointer[T]](
	h *testEventHarness,
	projection P,
) events.ProjectionHandle[P] {
	return evtstream.NewProjectionHandle(h.js, h.stream, projection, testCoreLogger())
}

func detachedTestProjectionHandle[T any, P evtstream.ProjectionPointer[T]](projection P) events.ProjectionHandle[P] {
	return evtstream.NewProjectionHandle(nil, nil, projection, testCoreLogger())
}

func optionalTestProjectionHandle[T any, P evtstream.ProjectionPointer[T]](
	t *testing.T,
	projection P,
	projector *events.Projector,
) events.ProjectionHandle[P] {
	t.Helper()
	value := reflect.ValueOf(projection)
	if !value.IsValid() || (value.Kind() == reflect.Pointer && value.IsNil()) {
		var zero events.ProjectionHandle[P]
		return zero
	}
	if projector == nil {
		return detachedTestProjectionHandle(projection)
	}
	handle, err := evtstream.BindProjectionHandle(projection, projector)
	if err != nil {
		t.Fatalf("BindProjectionHandle: %v", err)
	}
	return handle
}

func newTestRoomModel(
	t *testing.T,
	directory *RoomDirectoryProjection,
	directoryProjector *events.Projector,
	groupLayout *RoomGroupLayoutProjection,
	groupLayoutProjector *events.Projector,
	timeline *RoomTimelineProjection,
	timelineProjector *events.Projector,
	threads *ThreadProjection,
	threadsProjector *events.Projector,
	reactions *ReactionProjection,
	reactionsProjector *events.Projector,
) *RoomModel {
	t.Helper()
	return newRoomModel(
		optionalTestProjectionHandle(t, directory, directoryProjector),
		optionalTestProjectionHandle(t, groupLayout, groupLayoutProjector),
		optionalTestProjectionHandle(t, timeline, timelineProjector),
		optionalTestProjectionHandle(t, threads, threadsProjector),
		optionalTestProjectionHandle(t, reactions, reactionsProjector),
	)
}

func newTestUserModel(
	t *testing.T,
	publisher *evtstream.Publisher,
	users *UserProjection,
	usersProjector *events.Projector,
	auth *UserAuthProjection,
	authProjector *events.Projector,
	contentKeys *ContentKeyProjection,
	contentKeysProjector *events.Projector,
) *UserModel {
	t.Helper()
	return newUserModel(
		publisher,
		optionalTestProjectionHandle(t, users, usersProjector),
		optionalTestProjectionHandle(t, auth, authProjector),
		optionalTestProjectionHandle(t, contentKeys, contentKeysProjector),
	)
}

func newTestConfigModel(
	t *testing.T,
	publisher *evtstream.Publisher,
	projector *events.Projector,
	projection *ConfigProjection,
) *ConfigModel {
	t.Helper()
	return NewConfigModel(publisher, optionalTestProjectionHandle(t, projection, projector))
}

func newTestRBACModel(t *testing.T, projection *RBACProjection, projector *events.Projector) *RBACModel {
	t.Helper()
	return newRBACModel(optionalTestProjectionHandle(t, projection, projector))
}

func newTestAssetModel(t *testing.T, core *ChattoCore, projection *AssetProjection, projector *events.Projector) *AssetModel {
	t.Helper()
	return NewAssetModel(core, optionalTestProjectionHandle(t, projection, projector))
}

func startTestProjector(t *testing.T, projector *events.Projector) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- projector.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("projector did not stop within timeout")
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for !projector.Started() {
		if time.Now().After(deadline) {
			t.Fatal("projector did not start within timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func testCoreLogger() *log.Logger {
	return log.New(io.Discard)
}
