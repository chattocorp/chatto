package events

import (
	"fmt"
	"log/slog"
	"reflect"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
)

// MemoryProjection is an embeddable base for projections whose state lives
// entirely in process memory. It contributes only a sync.RWMutex for read/write
// coordination; projections opt into snapshot persistence by implementing
// SnapshotProjection explicitly.
//
// Embed by value — the zero mutex is ready to use. Subclasses still
// implement Subjects() and Apply(). Future non-memory projection types
// (KV-backed, file-backed) would have their own embed-friendly base.
type MemoryProjection struct {
	sync.RWMutex
}

// ProjectionHandle keeps a typed projection and the Projector constructed for
// it together. Application wiring passes the handle as one value so a read
// model cannot accidentally receive another projection's replay frontier.
//
// The zero value is valid for partial test wiring: Projection returns the zero
// value of P and Projector returns nil.
type ProjectionHandle[P SubjectProjection] struct {
	projection P
	projector  *Projector
}

// EventProjectionPointer constrains decoded handle construction to projection
// pointers so the projector and read side cannot receive separate value copies.
type EventProjectionPointer[T, E any] interface {
	EventProjection[E]
	*T
}

// PreparedEventProjectionPointer constrains prepared projection construction
// to pointers so the projector and read side share one projection instance.
type PreparedEventProjectionPointer[T, E any] interface {
	PreparedEventProjection[E]
	*T
}

// NewDecodedProjectionHandle constructs a typed projection handle using an
// application-supplied event decoder.
func NewDecodedProjectionHandle[T, E any, P EventProjectionPointer[T, E]](
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection P,
	decoder EventDecoder[E],
	logger *slog.Logger,
) ProjectionHandle[P] {
	if projection == nil {
		panic("events: decoded projection handle requires a non-nil projection")
	}
	return ProjectionHandle[P]{
		projection: projection,
		projector:  NewDecodedProjector(js, stream, projection, decoder, logger),
	}
}

// NewDecodedPreparedProjectionHandle constructs a typed prepared projection
// handle using an application-supplied event decoder.
func NewDecodedPreparedProjectionHandle[T, E any, P PreparedEventProjectionPointer[T, E]](
	js jetstream.JetStream,
	stream jetstream.Stream,
	projection P,
	decoder EventDecoder[E],
	logger *slog.Logger,
) ProjectionHandle[P] {
	if projection == nil {
		panic("events: decoded prepared projection handle requires a non-nil projection")
	}
	return ProjectionHandle[P]{
		projection: projection,
		projector:  NewDecodedPreparedProjector(js, stream, projection, decoder, logger),
	}
}

// BindDecodedProjectionHandle joins a decoded event projection to an
// already-constructed Projector. It rejects a projector that owns a different
// projection. Prefer NewDecodedProjectionHandle when constructing a new
// runtime; this adapter exists for lifecycle code that must configure the
// Projector before handing it onward.
func BindDecodedProjectionHandle[T, E any, P EventProjectionPointer[T, E]](projection P, projector *Projector) (ProjectionHandle[P], error) {
	if projection == nil {
		return ProjectionHandle[P]{}, fmt.Errorf("projection is nil")
	}
	if projector == nil {
		return ProjectionHandle[P]{}, fmt.Errorf("projection projector is nil")
	}
	if !projector.ownsProjection(projection) {
		return ProjectionHandle[P]{}, fmt.Errorf("projector owns a different projection")
	}
	return ProjectionHandle[P]{projection: projection, projector: projector}, nil
}

// Projection returns the typed read model owned by the handle.
func (h ProjectionHandle[P]) Projection() P {
	return h.projection
}

// Projector returns the replay, readiness, and failure lifecycle for
// Projection.
func (h ProjectionHandle[P]) Projector() *Projector {
	return h.projector
}

// DecodedEvent is one application event produced from an opaque event-log
// record. ID is a stable, non-sensitive identifier used only for diagnostics.
type DecodedEvent[E any] struct {
	Event E
	ID    string
}

// EventDecoder turns an opaque event-log record into an application event.
// Applications own the envelope and codec; the projector owns ordered replay,
// readiness, and failure handling.
type EventDecoder[E any] func([]byte) (DecodedEvent[E], error)

// SubjectProjection declares the durable subjects consumed by a projection.
type SubjectProjection interface {
	// Subjects returns the subject filter(s) this projection consumes.
	// Wildcards are supported.
	Subjects() []string
}

// EventProjection is the codec-neutral read side. Implementations consume
// decoded application events from a subject filter and serve derived reads.
type EventProjection[E any] interface {
	SubjectProjection

	// Apply is called for every decoded event matching Subjects(), in stream
	// order. seq is the stable stream sequence of this event.
	Apply(event E, seq uint64) error
}

// SequencedEventOf pairs one decoded application event with its stable stream
// sequence. StartupBatchEventProjection receives these in strictly increasing
// stream order.
type SequencedEventOf[E any] struct {
	Event    E
	Sequence uint64
}

// StartupBatchEventProjection atomically applies groups of decoded events
// while a projector replays its captured startup history.
type StartupBatchEventProjection[E any] interface {
	EventProjection[E]
	StartupBatchSize() int
	ApplyStartupBatch([]SequencedEventOf[E]) error
}

// ReplaySubjectProjection can be implemented when a projection's logical
// consumed subjects are narrower than the physical filters its ordered
// consumer should use. Waits and diagnostics still report the narrower
// Subjects contract.
type ReplaySubjectProjection interface {
	ReplaySubjects() []string
}

// StartupReplayCompleter can be implemented by projections that retain
// temporary state only while replaying the stream at process startup. The
// Projector calls CompleteStartupReplay exactly once after every event through
// the captured startup target has been applied. It is also called for an empty
// or already-current projection.
//
// The hook runs inside the Projector's apply barrier, so no event is applied
// concurrently with it. It must not call Projector methods that take the
// barrier or wait for startup, such as WithReadBarrier, CaptureSnapshot, or
// WaitForStartup. Status reports StartupComplete and WaitForStartup returns
// only after the hook has returned.
type StartupReplayCompleter interface {
	CompleteStartupReplay()
}

func isNilProjection(projection SubjectProjection) bool {
	if projection == nil {
		return true
	}
	value := reflect.ValueOf(projection)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
