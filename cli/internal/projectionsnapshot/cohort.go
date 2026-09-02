// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package projectionsnapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	projectionv1 "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
)

const (
	maxCohortComponents  = 64
	maxPartsPerComponent = 1024
	maxCohortPayloadSize = 1 << 30
)

// CohortPart is one stable, independently stored part of a component.
type CohortPart struct {
	Key     string
	Payload []byte
}

// CohortComponent contains the payload parts for one projection model.
type CohortComponent struct {
	Key        string
	ContractID string
	Parts      []CohortPart
}

// CohortComponentContract identifies one required component and bounds the
// number of payload parts that the repository can load for it.
type CohortComponentContract struct {
	Key        string
	ContractID string
	MaxParts   int
}

// SaveCohortInput describes one complete projection snapshot cohort.
type SaveCohortInput struct {
	ProjectionKey  string
	ContractID     string
	StreamName     string
	StreamIdentity string
	CutoffSequence uint64
	Components     []CohortComponent
	RefreshAge     time.Duration
	ClockSkew      time.Duration
}

// LoadedCohort is one complete compatible projection snapshot cohort.
type LoadedCohort struct {
	GenerationID    string
	CutoffSequence  uint64
	StreamIdentity  string
	Components      []CohortComponent
	CreatedAt       time.Time
	ProducerVersion string
}

