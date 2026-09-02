// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package projectionsnapshot

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	projectionv1 "hmans.de/chatto/internal/pb/chatto/core/projection/v1"
)

func testCohortInput(sequence uint64, suffix string) SaveCohortInput {
	return SaveCohortInput{
		ProjectionKey: "server_content_view", ContractID: "view-v1", StreamName: "EVT",
		StreamIdentity: testStreamIdentity, CutoffSequence: sequence,
		Components: []CohortComponent{
			{Key: "rooms", ContractID: "rooms-v1", Parts: []CohortPart{{Key: "state", Payload: []byte("rooms-" + suffix)}}},
			{Key: "messages", ContractID: "messages-v1", Parts: []CohortPart{{Key: "state", Payload: []byte("messages-" + suffix)}}},
		},
	}
}

func TestRepositoryCohortRoundTripStoresComponentsIndividually(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	input := testCohortInput(42, "current")
	saved, err := repository.SaveCohort(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.LoadCohort(t.Context(), input.ProjectionKey, input.ContractID, input.StreamName, input.StreamIdentity, 42)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != saved.GenerationID || loaded.CutoffSequence != 42 || len(loaded.Components) != 2 {
		t.Fatalf("loaded cohort = %#v", loaded)
	}
	if loaded.Components[0].Parts[0].Key != "state" ||
		!bytes.Equal(loaded.Components[0].Parts[0].Payload, input.Components[0].Parts[0].Payload) ||
		!bytes.Equal(loaded.Components[1].Parts[0].Payload, input.Components[1].Parts[0].Payload) {
		t.Fatalf("loaded component payloads = %#v", loaded.Components)
	}
	if len(blobs.objects) != 3 {
		t.Fatalf("stored objects = %d, want two component parts and one manifest", len(blobs.objects))
	}
	for key, data := range blobs.objects {
		if bytes.Contains(data, []byte("rooms-current")) || bytes.Contains(data, []byte("messages-current")) {
			t.Fatalf("snapshot payload leaked through encrypted object %q", key)
		}
	}
}

func TestRepositoryCohortPreservesStablePartKeys(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	input := testCohortInput(42, "bucketed")
	input.Components[1].Parts = []CohortPart{
		{Key: "month_2026_08", Payload: []byte("august")},
		{Key: "month_2026_09", Payload: []byte("september")},
	}
	if _, err := repository.SaveCohort(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.LoadCohort(t.Context(), input.ProjectionKey, input.ContractID, input.StreamName, input.StreamIdentity, 42)
	if err != nil {
		t.Fatal(err)
	}
	parts := loaded.Components[1].Parts
	if len(parts) != 2 || parts[0].Key != "month_2026_08" || parts[1].Key != "month_2026_09" {
		t.Fatalf("loaded stable parts = %#v", parts)
	}
}

func TestRepositoryCohortRejectsDuplicatePartKeys(t *testing.T) {
	repository := newTestRepository(t, newMemoryBlobStore(), testSecret)
	input := testCohortInput(42, "duplicate")
	input.Components[0].Parts = append(input.Components[0].Parts, input.Components[0].Parts[0])
	if _, err := repository.SaveCohort(t.Context(), input); err == nil {
		t.Fatal("SaveCohort accepted duplicate component part keys")
	}
}

func TestRepositoryCohortUsesConfiguredPartPayloadLimit(t *testing.T) {
	repository := newTestRepository(t, newMemoryBlobStore(), testSecret)
	repository.maxPayloadSize = 4
	input := testCohortInput(42, "oversized")
	if _, err := repository.SaveCohort(t.Context(), input); err == nil {
		t.Fatal("SaveCohort accepted a component part above the repository limit")
	}
}

func TestRepositoryCohortFallsBackWhenCurrentPartIsMissing(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	if _, err := repository.SaveCohort(t.Context(), testCohortInput(10, "previous")); err != nil {
		t.Fatal(err)
	}
	current, err := repository.SaveCohort(t.Context(), testCohortInput(20, "current"))
	if err != nil {
		t.Fatal(err)
	}
	manifestGeneration, err := repository.loadGeneration(
		t.Context(), current.GenerationID, "server_content_view", "view-v1", "EVT", testStreamIdentity, 20,
	)
	if err != nil {
		t.Fatal(err)
	}
	var manifest projectionv1.ProjectionSnapshotCohortManifest
	if err := proto.Unmarshal(manifestGeneration.Payload, &manifest); err != nil {
		t.Fatal(err)
	}
	part := manifest.GetComponents()[0]
	delete(blobs.objects, repository.generationObjectKey(
		cohortPartProjectionKey("server_content_view", part.GetComponentKey()),
		part.GetContractId(), part.GetParts()[0].GetGenerationId(),
	))

	loaded, err := repository.LoadCohort(t.Context(), "server_content_view", "view-v1", "EVT", testStreamIdentity, 20)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CutoffSequence != 10 || string(loaded.Components[0].Parts[0].Payload) != "rooms-previous" {
		t.Fatalf("fallback cohort = %#v", loaded)
	}
}

func TestRepositoryCohortRejectsUnexpectedComponentBeforeLoadingParts(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	input := testCohortInput(42, "current")
	if _, err := repository.SaveCohort(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	objectReads := 0
	blobs.failGet = func(key string) bool {
		if strings.HasPrefix(key, objectRootPrefix) {
			objectReads++
		}
		return false
	}
	_, err := repository.LoadCohortForComponents(
		t.Context(), input.ProjectionKey, input.ContractID, input.StreamName,
		input.StreamIdentity, input.CutoffSequence,
		[]CohortComponentContract{{Key: "rooms", ContractID: "rooms-v1", MaxParts: 1}},
	)
	if err == nil {
		t.Fatal("LoadCohortForComponents accepted an unexpected component")
	}
	if objectReads != 1 {
		t.Fatalf("snapshot object reads = %d, want manifest only", objectReads)
	}
}

func TestRepositoryCohortValidatesAllPartKeysBeforeLoadingParts(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	manifest := &projectionv1.ProjectionSnapshotCohortManifest{Components: []*projectionv1.ProjectionSnapshotCohortComponent{
		{ComponentKey: "rooms", ContractId: "rooms-v1", Parts: []*projectionv1.ProjectionSnapshotCohortPart{
			{PartKey: "state", GenerationId: "rooms-generation"},
		}},
		{ComponentKey: "messages", ContractId: "messages-v1", Parts: []*projectionv1.ProjectionSnapshotCohortPart{
			{PartKey: "same", GenerationId: "messages-one"},
			{PartKey: "same", GenerationId: "messages-two"},
		}},
	}}
	payload, err := proto.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	generation, _, _, err := repository.writeUnpublishedGeneration(t.Context(), SaveInput{
		ProjectionKey: "server_content_view", ContractID: "view-v1", StreamName: "EVT",
		StreamIdentity: testStreamIdentity, CutoffSequence: 42, Payload: payload,
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	objectReads := 0
	blobs.failGet = func(key string) bool {
		if strings.HasPrefix(key, objectRootPrefix) {
			objectReads++
		}
		return false
	}
	_, err = repository.loadCohortGeneration(
		t.Context(), generation.GenerationID, "server_content_view", "view-v1",
		"EVT", testStreamIdentity, 42, nil,
	)
	if err == nil {
		t.Fatal("loadCohortGeneration accepted duplicate part keys")
	}
	if objectReads != 1 {
		t.Fatalf("snapshot object reads = %d, want manifest only", objectReads)
	}
}

func TestRepositoryCohortCleansUnpublishedParts(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	objectPuts := 0
	blobs.failPut = func(key string) bool {
		if !strings.HasPrefix(key, objectRootPrefix) {
			return false
		}
		objectPuts++
		return objectPuts == 2
	}
	if _, err := repository.SaveCohort(t.Context(), testCohortInput(10, "failed")); err == nil {
		t.Fatal("SaveCohort succeeded despite an injected part failure")
	}
	if len(blobs.objects) != 0 {
		t.Fatalf("unpublished objects retained = %d, want 0", len(blobs.objects))
	}
}

func TestRepositoryCohortRejectsCutoffRegression(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	if _, err := repository.SaveCohort(t.Context(), testCohortInput(20, "newer")); err != nil {
		t.Fatal(err)
	}
	_, err := repository.SaveCohort(t.Context(), testCohortInput(19, "older"))
	if !errors.Is(err, ErrSnapshotRegressed) {
		t.Fatalf("SaveCohort error = %v, want ErrSnapshotRegressed", err)
	}
}

func TestRepositoryCohortRejectsStalePointerPublication(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	if _, err := repository.SaveCohort(t.Context(), testCohortInput(1, "first")); err != nil {
		t.Fatal(err)
	}
	var newest LoadedCohort
	blobs.beforePointerUpdate = func(_ string, _ uint64) {
		blobs.beforePointerUpdate = nil
		if _, err := repository.SaveCohort(t.Context(), testCohortInput(2, "second")); err != nil {
			t.Fatal(err)
		}
		var err error
		newest, err = repository.SaveCohort(t.Context(), testCohortInput(3, "third"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repository.SaveCohort(t.Context(), testCohortInput(4, "stale")); !errors.Is(err, ErrPointerConflict) {
		t.Fatalf("stale SaveCohort error = %v, want ErrPointerConflict", err)
	}
	loaded, err := repository.LoadCohort(t.Context(), "server_content_view", "view-v1", "EVT", testStreamIdentity, 4)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GenerationID != newest.GenerationID || loaded.CutoffSequence != 3 {
		t.Fatalf("stale writer regressed cohort pointer: loaded=%#v newest=%#v", loaded, newest)
	}
	if len(blobs.objects) != 6 {
		t.Fatalf("stored cohort objects = %d, want current and previous component sets", len(blobs.objects))
	}
}

func TestRepositoryCohortSkipsFreshSameCutoff(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	input := testCohortInput(20, "same")
	input.RefreshAge = time.Hour
	if _, err := repository.SaveCohort(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SaveCohort(context.Background(), input); !errors.Is(err, ErrSnapshotFresh) {
		t.Fatalf("SaveCohort error = %v, want ErrSnapshotFresh", err)
	}
}

func TestRepositoryCohortRepairsInvalidFreshGeneration(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	input := testCohortInput(20, "same")
	input.RefreshAge = time.Hour
	saved, err := repository.SaveCohort(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	manifestGeneration, err := repository.loadGeneration(
		t.Context(), saved.GenerationID, input.ProjectionKey, input.ContractID,
		input.StreamName, input.StreamIdentity, input.CutoffSequence,
	)
	if err != nil {
		t.Fatal(err)
	}
	var manifest projectionv1.ProjectionSnapshotCohortManifest
	if err := proto.Unmarshal(manifestGeneration.Payload, &manifest); err != nil {
		t.Fatal(err)
	}
	component := manifest.GetComponents()[0]
	delete(blobs.objects, repository.generationObjectKey(
		cohortPartProjectionKey(input.ProjectionKey, component.GetComponentKey()),
		component.GetContractId(), component.GetParts()[0].GetGenerationId(),
	))

	repaired, err := repository.SaveCohort(t.Context(), input)
	if err != nil {
		t.Fatalf("SaveCohort repair error = %v", err)
	}
	if repaired.GenerationID == saved.GenerationID {
		t.Fatalf("SaveCohort reused invalid generation %q", saved.GenerationID)
	}
}
