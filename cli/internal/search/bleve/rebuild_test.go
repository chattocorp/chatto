package bleve

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/encryption"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	"hmans.de/chatto/pkg/events"
)

func TestProjectionRebuildRetainsIntentUntilReplacementIsDurable(t *testing.T) {
	for _, failedEntry := range []string{"index_meta.json", "store"} {
		t.Run(failedEntry, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "index")
			p, err := NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
			require.NoError(t, err)
			t.Cleanup(func() { _ = p.Close() })
			request := events.ProjectionCheckpointRequest{ProjectionKey: "message_search", ContractID: p.contractID, StreamName: "EVT", StreamIdentity: "same", FirstSequence: 1, LastSequence: 1}
			failure := errors.New("injected sync failure")
			p.syncPathOverride = func(path string) error {
				if filepath.Base(path) == failedEntry {
					return failure
				}
				entry, err := os.Open(path)
				if err != nil {
					return err
				}
				return errors.Join(entry.Sync(), entry.Close())
			}
			require.ErrorIs(t, p.rebuildIndex(request), failure)
			marker, err := os.ReadFile(filepath.Join(directory, rebuildMarkerName))
			require.NoError(t, err)
			require.Equal(t, rebuildMarker, string(marker))
			require.NoError(t, p.Close())
			// Model lost metadata after an unsuccessful sync. The retained marker
			// must repair it automatically, without operator intervention.
			require.NoError(t, os.WriteFile(filepath.Join(directory, "index_meta.json"), []byte("partial"), 0o600))
			p, err = NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
			require.NoError(t, err)
			_, err = os.Stat(filepath.Join(directory, rebuildMarkerName))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestProjectionRebuildsKnownContractChanges(t *testing.T) {
	for _, base := range []string{"bleve-message-index-v10", checkpointContractBaseID} {
		t.Run(base, func(t *testing.T) {
			ctx := context.Background()
			key, err := encryption.GenerateKey()
			require.NoError(t, err)
			directory := filepath.Join(t.TempDir(), "index")
			old, err := NewProjection(directory, []string{"en"}, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
			require.NoError(t, err)
			request := events.ProjectionCheckpointRequest{
				ProjectionKey: "message_search", ContractID: languageCheckpointContractIDForBase(base, old.languages),
				StreamName: "EVT", StreamIdentity: "original-stream", FirstSequence: 1, LastSequence: 2,
			}
			_, err = old.RestoreCheckpoint(ctx, request)
			require.NoError(t, err)
			applyLegacyMessage(t, old, key, "OLD", "B1", "R1", "U1", "obsolete", time.Unix(1, 0), 1)
			require.NoError(t, old.Close())

			current, err := NewProjection(directory, []string{}, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, current.Close()) })
			request.ContractID = current.CheckpointContractID()
			_, err = current.RestoreCheckpoint(ctx, request)
			require.ErrorIs(t, err, events.ErrProjectionCheckpointInvalid)
			require.NoError(t, current.ResetCheckpoint(ctx, request))
			response, err := current.query(ctx, relevanceRequest([]string{"obsolete"}, nil))
			require.NoError(t, err)
			require.Empty(t, response.Hits)
			// Replay starts at sequence one; old documents and cutoffs cannot survive.
			applyLegacyMessage(t, current, key, "ROOT", "B2", "R1", "U1", "rebuilt", time.Unix(2, 0), 1)
			response, err = current.query(ctx, &searchv1.QueryRequest{
				RequiredTerms: []string{"rebuilt"}, ThreadRootIds: []string{"ROOT"}, PageSize: 10,
				Order: searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE,
			})
			require.NoError(t, err)
			require.Equal(t, []string{"ROOT"}, hitIDs(response))
			require.NoError(t, current.Close())
			current, err = NewProjection(directory, []string{}, nil, staticLegacyKeys{key: key}, nil, log.New(nil))
			require.NoError(t, err)
			checkpoint, err := current.RestoreCheckpoint(ctx, request)
			require.NoError(t, err)
			require.EqualValues(t, 2, checkpoint.CutoffSequence)
		})
	}
}

func TestProjectionDoesNotRebuildUntrustedOrUnrelatedState(t *testing.T) {
	for _, mode := range []string{"future-format", "stream-changed", "future-cutoff", "retention-gap", "corrupt-checkpoint", "unrelated-file", "unrelated-symlink"} {
		t.Run(mode, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "index")
			p, err := NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
			require.NoError(t, err)
			t.Cleanup(func() { _ = p.Close() })
			request := events.ProjectionCheckpointRequest{ProjectionKey: "message_search", ContractID: p.contractID, StreamName: "EVT", StreamIdentity: "same", FirstSequence: 1, LastSequence: 10}
			record := checkpointFromRequest(request)
			record.ContractID = languageCheckpointContractIDForBase("bleve-message-index-v10", p.languages)
			record.CutoffSequence = 5
			switch mode {
			case "future-format":
				record.ContractID = "bleve-message-index-v999-0123456789abcdef"
			case "stream-changed":
				record.StreamIdentity = "different"
			case "future-cutoff":
				record.CutoffSequence = 11
			case "retention-gap":
				request.FirstSequence = 7
			case "unrelated-file":
				require.NoError(t, os.WriteFile(filepath.Join(directory, "operator-data"), []byte("keep"), 0o600))
			case "unrelated-symlink":
				// A link beside the real index must block the destructive path.
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(directory, "foreign-link")))
			}
			data, err := json.Marshal(record)
			require.NoError(t, err)
			if mode == "corrupt-checkpoint" {
				data = []byte("invalid")
			}
			require.NoError(t, p.index.SetInternal([]byte(checkpointInternalKey), data))
			require.Error(t, p.ResetCheckpoint(context.Background(), request))
			retained, err := p.index.GetInternal([]byte(checkpointInternalKey))
			require.NoError(t, err)
			require.Equal(t, data, retained)
		})
	}
}

