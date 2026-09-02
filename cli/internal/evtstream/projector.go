package evtstream

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// Projection is Chatto's core-event specialization of the codec-neutral
// projection contract.
//
// Apply runs from one projector goroutine in stream order. Implementations must
// be idempotent for duplicate (event, sequence) delivery and must treat durable
// event protobufs as immutable.
type Projection = events.EventProjection[*evtv1.Event]

// ProjectionPointer constrains Chatto projection construction to pointers so
// the projector and read side share one projection instance.
type ProjectionPointer[T any] interface {
	Projection
	*T
}

// PreparedProjection is Chatto's specialization of the prepared projection
// contract.
type PreparedProjection = events.PreparedEventProjection[*evtv1.Event]

// PreparedProjectionPointer constrains prepared Chatto projection
// construction to pointers.
type PreparedProjectionPointer[T any] interface {
	PreparedProjection
	*T
}

// SequencedEvent pairs one decoded EVT event with its stable stream sequence.
type SequencedEvent = events.SequencedEventOf[*evtv1.Event]

// StartupBatchProjection atomically applies groups of Chatto events while a
// projector replays its captured startup history.
type StartupBatchProjection = events.StartupBatchEventProjection[*evtv1.Event]

// NewProjector binds a Chatto core-event projection to the generic ordered
// projector lifecycle.
func NewProjector(
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection Projection,
	logger events.Logger,
) *events.Projector {
	return events.NewDecodedProjector(js, stream, projection, decodeEvent, logger)
}

// NewProjectionHandle constructs a typed Chatto projection handle and its
// owning projector.
func NewProjectionHandle[T any, P ProjectionPointer[T]](
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection P,
	logger events.Logger,
) events.ProjectionHandle[P] {
	return events.NewDecodedProjectionHandle(js, stream, projection, decodeEvent, logger)
}

// NewPreparedProjectionHandle constructs a typed prepared projection handle
// and its owning projector.
func NewPreparedProjectionHandle[T any, P PreparedProjectionPointer[T]](
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection P,
	logger events.Logger,
) events.ProjectionHandle[P] {
	return events.NewDecodedPreparedProjectionHandle(js, stream, projection, decodeEvent, logger)
}

// BindProjectionHandle joins a Chatto projection to an already-configured
// projector while verifying that the projector owns the same projection.
func BindProjectionHandle[T any, P ProjectionPointer[T]](
	projection P,
	projector *events.Projector,
) (events.ProjectionHandle[P], error) {
	return events.BindDecodedProjectionHandle[T, *evtv1.Event](projection, projector)
}

func decodeEvent(data []byte) (events.DecodedEvent[*evtv1.Event], error) {
	var event evtv1.Event
	if err := proto.Unmarshal(data, &event); err != nil {
		return events.DecodedEvent[*evtv1.Event]{}, err
	}
	return events.DecodedEvent[*evtv1.Event]{Event: &event, ID: event.GetId()}, nil
}

// AppendAndWait publishes a Chatto event on its aggregate subject and waits
// until projector has applied the resulting stream position.
//
// A non-zero sequence with an error means the event committed but the local
// projection did not catch up before the context ended.
func (p *Publisher) AppendAndWait(
	ctx context.Context,
	projector *events.Projector,
	aggregate Aggregate,
	event *evtv1.Event,
) (uint64, error) {
	subject := aggregate.SubjectFor(event)
	sequence, err := p.Append(ctx, subject, event)
	if err != nil {
		return 0, err
	}
	if err := projector.WaitFor(ctx, events.SubjectPosition(subject, sequence)); err != nil {
		return sequence, err
	}
	return sequence, nil
}

// AppendEventuallyAndWait is AppendAndWait for append-only facts whose exact
// encoded payload remains safe after an intervening write.
func (p *Publisher) AppendEventuallyAndWait(
	ctx context.Context,
	projector *events.Projector,
	aggregate Aggregate,
	event *evtv1.Event,
) (uint64, error) {
	subject := aggregate.SubjectFor(event)
	sequence, err := p.AppendEventually(ctx, subject, event)
	if err != nil {
		return 0, err
	}
	if err := projector.WaitFor(ctx, events.SubjectPosition(subject, sequence)); err != nil {
		return sequence, err
	}
	return sequence, nil
}
