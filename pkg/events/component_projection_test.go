// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"
)

type componentTestModel struct {
	MemoryProjection
	value       int
	restoreFail bool
	subject     string
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

func TestComponentProjectionPreparationFailureIsAtomic(t *testing.T) {
	first := &componentTestModel{}
	second := &componentTestModel{}
	projection := NewComponentProjection(
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

func TestComponentProjectionRoutesEventsByComponentSubject(t *testing.T) {
	rooms := &componentTestModel{subject: "evt.room.>"}
	users := &componentTestModel{subject: "evt.user.>"}
	projection := NewComponentProjection(
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

func TestComponentProjectionCommitAndReadShareBarrier(t *testing.T) {
	first := &componentTestModel{}
	second := &componentTestModel{}
	prepared := make(chan struct{})
	release := make(chan struct{})
	projection := NewComponentProjection(
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

func TestComponentProjectionRestoreRollsBackCompleteView(t *testing.T) {
	first := &componentTestModel{value: 1}
	second := &componentTestModel{value: 2, restoreFail: true}
	projection := NewComponentProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("first", first, componentIncrementReducer(first, -1)),
		NewProjectionComponent("second", second, componentIncrementReducer(second, -1)),
	)
	err := projection.RestoreComponents([]ProjectionSnapshotComponent{
		{Key: "first", ContractID: "component-test-v1", Parts: [][]byte{[]byte("10")}},
		{Key: "second", ContractID: "component-test-v1", Parts: [][]byte{[]byte("99")}},
	})
	if err == nil {
		t.Fatal("RestoreComponents succeeded despite component restore failure")
	}
	if first.Value() != 1 || second.Value() != 2 {
		t.Fatalf("component values = (%d, %d), want rollback to (1, 2)", first.Value(), second.Value())
	}
}

func TestComponentProjectionSnapshotCaptureUsesOneBarrier(t *testing.T) {
	first := &componentTestModel{value: 4}
	second := &componentTestModel{value: 5}
	projection := NewComponentProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("first", first, componentIncrementReducer(first, -1)),
		NewProjectionComponent("second", second, componentIncrementReducer(second, -1)),
	)
	components, err := projection.SnapshotComponents()
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 2 || string(components[0].Parts[0]) != "4" || string(components[1].Parts[0]) != "5" {
		t.Fatalf("snapshot components = %#v", components)
	}
}

func TestComponentProjectionRejectsDuplicateComponentKeys(t *testing.T) {
	model := &componentTestModel{}
	other := &componentTestModel{}
	defer func() {
		if recover() == nil {
			t.Fatal("NewComponentProjection accepted duplicate component keys")
		}
	}()
	NewComponentProjection(
		[]string{"evt.>"}, "cohort-v1",
		NewProjectionComponent("same", model, componentIncrementReducer(model, -1)),
		NewProjectionComponent("same", other, componentIncrementReducer(other, -1)),
	)
}