func TestProjectionResumesInterruptedRebuild(t *testing.T) {
	for _, phase := range []string{"before-delete", "metadata-removed", "store-removed"} {
		t.Run(phase, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "index")
			p, err := NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
			require.NoError(t, err)
			require.NoError(t, p.index.SetInternal([]byte("old-sentinel"), []byte("old")))
			require.NoError(t, p.Close())
			require.NoError(t, os.WriteFile(filepath.Join(directory, rebuildMarkerName), []byte(rebuildMarker), 0o600))
			if phase == "metadata-removed" || phase == "store-removed" {
				require.NoError(t, os.Remove(filepath.Join(directory, "index_meta.json")))
			}
			if phase == "store-removed" {
				require.NoError(t, os.RemoveAll(filepath.Join(directory, "store")))
			}
			p, err = NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, p.Close()) })
			data, err := p.index.GetInternal([]byte("old-sentinel"))
			require.NoError(t, err)
			require.Empty(t, data)
			_, err = os.Stat(filepath.Join(directory, rebuildMarkerName))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestProjectionDirectoryLockExcludesSecondProvider(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "index")
	first, err := NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	_, err = NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
	require.ErrorContains(t, err, "stop any other provider")
	require.NoError(t, first.Close())
	next, err := NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
	require.NoError(t, err)
	require.NoError(t, next.Close())
}

func TestProjectionRecoveryPreservesUnknownState(t *testing.T) {
	for _, mode := range []string{"invalid-marker", "unrelated-entry", "linked-store"} {
		t.Run(mode, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "index")
			p, err := NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
			require.NoError(t, err)
			require.NoError(t, p.Close())
			metadataPath := filepath.Join(directory, "index_meta.json")
			metadata, err := os.ReadFile(metadataPath)
			require.NoError(t, err)
			marker := rebuildMarker
			switch mode {
			case "invalid-marker":
				marker = "incomplete"
			case "unrelated-entry":
				require.NoError(t, os.WriteFile(filepath.Join(directory, "keep"), []byte("operator data"), 0o600))
			case "linked-store":
				store := filepath.Join(directory, "store")
				moved := filepath.Join(t.TempDir(), "store")
				require.NoError(t, os.Rename(store, moved))
				require.NoError(t, os.Symlink(moved, store))
			}
			require.NoError(t, os.WriteFile(filepath.Join(directory, rebuildMarkerName), []byte(marker), 0o600))
			_, err = NewProjection(directory, []string{}, nil, nil, nil, log.New(nil))
			require.Error(t, err)
			retained, err := os.ReadFile(metadataPath)
			require.NoError(t, err)
			require.Equal(t, metadata, retained)
		})
	}
}