// SaveCohort stores each component part independently and publishes one
// encrypted manifest pointer after every part is durable.
func (r *Repository) SaveCohort(ctx context.Context, input SaveCohortInput) (LoadedCohort, error) {
	if err := validateCohortInput(input, r.maxPayloadSize); err != nil {
		return LoadedCohort{}, err
	}
	pointer, pointerRevision, err := r.loadPointerAtRevision(ctx, input.ProjectionKey, input.ContractID)
	switch {
	case err == nil:
	case errors.Is(err, ErrSnapshotNotFound):
		pointer = &projectionv1.ProjectionSnapshotPointer{}
	case errors.Is(err, errInvalidPointer):
		r.logWarn("Projection snapshot pointer invalid; replacing it", input.ProjectionKey, "pointer_read", err)
		pointer = &projectionv1.ProjectionSnapshotPointer{}
	default:
		return LoadedCohort{}, fmt.Errorf("read snapshot pointer: %w", err)
	}
	if pointer.GetCurrentGenerationId() != "" &&
		pointer.GetCurrentStreamIdentity() == input.StreamIdentity &&
		input.CutoffSequence < pointer.GetCurrentCutoffSequence() {
		return LoadedCohort{}, fmt.Errorf("%w: cutoff %d is older than current cutoff %d", ErrSnapshotRegressed, input.CutoffSequence, pointer.GetCurrentCutoffSequence())
	}
	if input.RefreshAge > 0 && pointer.GetCurrentGenerationId() != "" &&
		pointer.GetCurrentStreamIdentity() == input.StreamIdentity &&
		input.CutoffSequence == pointer.GetCurrentCutoffSequence() && pointer.GetCurrentCreatedAt() != nil {
		createdAt := pointer.GetCurrentCreatedAt().AsTime()
		now := r.now().UTC()
		if createdAt.After(now.Add(-input.RefreshAge)) && !createdAt.After(now.Add(input.ClockSkew)) {
			loaded, loadErr := r.loadCohortGeneration(ctx, pointer.GetCurrentGenerationId(),
				input.ProjectionKey, input.ContractID, input.StreamName, input.StreamIdentity,
				input.CutoffSequence, cohortContractsFromComponents(input.Components))
			if loadErr == nil && loaded.CreatedAt.After(now.Add(-input.RefreshAge)) &&
				!loaded.CreatedAt.After(now.Add(input.ClockSkew)) {
				loaded.Components = nil
				return loaded, ErrSnapshotFresh
			}
		}
	}

	started := time.Now()
	createdAt := r.now().UTC()
	manifest := &projectionv1.ProjectionSnapshotCohortManifest{}
	writtenKeys := make([]string, 0, len(input.Components)+1)
	cleanupWritten := func() {
		for _, key := range writtenKeys {
			if err := r.blobs.Delete(ctx, key); err != nil && !errors.Is(err, ErrBlobNotFound) {
				r.logWarn("Unpublished projection snapshot cleanup failed", input.ProjectionKey, "publish_rollback", err)
			}
		}
	}

	for _, component := range input.Components {
		manifestComponent := &projectionv1.ProjectionSnapshotCohortComponent{
			ComponentKey: component.Key, ContractId: component.ContractID,
		}
		partProjectionKey := cohortPartProjectionKey(input.ProjectionKey, component.Key)
		for _, part := range component.Parts {
			loaded, objectKey, _, err := r.writeUnpublishedGeneration(ctx, SaveInput{
				ProjectionKey: partProjectionKey, ContractID: component.ContractID,
				StreamName: input.StreamName, StreamIdentity: input.StreamIdentity,
				CutoffSequence: input.CutoffSequence, Payload: part.Payload,
			}, createdAt)
			if err != nil {
				cleanupWritten()
				return LoadedCohort{}, fmt.Errorf("store component %q: %w", component.Key, err)
			}
			writtenKeys = append(writtenKeys, objectKey)
			manifestComponent.Parts = append(manifestComponent.Parts, &projectionv1.ProjectionSnapshotCohortPart{
				PartKey: part.Key, GenerationId: loaded.GenerationID,
			})
		}
		manifest.Components = append(manifest.Components, manifestComponent)
	}
	manifestPayload, err := proto.MarshalOptions{Deterministic: true}.Marshal(manifest)
	if err != nil {
		cleanupWritten()
		return LoadedCohort{}, fmt.Errorf("marshal snapshot cohort manifest: %w", err)
	}
	manifestGeneration, manifestObjectKey, _, err := r.writeUnpublishedGeneration(ctx, SaveInput{
		ProjectionKey: input.ProjectionKey, ContractID: input.ContractID,
		StreamName: input.StreamName, StreamIdentity: input.StreamIdentity,
		CutoffSequence: input.CutoffSequence, Payload: manifestPayload,
	}, createdAt)
	if err != nil {
		cleanupWritten()
		return LoadedCohort{}, fmt.Errorf("store snapshot cohort manifest: %w", err)
	}
	writtenKeys = append(writtenKeys, manifestObjectKey)

	droppedID := pointer.GetPreviousGenerationId()
	droppedIdentity := pointer.GetPreviousStreamIdentity()
	droppedCutoff := pointer.GetPreviousCutoffSequence()
	pointer.PreviousGenerationId = pointer.GetCurrentGenerationId()
	pointer.PreviousCutoffSequence = pointer.GetCurrentCutoffSequence()
	pointer.PreviousStreamIdentity = pointer.GetCurrentStreamIdentity()
	pointer.PreviousCompatibilityId = pointer.GetCurrentCompatibilityId()
	pointer.PreviousCreatedAt = pointer.GetCurrentCreatedAt()
	pointer.CurrentGenerationId = manifestGeneration.GenerationID
	pointer.CurrentCutoffSequence = input.CutoffSequence
	pointer.CurrentStreamIdentity = input.StreamIdentity
	pointer.CurrentCompatibilityId = input.ContractID
	pointer.CurrentCreatedAt = timestamppb.New(createdAt)
	if err := r.savePointer(ctx, input.ProjectionKey, input.ContractID, pointer, pointerRevision); err != nil {
		cleanupWritten()
		return LoadedCohort{}, fmt.Errorf("publish snapshot cohort pointer: %w", err)
	}
	if droppedID != "" && droppedID != pointer.GetPreviousGenerationId() {
		r.deleteCohortGeneration(ctx, input.ProjectionKey, input.ContractID, input.StreamName, droppedIdentity, droppedCutoff, droppedID)
	}
	r.logInfo("Projection snapshot cohort published", input.ProjectionKey, "publish", nil,
		"generation_id", manifestGeneration.GenerationID, "cutoff_seq", input.CutoffSequence,
		"components", len(input.Components), "duration", time.Since(started))
	return LoadedCohort{
		GenerationID: manifestGeneration.GenerationID, CutoffSequence: input.CutoffSequence,
		StreamIdentity: input.StreamIdentity, Components: cloneCohortComponents(input.Components),
		CreatedAt: createdAt, ProducerVersion: r.producerVersion,
	}, nil
}

