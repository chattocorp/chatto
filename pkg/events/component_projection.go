// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"
)

// PreparedMutation is an infallible state change produced by an EventReducer.
// Prepare must complete all work that can fail. Commit runs under the owning
// Projector's apply barrier and must not perform external I/O.
type PreparedMutation interface {
	Commit()
}

// PreparedMutationFunc adapts a function to PreparedMutation.
type PreparedMutationFunc func()

// Commit applies the prepared state change.
func (f PreparedMutationFunc) Commit() {
	if f != nil {
		f()
	}
}

// EventReducer prepares one decoded event without changing live state.
// Projector commits the returned mutation only after all reducers prepare the
// event successfully.
type EventReducer[E any] interface {
	Prepare(E, uint64) (PreparedMutation, error)
}

// EventReducerFunc adapts a function to EventReducer.
type EventReducerFunc[E any] func(E, uint64) (PreparedMutation, error)

// Prepare prepares one event mutation.
func (f EventReducerFunc[E]) Prepare(event E, sequence uint64) (PreparedMutation, error) {
	return f(event, sequence)
}

// PreparedEventProjection separates fallible event preparation from
// infallible state mutation behind one Projector apply barrier.
type PreparedEventProjection[E any] interface {
	SubjectProjection
	EventReducer[E]
}

// ProjectionSnapshotPart is one stable, independently stored part of a
// projection component snapshot.
type ProjectionSnapshotPart struct {
	Key     string
	Payload []byte
}

// ProjectionSnapshotComponent is one independently serialized component in a
// projection snapshot cohort. Part keys are stable within its contract.
type ProjectionSnapshotComponent struct {
	Key        string
	ContractID string
	Parts      []ProjectionSnapshotPart
}

// ProjectionSnapshotComponentContract identifies one required component and
// bounds the number of payload parts that a snapshot source can load.
type ProjectionSnapshotComponentContract struct {
	Key        string
	ContractID string
	MaxParts   int
}

// ProjectionSnapshotCohort is component state captured or restored at one
// event-log cutoff. A cohort is installed as one unit.
type ProjectionSnapshotCohort struct {
	GenerationID   string
	ContractID     string
	StreamName     string
	CutoffSequence uint64
	StreamIdentity string
	CreatedAt      time.Time
	Components     []ProjectionSnapshotComponent
}

// ProjectionSnapshotCohortLoadRequest contains the repository constraints for
// one projection snapshot cohort.
type ProjectionSnapshotCohortLoadRequest struct {
	ProjectionKey  string
	ContractID     string
	StreamName     string
	StreamIdentity string
	MaxCutoff      uint64
	Components     []ProjectionSnapshotComponentContract
}

// ProjectionSnapshotCohortSource loads one complete projection snapshot
// cohort. It must not return a partial generation.
type ProjectionSnapshotCohortSource interface {
	LoadProjectionSnapshotCohort(context.Context, ProjectionSnapshotCohortLoadRequest) (ProjectionSnapshotCohort, error)
}

type snapshotCohortProjectionState interface {
	SnapshotComponents() ([]ProjectionSnapshotComponent, error)
	RestoreComponents([]ProjectionSnapshotComponent) error
	ResetComponents() error
	SnapshotCohortContractID() string
	SnapshotComponentContracts() []ProjectionSnapshotComponentContract
}

// SnapshotComponentModel is a focused projection model with an independent
// snapshot contract. Its Subjects declaration remains available to focused
// application diagnostics even when one Projector owns replay for the
// componentized projection.
type SnapshotComponentModel interface {
	SubjectProjection
	Snapshot() ([]byte, error)
	Restore([]byte) error
	SnapshotContractID() string
}

// ProjectionComponent binds one focused reducer/model to its stable snapshot
// key. The model remains directly available to application read code. The
// componentized projection includes it in one combined mutation and snapshot
// shape, while the Projector owns the ordered lifecycle.
type ProjectionComponent[E any] struct {
	key     string
	model   SnapshotComponentModel
	reducer EventReducer[E]
	filters []compiledSubjectFilter
}

