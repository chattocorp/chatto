package evtstream

import (
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

// NewProjector binds a Chatto core-event projection to the generic ordered
// projector lifecycle.
func NewProjector(
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection Projection,
	opts events.ProjectorOptions,
) (*events.Projector, error) {
	return events.NewDecodedProjector(js, stream, projection, decodeEvent, opts)
}

// NewProjectionHandle constructs a typed Chatto projection handle and its
// owning projector.
func NewProjectionHandle[T any, P ProjectionPointer[T]](
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection P,
	opts events.ProjectorOptions,
) (events.ProjectionHandle[P], error) {
	return events.NewDecodedProjectionHandle(js, stream, projection, decodeEvent, opts)
}

// NewPreparedProjectionHandle constructs a typed prepared projection handle
// and its owning projector.
func NewPreparedProjectionHandle[T any, P PreparedProjectionPointer[T]](
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection P,
	opts events.ProjectorOptions,
) (events.ProjectionHandle[P], error) {
	return events.NewDecodedPreparedProjectionHandle(js, stream, projection, decodeEvent, opts)
}

// BindProjectionHandle joins a Chatto projection to the projector that owns
// it, such as the ServerContentView projector for one of its components. It
// panics when the projector owns a different projection.
func BindProjectionHandle[T any, P ProjectionPointer[T]](
	projection P,
	projector *events.Projector,
) events.ProjectionHandle[P] {
	return events.BindDecodedProjectionHandle[T, *evtv1.Event](projection, projector)
}

func decodeEvent(data []byte) (events.DecodedEvent[*evtv1.Event], error) {
	var event evtv1.Event
	if err := proto.Unmarshal(data, &event); err != nil {
		return events.DecodedEvent[*evtv1.Event]{}, err
	}
	return events.DecodedEvent[*evtv1.Event]{Event: &event, ID: event.GetId()}, nil
}