// LoadCohort loads the current complete cohort or falls back to the previous
// complete cohort when the current manifest or any part is invalid.
func (r *Repository) LoadCohort(ctx context.Context, projectionKey, contractID, streamName, streamIdentity string, maxCutoff uint64) (LoadedCohort, error) {
	return r.LoadCohortForComponents(ctx, projectionKey, contractID, streamName, streamIdentity, maxCutoff, nil)
}

// LoadCohortForComponents validates the exact required component contracts
// and part limits from the manifest before it loads any component payload.
func (r *Repository) LoadCohortForComponents(
	ctx context.Context,
	projectionKey, contractID, streamName, streamIdentity string,
	maxCutoff uint64,
	contracts []CohortComponentContract,
) (LoadedCohort, error) {
	if !validProjectionKey(projectionKey) || !validContractID(contractID) {
		return LoadedCohort{}, fmt.Errorf("snapshot projection key or contract id is invalid")
	}
	if err := validateCohortContracts(contracts, r.maxPayloadSize); err != nil {
		return LoadedCohort{}, err
	}
	pointer, err := r.loadPointer(ctx, projectionKey, contractID)
	if err != nil {
		return LoadedCohort{}, err
	}
	positions := []string{pointer.GetCurrentGenerationId(), pointer.GetPreviousGenerationId()}
	var failures []error
	for index, generationID := range positions {
		if generationID == "" {
			continue
		}
		loaded, err := r.loadCohortGeneration(ctx, generationID, projectionKey, contractID, streamName, streamIdentity, maxCutoff, contracts)
		if err == nil {
			r.logInfo("Projection snapshot cohort loaded", projectionKey, "restore", nil,
				"generation_id", generationID, "cutoff_seq", loaded.CutoffSequence,
				"pointer_slot", index, "components", len(loaded.Components))
			return loaded, nil
		}
		failures = append(failures, fmt.Errorf("generation %s: %w", generationID, err))
		r.logWarn("Projection snapshot cohort rejected", projectionKey, "generation_read", err,
			"generation_id", generationID, "pointer_slot", index)
	}
	if len(failures) == 0 {
		return LoadedCohort{}, ErrSnapshotNotFound
	}
	return LoadedCohort{}, errors.Join(failures...)
}