// NewProjectionComponent constructs one component registration. key must be a
// stable, path-safe application identifier. model must be a non-nil pointer
// with a snapshot contract, and reducer must prepare changes for that model.
func NewProjectionComponent[E any](key string, model SnapshotComponentModel, reducer EventReducer[E]) ProjectionComponent[E] {
	if key == "" {
		panic("events: projection component key is required")
	}
	if isNilProjection(model) || reflect.ValueOf(model).Kind() != reflect.Pointer {
		panic("events: projection component requires a non-nil pointer model")
	}
	if reducer == nil {
		panic("events: projection component reducer is required")
	}
	if model.SnapshotContractID() == "" {
		panic("events: projection component snapshot contract is required")
	}
	return ProjectionComponent[E]{
		key: key, model: model, reducer: reducer,
		filters: compileSubjectFilters(model.Subjects()),
	}
}

// ComponentizedProjection combines focused reducers and snapshot models into
// one projection. Its Projector owns the ordered apply barrier and sequence.
type ComponentizedProjection[E any] struct {
	subjects   []string
	components []ProjectionComponent[E]
	contractID string
}

// NewComponentizedProjection constructs one componentized projection.
// contractID is the restore-equivalence contract for the registered component
// set and ordering. Subjects is the logical wait and replay contract for the
// complete view.
func NewComponentizedProjection[E any](subjects []string, contractID string, components ...ProjectionComponent[E]) *ComponentizedProjection[E] {
	if len(subjects) == 0 {
		panic("events: componentized projection requires subjects")
	}
	if contractID == "" {
		panic("events: componentized projection contract is required")
	}
	if len(components) == 0 {
		panic("events: componentized projection requires components")
	}
	seen := make(map[string]struct{}, len(components))
	for _, component := range components {
		if _, ok := seen[component.key]; ok {
			panic(fmt.Sprintf("events: duplicate projection component key %q", component.key))
		}
		seen[component.key] = struct{}{}
	}
	return &ComponentizedProjection[E]{
		subjects:   slices.Clone(subjects),
		components: slices.Clone(components),
		contractID: contractID,
	}
}

// Subjects returns the complete projection's logical subject filters.
func (p *ComponentizedProjection[E]) Subjects() []string {
	return slices.Clone(p.subjects)
}

// Prepare asks every reducer to prepare before it commits any component. A
// preparation error leaves every component unchanged.
func (p *ComponentizedProjection[E]) Prepare(event E, sequence uint64) (PreparedMutation, error) {
	return p.prepareComponents(event, "", sequence)
}

// PrepareSubject prepares only components whose logical subject filters match
// the delivered record. The parent projection still advances its global
// sequence for every record in its own subject contract.
func (p *ComponentizedProjection[E]) PrepareSubject(event E, subject string, sequence uint64) (PreparedMutation, error) {
	return p.prepareComponents(event, subject, sequence)
}

func (p *ComponentizedProjection[E]) prepareComponents(event E, subject string, sequence uint64) (PreparedMutation, error) {
	mutations := make([]PreparedMutation, 0, len(p.components))
	for _, component := range p.components {
		if subject != "" && !matchesAnySubject(component.filters, subject) {
			continue
		}
		mutation, err := component.reducer.Prepare(event, sequence)
		if err != nil {
			return nil, fmt.Errorf("prepare projection component %q: %w", component.key, err)
		}
		if mutation != nil {
			mutations = append(mutations, mutation)
		}
	}
	return PreparedMutationFunc(func() {
		for _, mutation := range mutations {
			mutation.Commit()
		}
	}), nil
}

func matchesAnySubject(filters []compiledSubjectFilter, subject string) bool {
	for i := range filters {
		if filters[i].matches(subject) {
			return true
		}
	}
	return false
}

// OwnsProjection reports whether model is one of the registered focused
// models. Projector uses this to create typed handles that share its frontier.
func (p *ComponentizedProjection[E]) OwnsProjection(model SubjectProjection) bool {
	if isNilProjection(model) {
		return false
	}
	for _, component := range p.components {
		if sameProjection(component.model, model) {
			return true
		}
	}
	return false
}

// SnapshotCohortContractID returns the contract for the component set.
func (p *ComponentizedProjection[E]) SnapshotCohortContractID() string {
	return p.contractID
}

// SnapshotComponentContracts returns the exact component set that a snapshot
// source must validate before it loads payload parts.
func (p *ComponentizedProjection[E]) SnapshotComponentContracts() []ProjectionSnapshotComponentContract {
	contracts := make([]ProjectionSnapshotComponentContract, 0, len(p.components))
	for _, component := range p.components {
		contracts = append(contracts, ProjectionSnapshotComponentContract{
			Key: component.key, ContractID: component.model.SnapshotContractID(), MaxParts: 1,
		})
	}
	return contracts
}

