package video

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/events"
)

type fakeProcessingRuntime struct {
	mu    sync.Mutex
	state core.AssetState
	waits int
}

func (r *fakeProcessingRuntime) WaitForEvent(context.Context, string, uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.waits++
	return nil
}

func (r *fakeProcessingRuntime) AssetState(string) core.AssetState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *fakeProcessingRuntime) setTerminal() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.VideoManifest = &core.VideoAttachmentManifest{Failed: &evtv1.AssetProcessingFailedEvent{AssetId: "A-video"}}
}

type fakeAssetProcessor struct {
	process func(context.Context, string, string) error
	calls   int
}

func (p *fakeAssetProcessor) ProcessAsset(ctx context.Context, assetID, messageID string) error {
	p.calls++
	return p.process(ctx, assetID, messageID)
}

func processingDelivery(t *testing.T) events.DurableDelivery {
	t.Helper()
	event := &evtv1.Event{
		Id: "E-request",
		Event: &evtv1.Event_AssetProcessingStarted{
			AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{
				AssetId:        "A-video",
				MessageEventId: "E-message",
			},
		},
	}
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return events.DurableDelivery{
		Subject:        "evt.asset.A-video.asset_processing_started",
		Data:           data,
		StreamSequence: 42,
	}
}

func TestProcessDeliveryAcknowledgesOnlyAfterTerminalState(t *testing.T) {
	runtime := &fakeProcessingRuntime{}
	processor := &fakeAssetProcessor{process: func(context.Context, string, string) error {
		runtime.setTerminal()
		return errors.New("terminal failure already recorded")
	}}
	msg := processingDelivery(t)

	err := processDelivery(context.Background(), msg, runtime, processor, log.New(io.Discard))

	if processor.calls != 1 || runtime.waits != 1 {
		t.Fatalf("processor calls = %d, projection waits = %d; want 1, 1", processor.calls, runtime.waits)
	}
	if err != nil {
		t.Fatalf("processDelivery = %v, want successful terminal result", err)
	}
}

func TestProcessDeliveryNaksRetryableWork(t *testing.T) {
	runtime := &fakeProcessingRuntime{}
	processor := &fakeAssetProcessor{process: func(context.Context, string, string) error {
		return errors.New("temporary failure")
	}}
	msg := processingDelivery(t)

	err := processDelivery(context.Background(), msg, runtime, processor, log.New(io.Discard))

	if err == nil || err.Error() != "temporary failure" {
		t.Fatalf("processDelivery = %v, want temporary failure", err)
	}
}

func TestAssetProcessingConsumerHandsOffUnackedWork(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "EVT",
		Subjects: []string{"evt.>"},
		Storage:  jetstream.MemoryStorage,
	}); err != nil {
		t.Fatal(err)
	}

	firstWorker, err := createConsumer(ctx, js)
	if err != nil {
		t.Fatal(err)
	}
	secondWorker, err := createConsumer(ctx, js)
	if err != nil {
		t.Fatal(err)
	}
	event := &evtv1.Event{
		Id: "E-request",
		Event: &evtv1.Event_AssetProcessingStarted{
			AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{
				AssetId:        "A-video",
				MessageEventId: "E-message",
			},
		},
	}
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.Publish(ctx, evtstream.AssetAggregate("A-video").SubjectFor(event), data); err != nil {
		t.Fatal(err)
	}

	first := fetchOne(t, firstWorker)
	if err := first.Nak(); err != nil {
		t.Fatal(err)
	}
	second := fetchOne(t, secondWorker)
	if string(second.Data()) != string(data) {
		t.Fatal("second worker did not receive the negatively acknowledged work item")
	}
	if err := second.DoubleAck(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAssetProcessingConsumerConsumesLegacyRoomMarker(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "EVT",
		Subjects: []string{"evt.>"},
		Storage:  jetstream.MemoryStorage,
	}); err != nil {
		t.Fatal(err)
	}

	consumer, err := createConsumer(ctx, js)
	if err != nil {
		t.Fatal(err)
	}
	event := &evtv1.Event{
		Id: "E-legacy-request",
		Event: &evtv1.Event_AssetProcessingStarted{
			AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{
				AssetId:        "A-legacy-video",
				MessageEventId: "E-legacy-message",
			},
		},
	}
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	legacySubject := evtstream.RoomAggregate("R-legacy").SubjectFor(event)
	if _, err := js.Publish(ctx, legacySubject, data); err != nil {
		t.Fatal(err)
	}

	delivery := fetchOne(t, consumer)
	if delivery.Subject() != legacySubject {
		t.Fatalf("delivery subject = %q, want %q", delivery.Subject(), legacySubject)
	}
	if err := delivery.DoubleAck(ctx); err != nil {
		t.Fatal(err)
	}
}

func fetchOne(t *testing.T, consumer jetstream.Consumer) jetstream.Msg {
	t.Helper()
	batch, err := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for msg := range batch.Messages() {
		return msg
	}
	if err := batch.Error(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("consumer returned no work")
	return nil
}