func (r *Repository) loadCohortGeneration(
	ctx context.Context,
	id, projectionKey, contractID, streamName, streamIdentity string,
	maxCutoff uint64,
	contracts []CohortComponentContract,
) (LoadedCohort, error) {
	manifestGeneration, err := r.loadGeneration(ctx, id, projectionKey, contractID, streamName, streamIdentity, maxCutoff)
	if err != nil {
		return LoadedCohort{}, err
	}
	var manifest projectionv1.ProjectionSnapshotCohortManifest
	if err := proto.Unmarshal(manifestGeneration.Payload, &manifest); err != nil {
		return LoadedCohort{}, fmt.Errorf("unmarshal snapshot cohort manifest: %w", err)
	}
	if len(manifest.GetComponents()) == 0 || len(manifest.GetComponents()) > maxCohortComponents {
		return LoadedCohort{}, fmt.Errorf("snapshot cohort component count is invalid")
	}
	maxTotalPayload := int64(maxCohortPayloadSize)
	expected := make(map[string]CohortComponentContract, len(contracts))
	if len(contracts) > 0 {
		if len(manifest.GetComponents()) != len(contracts) {
			return LoadedCohort{}, fmt.Errorf("snapshot cohort component count is %d, want %d", len(manifest.GetComponents()), len(contracts))
		}
		maxTotalPayload = 0
		for _, contract := range contracts {
			expected[contract.Key] = contract
			componentLimit, ok := checkedPayloadLimit(contract.MaxParts, r.maxPayloadSize)
			if !ok || componentLimit > int64(maxCohortPayloadSize)-maxTotalPayload {
				return LoadedCohort{}, fmt.Errorf("snapshot cohort registered payload limit exceeds %d bytes", maxCohortPayloadSize)
			}
			maxTotalPayload += componentLimit
		}
	}
	components := make([]CohortComponent, 0, len(manifest.GetComponents()))
	seen := make(map[string]struct{}, len(manifest.GetComponents()))
	for _, component := range manifest.GetComponents() {
		if !validProjectionKey(component.GetComponentKey()) || !validContractID(component.GetContractId()) {
			return LoadedCohort{}, fmt.Errorf("snapshot cohort component identity is invalid")
		}
		if _, ok := seen[component.GetComponentKey()]; ok {
			return LoadedCohort{}, fmt.Errorf("snapshot cohort component %q is duplicated", component.GetComponentKey())
		}
		seen[component.GetComponentKey()] = struct{}{}
		maxParts := maxPartsPerComponent
		if len(expected) > 0 {
			contract, ok := expected[component.GetComponentKey()]
			if !ok || contract.ContractID != component.GetContractId() {
				return LoadedCohort{}, fmt.Errorf("snapshot cohort component %q is not registered", component.GetComponentKey())
			}
			maxParts = contract.MaxParts
		}
		if len(component.GetParts()) == 0 || len(component.GetParts()) > maxParts {
			return LoadedCohort{}, fmt.Errorf("snapshot cohort component %q part count is invalid", component.GetComponentKey())
		}
		seenParts := make(map[string]struct{}, len(component.GetParts()))
		for _, manifestPart := range component.GetParts() {
			if manifestPart == nil || !validProjectionKey(manifestPart.GetPartKey()) || manifestPart.GetGenerationId() == "" {
				return LoadedCohort{}, fmt.Errorf("snapshot cohort component %q part identity is invalid", component.GetComponentKey())
			}
			if _, ok := seenParts[manifestPart.GetPartKey()]; ok {
				return LoadedCohort{}, fmt.Errorf("snapshot cohort component %q part %q is duplicated", component.GetComponentKey(), manifestPart.GetPartKey())
			}
			seenParts[manifestPart.GetPartKey()] = struct{}{}
		}
		components = append(components, CohortComponent{Key: component.GetComponentKey(), ContractID: component.GetContractId()})
	}

	// The complete manifest is valid before any component object is read. This
	// prevents malformed later entries from causing unnecessary allocations or
	// object-store reads.
	totalPayloadBytes := int64(0)
	for componentIndex, component := range manifest.GetComponents() {
		partProjectionKey := cohortPartProjectionKey(projectionKey, component.GetComponentKey())
		for _, manifestPart := range component.GetParts() {
			part, err := r.loadGeneration(ctx, manifestPart.GetGenerationId(), partProjectionKey, component.GetContractId(), streamName, streamIdentity, manifestGeneration.CutoffSequence)
			if err != nil {
				return LoadedCohort{}, fmt.Errorf("load component %q part %q: %w", component.GetComponentKey(), manifestPart.GetPartKey(), err)
			}
			if part.CutoffSequence != manifestGeneration.CutoffSequence {
				return LoadedCohort{}, fmt.Errorf("component %q cutoff does not match cohort", component.GetComponentKey())
			}
			if int64(len(part.Payload)) > maxTotalPayload-totalPayloadBytes {
				return LoadedCohort{}, fmt.Errorf("snapshot cohort payload exceeds %d bytes", maxTotalPayload)
			}
			totalPayloadBytes += int64(len(part.Payload))
			components[componentIndex].Parts = append(components[componentIndex].Parts, CohortPart{
				Key: manifestPart.GetPartKey(), Payload: bytes.Clone(part.Payload),
			})
		}
	}
	return LoadedCohort{
		GenerationID: manifestGeneration.GenerationID, CutoffSequence: manifestGeneration.CutoffSequence,
		StreamIdentity: manifestGeneration.StreamIdentity, Components: components,
		CreatedAt: manifestGeneration.CreatedAt, ProducerVersion: manifestGeneration.ProducerVersion,
	}, nil
}

