// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const serverContentViewSnapshotSemantics = "v1"

// ServerContentView is the sequence-consistent process-local view of
// client-readable EVT state. Focused models keep their own read APIs. The
// bound projector owns their ordered apply, readiness, failure, and snapshot
// lifecycle.
type ServerContentView struct {
	components *events.ComponentizedProjection[*evtv1.Event]
	projector  *events.Projector
}

func newServerContentView(components ...serverContentComponent) *ServerContentView {
	registrations := make([]events.ProjectionComponent[*evtv1.Event], 0, len(components))
	var contract strings.Builder
	for _, component := range components {
		registrations = append(registrations, events.NewProjectionComponent(
			component.key, component.model, component.reducer,
		))
		fmt.Fprintf(&contract, "%s:%s;", component.key, component.model.SnapshotContractID())
	}
	sum := sha256.Sum256([]byte(contract.String()))
	contractID := serverContentViewSnapshotSemantics + "-" + hex.EncodeToString(sum[:8])
	return &ServerContentView{components: events.NewComponentizedProjection(
		[]string{"evt.>"}, contractID, registrations...,
	)}
}

type serverContentComponent struct {
	key     string
	model   events.SnapshotComponentModel
	reducer events.EventReducer[*evtv1.Event]
}

func newServerContentComponent(
	key string,
	model events.SnapshotComponentModel,
	reducer events.EventReducer[*evtv1.Event],
) serverContentComponent {
	return serverContentComponent{key: key, model: model, reducer: reducer}
}

// newInfallibleServerContentComponent is an explicit adapter for an existing
// projection whose Apply implementation has no error paths. A component with
// fallible work must supply its own EventReducer instead.
func newInfallibleServerContentComponent(
	key string,
	model events.SnapshotComponentModel,
	apply func(*evtv1.Event, uint64) error,
) serverContentComponent {
	return serverContentComponent{
		key: key, model: model,
		reducer: events.EventReducerFunc[*evtv1.Event](func(event *evtv1.Event, sequence uint64) (events.PreparedMutation, error) {
			return events.PreparedMutationFunc(func() {
				if err := apply(event, sequence); err != nil {
					panic(fmt.Sprintf("core: prepared server content component %q failed to commit: %v", key, err))
				}
			}), nil
		}),
	}
}

func (v *ServerContentView) bindProjector(projector *events.Projector) {
	if projector == nil {
		panic("core: ServerContentView requires a projector")
	}
	v.projector = projector
}

// Subjects declares the complete EVT stream as the view's logical sequence
// contract. Events that do not change a component still advance the exact
// global sequence represented by the view.
func (v *ServerContentView) Subjects() []string {
	return v.components.Subjects()
}

// Prepare prepares every component mutation before any component changes.
func (v *ServerContentView) Prepare(event *evtv1.Event, sequence uint64) (events.PreparedMutation, error) {
	return v.components.Prepare(event, sequence)
}

// PrepareSubject prepares only components that consume the delivered subject.
func (v *ServerContentView) PrepareSubject(event *evtv1.Event, subject string, sequence uint64) (events.PreparedMutation, error) {
	return v.components.PrepareSubject(event, subject, sequence)
}

// OwnsProjection reports whether model is part of this content view.
func (v *ServerContentView) OwnsProjection(model events.SubjectProjection) bool {
	return v.components.OwnsProjection(model)
}

// SnapshotCohortContractID returns the contract for this exact component set.
func (v *ServerContentView) SnapshotCohortContractID() string {
	return v.components.SnapshotCohortContractID()
}

// SnapshotComponentContracts returns the exact required snapshot component
// set and part limits for repository validation.
func (v *ServerContentView) SnapshotComponentContracts() []events.ProjectionSnapshotComponentContract {
	return v.components.SnapshotComponentContracts()
}

// SnapshotComponents captures the focused model payloads.
func (v *ServerContentView) SnapshotComponents() ([]events.ProjectionSnapshotComponent, error) {
	return v.components.SnapshotComponents()
}

// RestoreComponents restores a complete focused model cohort.
func (v *ServerContentView) RestoreComponents(components []events.ProjectionSnapshotComponent) error {
	return v.components.RestoreComponents(components)
}

// ResetComponents restores the canonical empty state of every focused model.
func (v *ServerContentView) ResetComponents() error {
	return v.components.ResetComponents()
}

// CompleteStartupReplay releases component compatibility state after replay.
func (v *ServerContentView) CompleteStartupReplay() {
	v.components.CompleteStartupReplay()
}

// Read runs one bounded in-memory operation against a stable content-view
// generation and supplies its exact EVT sequence.
func (v *ServerContentView) Read(read func(sequence uint64) error) error {
	if v.projector == nil {
		return fmt.Errorf("ServerContentView projector is not bound")
	}
	return v.projector.WithReadBarrier(read)
}

func (v *ServerContentView) adminProjectionEstimate(components ...events.SnapshotComponentModel) (int64, int64, []ProjectionAdminMetric) {
	var entries int64
	var estimatedBytes int64
	var metrics []ProjectionAdminMetric
	for _, component := range components {
		estimator, ok := component.(interface {
			adminProjectionEstimate() (int64, int64, []ProjectionAdminMetric)
		})
		if !ok {
			continue
		}
		componentEntries, componentBytes, componentMetrics := estimator.adminProjectionEstimate()
		entries += componentEntries
		estimatedBytes += componentBytes
		metrics = append(metrics, componentMetrics...)
	}
	return entries, estimatedBytes, metrics
}
