// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type componentTestModel struct {
	MemoryProjection
	value       int
	restoreFail bool
	subject     string
}

type blockingRestoreComponentModel struct {
	*componentTestModel
	started chan struct{}
	release chan struct{}
}

func (m *blockingRestoreComponentModel) Restore(payload []byte) error {
	if string(payload) == "9" {
		close(m.started)
		<-m.release
	}
	return m.componentTestModel.Restore(payload)
}

type fixedComponentCohortSource struct {
	cohort ProjectionSnapshotCohort
}

func (s fixedComponentCohortSource) LoadProjectionSnapshotCohort(context.Context, ProjectionSnapshotCohortLoadRequest) (ProjectionSnapshotCohort, error) {
	return s.cohort, nil
}

func (m *componentTestModel) Subjects() []string {
	if m.subject != "" {
		return []string{m.subject}
	}
	return []string{"evt.>"}
}
func (*componentTestModel) Apply(int, uint64) error {
	return nil
}
func (m *componentTestModel) SnapshotContractID() string { return "component-test-v1" }
func (m *componentTestModel) Snapshot() ([]byte, error) {
	m.RLock()
	defer m.RUnlock()
	return []byte(strconv.Itoa(m.value)), nil
}
func (m *componentTestModel) Restore(payload []byte) error {
	if m.restoreFail && string(payload) == "99" {
		return errors.New("restore failed")
	}
	value := 0
	if len(payload) > 0 {
		var err error
		value, err = strconv.Atoi(string(payload))
		if err != nil {
			return err
		}
	}
	m.Lock()
	m.value = value
	m.Unlock()
	return nil
}
func (m *componentTestModel) Value() int {
	m.RLock()
	defer m.RUnlock()
	return m.value
}

func componentIncrementReducer(model *componentTestModel, failOn int) EventReducer[int] {
	return EventReducerFunc[int](func(event int, _ uint64) (PreparedMutation, error) {
		if event == failOn {
			return nil, fmt.Errorf("rejected %d", event)
		}
		return PreparedMutationFunc(func() {
			model.Lock()
			model.value += event
			model.Unlock()
		}), nil
	})
}

func TestComponentizedProjectionPreparationFailureIsAtomic(t *testing.T) {
	first := &componentTestModel{}
	second := &componentTestModel{}
	projection := NewComponentizedProjection(
		[]string{"evt.>"},
		"cohort-v1",
		NewProjectionComponent("first", first, componentIncrementReducer(first, -1)),
		NewProjectionComponent("second", second, componentIncrementReducer(second, 2)),
	)
	projector := NewDecodedPreparedProjector(
		nil, nil, projection,
		func([]byte) (DecodedEvent[int], error) { return DecodedEvent[int]{}, nil },
		discardLogger{},
	)

	failureSeq, err := projector.applyEvent(typedDecodedEvent[int]{event: 2, id: "event"}, "evt.test.changed", 7)
	if err == nil {
		t.Fatal("applyEvent succeeded despite component preparation failure")
	}
	if failureSeq != 7 {
		t.Fatalf("failure sequence = %d, want 7", failureSeq)
	}
	if first.Value() != 0 || second.Value() != 0 {
		t.Fatalf("component values = (%d, %d), want unchanged", first.Value(), second.Value())
	}
	if projector.LastSeq() != 0 {
		t.Fatalf("last sequence = %d, want 0", projector.LastSeq())
	}
}

func TestComponentizedProjectionRoutesEventsByComponentSubject(t *testing.T) {
	rooms := &componentTestModel{subject: "evt.room.>"}
	users := &componentTestModel{subject: "evt.user.>"}
	projection := NewComponentizedProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("rooms", rooms, componentIncrementReducer(rooms, -1)),
		NewProjectionComponent("users", users, componentIncrementReducer(users, -1)),
	)
	mutation, err := projection.PrepareSubject(4, "evt.room.room-1.created", 9)
	if err != nil {
		t.Fatal(err)
	}
	mutation.Commit()
	if rooms.Value() != 4 || users.Value() != 0 {
		t.Fatalf("component values = (%d, %d), want (4, 0)", rooms.Value(), users.Value())
	}
}