func (r *Repository) writeUnpublishedGeneration(ctx context.Context, input SaveInput, createdAt time.Time) (LoadedSnapshot, string, int, error) {
	if !validProjectionKey(input.ProjectionKey) || !validContractID(input.ContractID) || input.StreamName == "" {
		return LoadedSnapshot{}, "", 0, fmt.Errorf("snapshot projection key, contract id, and stream name are required")
	}
	if input.StreamIdentity == "" {
		return LoadedSnapshot{}, "", 0, fmt.Errorf("snapshot stream identity is required")
	}
	if len(input.Payload) > r.maxPayloadSize {
		return LoadedSnapshot{}, "", 0, fmt.Errorf("snapshot payload exceeds %d bytes", r.maxPayloadSize)
	}
	var generationID [generationIDSize]byte
	if _, err := io.ReadFull(r.rand, generationID[:]); err != nil {
		return LoadedSnapshot{}, "", 0, fmt.Errorf("generate snapshot id: %w", err)
	}
	id := generationIDString(generationID)
	payloadHash := sha256.Sum256(input.Payload)
	generation := &projectionv1.ProjectionSnapshotGeneration{
		GenerationId: id, StreamName: input.StreamName, CutoffSequence: input.CutoffSequence,
		ProjectionKey: input.ProjectionKey, CompatibilityId: input.ContractID,
		ProducerVersion: r.producerVersion, CreatedAt: timestamppb.New(createdAt),
		Payload: input.Payload, PayloadSize: uint64(len(input.Payload)), PayloadSha256: payloadHash[:],
		StreamIdentity: input.StreamIdentity,
	}
	plain, err := proto.MarshalOptions{Deterministic: true}.Marshal(generation)
	if err != nil {
		return LoadedSnapshot{}, "", 0, fmt.Errorf("marshal snapshot generation: %w", err)
	}
	compressed, err := compress(plain)
	if err != nil {
		return LoadedSnapshot{}, "", 0, err
	}
	sealed, err := r.codec.seal(generationID, compressed)
	if err != nil {
		return LoadedSnapshot{}, "", 0, fmt.Errorf("encrypt snapshot generation: %w", err)
	}
	if len(sealed) > maxEncryptedSize {
		return LoadedSnapshot{}, "", 0, fmt.Errorf("encrypted snapshot exceeds %d bytes", maxEncryptedSize)
	}
	objectKey := r.generationObjectKey(input.ProjectionKey, input.ContractID, id)
	if err := r.blobs.Put(ctx, objectKey, sealed, contentType); err != nil {
		return LoadedSnapshot{}, "", 0, fmt.Errorf("write snapshot generation: %w", err)
	}
	return LoadedSnapshot{
		GenerationID: id, CutoffSequence: input.CutoffSequence, StreamIdentity: input.StreamIdentity,
		Payload: bytes.Clone(input.Payload), CreatedAt: createdAt, ProducerVersion: r.producerVersion,
	}, objectKey, len(sealed), nil
}

func (r *Repository) deleteCohortGeneration(ctx context.Context, projectionKey, contractID, streamName, streamIdentity string, cutoff uint64, id string) {
	manifestGeneration, err := r.loadGeneration(ctx, id, projectionKey, contractID, streamName, streamIdentity, cutoff)
	if err != nil {
		return
	}
	var manifest projectionv1.ProjectionSnapshotCohortManifest
	if err := proto.Unmarshal(manifestGeneration.Payload, &manifest); err != nil {
		return
	}
	for _, component := range manifest.GetComponents() {
		partProjectionKey := cohortPartProjectionKey(projectionKey, component.GetComponentKey())
		for _, part := range component.GetParts() {
			if err := r.blobs.Delete(ctx, r.generationObjectKey(partProjectionKey, component.GetContractId(), part.GetGenerationId())); err != nil && !errors.Is(err, ErrBlobNotFound) {
				r.logWarn("Projection snapshot component cleanup failed", projectionKey, "cleanup", err)
			}
		}
	}
	if err := r.blobs.Delete(ctx, r.generationObjectKey(projectionKey, contractID, id)); err != nil && !errors.Is(err, ErrBlobNotFound) {
		r.logWarn("Projection snapshot cohort cleanup failed", projectionKey, "cleanup", err)
	}
}

