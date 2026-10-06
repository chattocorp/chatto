package evtstream_test

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	. "hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	. "hmans.de/chatto/pkg/events"
)

// stubJetStream and stubStream satisfy projector construction for projectors
// that never run.
type stubJetStream struct{ jetstream.JetStream }

type stubStream struct{ jetstream.Stream }

type projectionHandleTestProjection struct {
	subject string
}

func (p *projectionHandleTestProjection) Subjects() []string {
	return []string{p.subject}
}

func (*projectionHandleTestProjection) Apply(*evtv1.Event, uint64) error {
	return nil
}

func TestProjectionHandleKeepsProjectionAndProjectorTogether(t *testing.T) {
	projection := &projectionHandleTestProjection{subject: RoomSubjectFilter()}
	handle := must(NewProjectionHandle(stubJetStream{}, stubStream{}, projection, ProjectorOptions{}))

	if handle.Projection() != projection {
		t.Fatal("Projection() did not return the constructed projection")
	}
	if handle.Projector() == nil {
		t.Fatal("Projector() returned nil")
	}
	if rebound := BindProjectionHandle(projection, handle.Projector()); rebound.Projection() != projection || rebound.Projector() != handle.Projector() {
		t.Fatal("BindProjectionHandle() did not preserve the projection runtime")
	}
}

func TestBindProjectionHandleRejectsAnotherProjection(t *testing.T) {
	first := &projectionHandleTestProjection{subject: RoomSubjectFilter()}
	second := &projectionHandleTestProjection{subject: UserSubjectFilter()}
	projector := must(NewProjector(stubJetStream{}, stubStream{}, first, ProjectorOptions{}))

	defer func() {
		if recover() == nil {
			t.Fatal("BindProjectionHandle() accepted a projector for another projection")
		}
	}()
	BindProjectionHandle(second, projector)
}

func TestProjectionHandleRejectsNilProjection(t *testing.T) {
	var projection *projectionHandleTestProjection

	defer func() {
		if recover() == nil {
			t.Fatal("NewProjectionHandle() accepted a nil projection")
		}
	}()
	must(NewProjectionHandle(stubJetStream{}, stubStream{}, projection, ProjectorOptions{}))
}

func TestBindProjectionHandleRejectsNilProjection(t *testing.T) {
	var projection *projectionHandleTestProjection
	projector := must(NewProjector(stubJetStream{}, stubStream{}, &projectionHandleTestProjection{}, ProjectorOptions{}))

	defer func() {
		if recover() == nil {
			t.Fatal("BindProjectionHandle() accepted a nil projection")
		}
	}()
	BindProjectionHandle(projection, projector)
}

func TestProjectionHandleZeroValueIsEmpty(t *testing.T) {
	var handle ProjectionHandle[*projectionHandleTestProjection]

	if handle.Projection() != nil || handle.Projector() != nil {
		t.Fatal("zero ProjectionHandle was not empty")
	}
}