// SnapshotComponents serializes every component in registration order.
func (p *ComponentizedProjection[E]) SnapshotComponents() ([]ProjectionSnapshotComponent, error) {
	components := make([]ProjectionSnapshotComponent, 0, len(p.components))
	for _, component := range p.components {
		payload, err := component.model.Snapshot()
		if err != nil {
			return nil, fmt.Errorf("snapshot projection component %q: %w", component.key, err)
		}
		components = append(components, ProjectionSnapshotComponent{
			Key:        component.key,
			ContractID: component.model.SnapshotContractID(),
			Parts:      []ProjectionSnapshotPart{{Key: "state", Payload: payload}},
		})
	}
	return components, nil
}

// RestoreComponents installs a complete compatible component set. It restores
// the prior state if any component rejects its payload.
func (p *ComponentizedProjection[E]) RestoreComponents(stored []ProjectionSnapshotComponent) error {
	if len(stored) != len(p.components) {
		return fmt.Errorf("projection component count is %d, want %d", len(stored), len(p.components))
	}
	byKey := make(map[string]ProjectionSnapshotComponent, len(stored))
	for _, component := range stored {
		if _, ok := byKey[component.Key]; ok {
			return fmt.Errorf("duplicate projection component %q", component.Key)
		}
		byKey[component.Key] = component
	}

	previous := make([][]byte, len(p.components))
	for i, component := range p.components {
		storedComponent, ok := byKey[component.key]
		if !ok {
			return fmt.Errorf("projection component %q is missing", component.key)
		}
		if storedComponent.ContractID != component.model.SnapshotContractID() {
			return fmt.Errorf("projection component %q contract does not match", component.key)
		}
		if len(storedComponent.Parts) != 1 || storedComponent.Parts[0].Key != "state" {
			return fmt.Errorf("projection component %q has %d parts, want 1", component.key, len(storedComponent.Parts))
		}
		payload, err := component.model.Snapshot()
		if err != nil {
			return fmt.Errorf("capture projection component %q before restore: %w", component.key, err)
		}
		previous[i] = payload
	}

	for i, component := range p.components {
		if err := component.model.Restore(byKey[component.key].Parts[0].Payload); err != nil {
			restoreErr := fmt.Errorf("restore projection component %q: %w", component.key, err)
			var rollbackErrs []error
			for rollbackIndex := 0; rollbackIndex <= i; rollbackIndex++ {
				rollback := p.components[rollbackIndex]
				if rollbackErr := rollback.model.Restore(previous[rollbackIndex]); rollbackErr != nil {
					rollbackErrs = append(rollbackErrs, fmt.Errorf("roll back projection component %q: %w", rollback.key, rollbackErr))
				}
			}
			return errors.Join(append([]error{restoreErr}, rollbackErrs...)...)
		}
	}
	return nil
}

// ResetComponents installs every component's canonical empty state as one
// transaction.
func (p *ComponentizedProjection[E]) ResetComponents() error {
	stored := make([]ProjectionSnapshotComponent, 0, len(p.components))
	for _, component := range p.components {
		stored = append(stored, ProjectionSnapshotComponent{
			Key: component.key, ContractID: component.model.SnapshotContractID(),
			Parts: []ProjectionSnapshotPart{{Key: "state"}},
		})
	}
	return p.RestoreComponents(stored)
}

// CompleteStartupReplay forwards the lifecycle boundary to every component
// that retains replay-only compatibility state.
func (p *ComponentizedProjection[E]) CompleteStartupReplay() {
	for _, component := range p.components {
		if completer, ok := component.model.(StartupReplayCompleter); ok {
			completer.CompleteStartupReplay()
		}
	}
}

func sameProjection(left, right SubjectProjection) bool {
	leftValue := reflect.ValueOf(left)
	rightValue := reflect.ValueOf(right)
	return leftValue.IsValid() && rightValue.IsValid() &&
		leftValue.Kind() == reflect.Pointer && rightValue.Kind() == reflect.Pointer &&
		leftValue.Pointer() == rightValue.Pointer()
}
