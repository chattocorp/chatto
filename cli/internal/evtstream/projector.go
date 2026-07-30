package evtstream

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Projection is Chatto's core-event specialization of the codec-neutral
// projection contract.
//
// Apply runs from one projector goroutine in stream order. Implementations must
// be idempotent for duplicate (event, sequence) delivery and must treat durable
// event protobufs as immutable.
type Projection = events.EventProjection[*corev1.Event]

// ProjectionPointer constrains Chatto projection construction to pointers so
// the projector and read side share one projection instance.
type ProjectionPointer[T any] interface {
	Projection
	*T
}

// SequencedEvent pairs one decoded EVT event with its stable stream sequence.
type SequencedEvent = events.SequencedEventOf[*corev1.Event]

// StartupBatchProjection atomically applies groups of Chatto events while a
// projector replays its captured startup history.
type StartupBatchProjection = events.StartupBatchEventProjection[*corev1.Event]

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

// BindProjectionHandle joins a Chatto projection to an already-configured
// projector while verifying that the projector owns the same projection.
func BindProjectionHandle[T any, P ProjectionPointer[T]](
	projection P,
	projector *events.Projector,
) (events.ProjectionHandle[P], error) {
	return events.BindDecodedProjectionHandle[T, *corev1.Event](projection, projector)
}

func decodeEvent(data []byte) (events.DecodedEvent[*corev1.Event], error) {
	var event corev1.Event
	if err := proto.Unmarshal(data, &event); err != nil {
		return events.DecodedEvent[*corev1.Event]{}, err
	}
	return events.DecodedEvent[*corev1.Event]{Event: &event, ID: event.GetId()}, nil
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
	event *corev1.Event,
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
	event *corev1.Event,
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