func TestComponentizedProjectionCommitAndReadShareBarrier(t *testing.T) {
	first := &componentTestModel{}
	second := &componentTestModel{}
	prepared := make(chan struct{})
	release := make(chan struct{})
	projection := NewComponentizedProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("first", first, EventReducerFunc[int](func(event int, _ uint64) (PreparedMutation, error) {
			return PreparedMutationFunc(func() {
				first.Lock()
				first.value += event
				first.Unlock()
				close(prepared)
				<-release
			}), nil
		})),
		NewProjectionComponent("second", second, componentIncrementReducer(second, -1)),
	)
	projector := NewDecodedPreparedProjector(
		nil, nil, projection,
		func([]byte) (DecodedEvent[int], error) { return DecodedEvent[int]{}, nil },
		discardLogger{},
	)

	applied := make(chan error, 1)
	go func() {
		_, err := projector.applyEvent(typedDecodedEvent[int]{event: 3, id: "event"}, "evt.test.changed", 11)
		applied <- err
	}()
	<-prepared

	readDone := make(chan struct{})
	var readSeq uint64
	var values [2]int
	go func() {
		_ = projector.WithReadBarrier(func(sequence uint64) error {
			readSeq = sequence
			values = [2]int{first.Value(), second.Value()}
			return nil
		})
		close(readDone)
	}()
	select {
	case <-readDone:
		t.Fatal("read crossed an in-progress component commit")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-applied; err != nil {
		t.Fatal(err)
	}
	<-readDone
	if readSeq != 11 || values != [2]int{3, 3} {
		t.Fatalf("read = seq %d values %v, want seq 11 values [3 3]", readSeq, values)
	}
}

func TestComponentizedProjectionRestoreRollsBackCompleteView(t *testing.T) {
	first := &componentTestModel{value: 1}
	second := &componentTestModel{value: 2, restoreFail: true}
	projection := NewComponentizedProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("first", first, componentIncrementReducer(first, -1)),
		NewProjectionComponent("second", second, componentIncrementReducer(second, -1)),
	)
	err := projection.RestoreComponents([]ProjectionSnapshotComponent{
		{Key: "first", ContractID: "component-test-v1", Parts: []ProjectionSnapshotPart{{Key: "state", Payload: []byte("10")}}},
		{Key: "second", ContractID: "component-test-v1", Parts: []ProjectionSnapshotPart{{Key: "state", Payload: []byte("99")}}},
	})
	if err == nil {
		t.Fatal("RestoreComponents succeeded despite component restore failure")
	}
	if first.Value() != 1 || second.Value() != 2 {
		t.Fatalf("component values = (%d, %d), want rollback to (1, 2)", first.Value(), second.Value())
	}
}

func TestComponentizedProjectionSnapshotCaptureUsesOneBarrier(t *testing.T) {
	first := &componentTestModel{value: 4}
	second := &componentTestModel{value: 5}
	projection := NewComponentizedProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("first", first, componentIncrementReducer(first, -1)),
		NewProjectionComponent("second", second, componentIncrementReducer(second, -1)),
	)
	components, err := projection.SnapshotComponents()
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 2 || components[0].Parts[0].Key != "state" ||
		string(components[0].Parts[0].Payload) != "4" || string(components[1].Parts[0].Payload) != "5" {
		t.Fatalf("snapshot components = %#v", components)
	}
}

func TestComponentizedProjectionRestoreUsesReadBarrier(t *testing.T) {
	connection := startTestNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.CreateOrUpdateStream(t.Context(), jetstream.StreamConfig{
		Name: "COMPONENT_RESTORE_TEST", Subjects: []string{"evt.>"}, Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	base := &componentTestModel{}
	model := &blockingRestoreComponentModel{
		componentTestModel: base, started: make(chan struct{}), release: make(chan struct{}),
	}
	projection := NewComponentizedProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("state", model, componentIncrementReducer(base, -1)),
	)
	projector := NewDecodedPreparedProjector(
		js, stream, projection,
		func([]byte) (DecodedEvent[int], error) { return DecodedEvent[int]{}, nil },
		discardLogger{},
	)
	source := fixedComponentCohortSource{cohort: ProjectionSnapshotCohort{
		GenerationID: "generation", ContractID: "cohort-v1", StreamName: "COMPONENT_RESTORE_TEST",
		StreamIdentity: "stream", Components: []ProjectionSnapshotComponent{{
			Key: "state", ContractID: "component-test-v1",
			Parts: []ProjectionSnapshotPart{{Key: "state", Payload: []byte("9")}},
		}},
	}}
	if err := projector.ConfigureSnapshotCohorts("component", source, func(*jetstream.StreamInfo) (string, error) {
		return "stream", nil
	}); err != nil {
		t.Fatal(err)
	}
	runContext, cancelRun := context.WithCancel(t.Context())
	defer cancelRun()
	runDone := make(chan error, 1)
	go func() { runDone <- projector.Run(runContext) }()
	<-model.started

	readDone := make(chan error, 1)
	go func() {
		readDone <- projector.WithReadBarrier(func(uint64) error { return nil })
	}()
	select {
	case err := <-readDone:
		t.Fatalf("read crossed an in-progress component restore: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(model.release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	cancelRun()
	if err := <-runDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context cancellation", err)
	}
}

func TestComponentizedProjectionRejectsDuplicateComponentKeys(t *testing.T) {
	model := &componentTestModel{}
	other := &componentTestModel{}
	defer func() {
		if recover() == nil {
			t.Fatal("NewComponentizedProjection accepted duplicate component keys")
		}
	}()
	NewComponentizedProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("same", model, componentIncrementReducer(model, -1)),
		NewProjectionComponent("same", other, componentIncrementReducer(other, -1)),
	)
}
