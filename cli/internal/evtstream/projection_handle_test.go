package evtstream_test

import (
	"testing"

	. "hmans.de/chatto/internal/events"
	. "hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type projectionHandleTestProjection struct {
	subject string
}

func (p *projectionHandleTestProjection) Subjects() []string {
	return []string{p.subject}
}

func (*projectionHandleTestProjection) Apply(*corev1.Event, uint64) error {
	return nil
}

func TestProjectionHandleKeepsProjectionAndProjectorTogether(t *testing.T) {
	projection := &projectionHandleTestProjection{subject: RoomSubjectFilter()}
	handle := NewProjectionHandle(nil, nil, projection, testLogger())

	if handle.Projection() != projection {
		t.Fatal("Projection() did not return the constructed projection")
	}
	if handle.Projector() == nil {
		t.Fatal("Projector() returned nil")
	}
	if rebound, err := BindProjectionHandle(projection, handle.Projector()); err != nil {
		t.Fatalf("BindProjectionHandle() error = %v", err)
	} else if rebound.Projection() != projection || rebound.Projector() != handle.Projector() {
		t.Fatal("BindProjectionHandle() did not preserve the projection runtime")
	}
}

func TestBindProjectionHandleRejectsAnotherProjection(t *testing.T) {
	first := &projectionHandleTestProjection{subject: RoomSubjectFilter()}
	second := &projectionHandleTestProjection{subject: UserSubjectFilter()}
	projector := NewProjector(nil, nil, first, testLogger())

	if _, err := BindProjectionHandle(second, projector); err == nil {
		t.Fatal("BindProjectionHandle() accepted a projector for another projection")
	}
}

func TestProjectionHandleRejectsNilProjection(t *testing.T) {
	var projection *projectionHandleTestProjection

	defer func() {
		if recover() == nil {
			t.Fatal("NewProjectionHandle() accepted a nil projection")
		}
	}()
	NewProjectionHandle(nil, nil, projection, testLogger())
}

func TestBindProjectionHandleRejectsNilProjection(t *testing.T) {
	var projection *projectionHandleTestProjection
	projector := NewProjector(nil, nil, &projectionHandleTestProjection{}, testLogger())

	if _, err := BindProjectionHandle(projection, projector); err == nil {
		t.Fatal("BindProjectionHandle() accepted a nil projection")
	}
}

func TestProjectionHandleZeroValueIsEmpty(t *testing.T) {
	var handle ProjectionHandle[*projectionHandleTestProjection]

	if handle.Projection() != nil || handle.Projector() != nil {
		t.Fatal("zero ProjectionHandle was not empty")
	}
}