func validateCohortInput(input SaveCohortInput, partPayloadLimit int) error {
	if !validProjectionKey(input.ProjectionKey) || !validContractID(input.ContractID) || input.StreamName == "" {
		return fmt.Errorf("snapshot projection key, contract id, and stream name are required")
	}
	if input.StreamIdentity == "" {
		return fmt.Errorf("snapshot stream identity is required")
	}
	if len(input.Components) == 0 || len(input.Components) > maxCohortComponents || partPayloadLimit <= 0 {
		return fmt.Errorf("snapshot cohort component count is invalid")
	}
	seen := make(map[string]struct{}, len(input.Components))
	total := int64(0)
	for _, component := range input.Components {
		if !validProjectionKey(component.Key) || !validContractID(component.ContractID) {
			return fmt.Errorf("snapshot cohort component identity is invalid")
		}
		if _, ok := seen[component.Key]; ok {
			return fmt.Errorf("snapshot cohort component %q is duplicated", component.Key)
		}
		seen[component.Key] = struct{}{}
		if len(component.Parts) == 0 || len(component.Parts) > maxPartsPerComponent {
			return fmt.Errorf("snapshot cohort component %q part count is invalid", component.Key)
		}
		seenParts := make(map[string]struct{}, len(component.Parts))
		for _, part := range component.Parts {
			if !validProjectionKey(part.Key) {
				return fmt.Errorf("snapshot cohort component %q part key is invalid", component.Key)
			}
			if _, ok := seenParts[part.Key]; ok {
				return fmt.Errorf("snapshot cohort component %q part %q is duplicated", component.Key, part.Key)
			}
			seenParts[part.Key] = struct{}{}
			if len(part.Payload) > partPayloadLimit {
				return fmt.Errorf("snapshot cohort component %q part exceeds %d bytes", component.Key, partPayloadLimit)
			}
			if int64(len(part.Payload)) > maxCohortPayloadSize-total {
				return fmt.Errorf("snapshot cohort payload exceeds %d bytes", maxCohortPayloadSize)
			}
			total += int64(len(part.Payload))
		}
	}
	return nil
}

func checkedPayloadLimit(parts, partPayloadLimit int) (int64, bool) {
	if parts <= 0 || partPayloadLimit <= 0 {
		return 0, false
	}
	left := int64(parts)
	right := int64(partPayloadLimit)
	if left > int64(maxCohortPayloadSize)/right {
		return 0, false
	}
	return left * right, true
}

func validateCohortContracts(contracts []CohortComponentContract, partPayloadLimit int) error {
	if len(contracts) == 0 {
		return nil
	}
	if len(contracts) > maxCohortComponents || partPayloadLimit <= 0 {
		return fmt.Errorf("snapshot cohort component contracts are invalid")
	}
	seen := make(map[string]struct{}, len(contracts))
	for _, contract := range contracts {
		if !validProjectionKey(contract.Key) || !validContractID(contract.ContractID) ||
			contract.MaxParts <= 0 || contract.MaxParts > maxPartsPerComponent {
			return fmt.Errorf("snapshot cohort component contract is invalid")
		}
		if _, ok := seen[contract.Key]; ok {
			return fmt.Errorf("snapshot cohort component contract %q is duplicated", contract.Key)
		}
		seen[contract.Key] = struct{}{}
	}
	return nil
}

func cohortContractsFromComponents(components []CohortComponent) []CohortComponentContract {
	contracts := make([]CohortComponentContract, 0, len(components))
	for _, component := range components {
		contracts = append(contracts, CohortComponentContract{
			Key: component.Key, ContractID: component.ContractID, MaxParts: len(component.Parts),
		})
	}
	return contracts
}

func cohortPartProjectionKey(projectionKey, componentKey string) string {
	return projectionKey + "_" + componentKey + "_part"
}

func cloneCohortComponents(components []CohortComponent) []CohortComponent {
	cloned := make([]CohortComponent, len(components))
	for i, component := range components {
		cloned[i] = CohortComponent{Key: component.Key, ContractID: component.ContractID}
		for _, part := range component.Parts {
			cloned[i].Parts = append(cloned[i].Parts, CohortPart{Key: part.Key, Payload: bytes.Clone(part.Payload)})
		}
	}
	return cloned
}
