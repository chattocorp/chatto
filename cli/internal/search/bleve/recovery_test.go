package bleve

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	blevesearch "github.com/blevesearch/bleve/v2"
	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"

	"github.com/chattocorp/chatto/cli/internal/encryption"
	"github.com/chattocorp/chatto/pkg/events"
)

// recoveryFixture is an index directory with one indexed message, M1, at
// checkpoint 2.
type recoveryFixture struct {
	directory string
	key       []byte
	request   events.ProjectionCheckpointRequest
}

func newRecoveryFixture(t *testing.T) recoveryFixture {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	f := recoveryFixture{directory: filepath.Join(t.TempDir(), "index"), key: key}
	p := f.open(t)
	f.request = events.ProjectionCheckpointRequest{
		ProjectionKey: "message_search", ContractID: p.CheckpointContractID(),
		StreamName: "EVT", StreamIdentity: "evt-incarnation-v1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		FirstSequence: 1, LastSequence: 10,
	}
	_, err = p.RestoreCheckpoint(context.Background(), f.request)
	require.NoError(t, err)
	applyLegacyMessage(t, p, key, "M1", "B1", "R1", "U1", "kept before recovery", time.Unix(100, 0), 1)
	require.NoError(t, p.Close())
	return f
}

func (f recoveryFixture) open(t *testing.T) *Projection {
	t.Helper()
	p, err := NewProjection(f.directory, []string{}, nil, staticLegacyKeys{key: f.key}, nil, log.New(nil))
	require.NoError(t, err)
	return p
}

func plentyOfSpace(string) (uint64, error) { return 1 << 40, nil }

// requirePreviousIndexHas opens the kept previous index read-only and checks
// that it still has the document.
func requirePreviousIndexHas(t *testing.T, directory, messageID string) {
	t.Helper()
	index, err := blevesearch.OpenUsing(filepath.Join(directory, previousIndexName), map[string]any{"read_only": true})
	require.NoError(t, err)
	defer func() { require.NoError(t, index.Close()) }()
	document, err := index.Document(messageDocumentID(messageID))
	require.NoError(t, err)
	require.NotNil(t, document)
}

func TestRecoverIndexKeepsPreviousAndResumesAfterRestart(t *testing.T) {
	ctx := context.Background()
	f := newRecoveryFixture(t)

	result, err := recoverIndex(f.directory, plentyOfSpace)
	require.NoError(t, err)
	require.False(t, result.InProgress)
	require.NotZero(t, result.IndexBytes)
	require.FileExists(t, filepath.Join(f.directory, recoveryMarkerName))
	requirePreviousIndexHas(t, f.directory, "M1")

	// The provider builds a new index: replay starts at the first sequence.
	p := f.open(t)
	checkpoint, err := p.RestoreCheckpoint(ctx, f.request)
	require.NoError(t, err)
	require.Zero(t, checkpoint.CutoffSequence)
	response, err := p.query(ctx, relevanceRequest([]string{"recovery"}, nil))
	require.NoError(t, err)
	require.Empty(t, response.GetHits())
	applyLegacyMessage(t, p, f.key, "M2", "B2", "R1", "U1", "replayed after recovery", time.Unix(200, 0), 1)

	// A restart in the middle of the replay continues from its checkpoint and
	// keeps the recovery open. Running recover again changes nothing.
	require.NoError(t, p.Close())
	result, err = recoverIndex(f.directory, plentyOfSpace)
	require.NoError(t, err)
	require.True(t, result.InProgress)
	p = f.open(t)
	t.Cleanup(func() { _ = p.Close() })
	checkpoint, err = p.RestoreCheckpoint(ctx, f.request)
	require.NoError(t, err)
	require.Equal(t, uint64(2), checkpoint.CutoffSequence)
	require.FileExists(t, filepath.Join(f.directory, recoveryMarkerName))
	requirePreviousIndexHas(t, f.directory, "M1")

	// After the replay, a real query ends the recovery.
	recovered, err := p.completeStartup(ctx)
	require.NoError(t, err)
	require.True(t, recovered)
	require.NoFileExists(t, filepath.Join(f.directory, recoveryMarkerName))
	completion, err := p.index.GetInternal([]byte(completionInternalKey))
	require.NoError(t, err)
	require.NotEmpty(t, completion)
	requirePreviousIndexHas(t, f.directory, "M1")
	recovered, err = p.completeStartup(ctx)
	require.NoError(t, err)
	require.False(t, recovered)
}

func TestRecoverIndexReplacesOnlyTheEarlierPreviousIndex(t *testing.T) {
	ctx := context.Background()
	f := newRecoveryFixture(t)
	_, err := recoverIndex(f.directory, plentyOfSpace)
	require.NoError(t, err)
	p := f.open(t)
	_, err = p.RestoreCheckpoint(ctx, f.request)
	require.NoError(t, err)
	applyLegacyMessage(t, p, f.key, "M2", "B2", "R1", "U1", "second generation", time.Unix(200, 0), 3)
	_, err = p.completeStartup(ctx)
	require.NoError(t, err)
	require.NoError(t, p.Close())

	result, err := recoverIndex(f.directory, plentyOfSpace)
	require.NoError(t, err)
	require.NotZero(t, result.DiscardedPreviousBytes)
	requirePreviousIndexHas(t, f.directory, "M2")
}

func TestRecoverIndexRefusesWithoutDiskSpace(t *testing.T) {
	f := newRecoveryFixture(t)
	before, err := pathSize(f.directory)
	require.NoError(t, err)
	_, err = recoverIndex(f.directory, func(string) (uint64, error) { return 1, nil })
	require.ErrorIs(t, err, ErrInsufficientDiskSpace)
	require.ErrorContains(t, err, "add space to the volume")
	require.NoFileExists(t, filepath.Join(f.directory, recoveryMarkerName))
	require.NoDirExists(t, filepath.Join(f.directory, previousIndexName))
	after, err := pathSize(f.directory)
	require.NoError(t, err)
	require.Equal(t, before, after)
	p := f.open(t)
	t.Cleanup(func() { _ = p.Close() })
	checkpoint, err := p.RestoreCheckpoint(context.Background(), f.request)
	require.NoError(t, err)
	require.Equal(t, uint64(2), checkpoint.CutoffSequence)
}

func TestRecoverIndexRefusesWhileTheProviderRuns(t *testing.T) {
	f := newRecoveryFixture(t)
	p := f.open(t)
	t.Cleanup(func() { _ = p.Close() })
	_, err := recoverIndex(f.directory, plentyOfSpace)
	require.ErrorContains(t, err, "stop any other provider")
	_, err = DiscardPreviousIndex(f.directory)
	require.NoError(t, err, "no previous index exists, so there is nothing to lock")
	require.NoFileExists(t, filepath.Join(f.directory, recoveryMarkerName))
}

func TestRecoverIndexRefusesWithoutIndexOrWithUnrelatedEntries(t *testing.T) {
	_, err := recoverIndex(filepath.Join(t.TempDir(), "missing"), plentyOfSpace)
	require.ErrorContains(t, err, "no search index")

	f := newRecoveryFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.directory, "operator-notes"), []byte("x"), 0o600))
	_, err = recoverIndex(f.directory, plentyOfSpace)
	require.ErrorContains(t, err, "unrelated entries")
	require.NoDirExists(t, filepath.Join(f.directory, previousIndexName))
}

func TestProviderStartupFinishesAnInterruptedRecoveryMove(t *testing.T) {
	f := newRecoveryFixture(t)
	// Model a crash after the marker and the first rename.
	require.NoError(t, writeMarker(f.directory, recoveryMarkerName, recoveryMarker))
	require.NoError(t, os.MkdirAll(filepath.Join(f.directory, previousIndexName), 0o700))
	require.NoError(t, os.Rename(filepath.Join(f.directory, "store"), filepath.Join(f.directory, previousIndexName, "store")))

	p := f.open(t)
	t.Cleanup(func() { _ = p.Close() })
	checkpoint, err := p.RestoreCheckpoint(context.Background(), f.request)
	require.NoError(t, err)
	require.Zero(t, checkpoint.CutoffSequence)
	requirePreviousIndexHas(t, f.directory, "M1")
}

func TestDiscardPreviousIndex(t *testing.T) {
	ctx := context.Background()
	f := newRecoveryFixture(t)
	_, err := recoverIndex(f.directory, plentyOfSpace)
	require.NoError(t, err)

	// While the recovery is open, the previous index can be the last complete one.
	_, err = DiscardPreviousIndex(f.directory)
	require.ErrorContains(t, err, "recovery is not complete")
	requirePreviousIndexHas(t, f.directory, "M1")

	p := f.open(t)
	_, err = p.RestoreCheckpoint(ctx, f.request)
	require.NoError(t, err)
	_, err = p.completeStartup(ctx)
	require.NoError(t, err)
	require.NoError(t, p.Close())

	size, err := DiscardPreviousIndex(f.directory)
	require.NoError(t, err)
	require.NotZero(t, size)
	require.NoDirExists(t, filepath.Join(f.directory, previousIndexName))
	size, err = DiscardPreviousIndex(f.directory)
	require.NoError(t, err)
	require.Zero(t, size)
}

func TestCompleteStartupFailureKeepsTheRecoveryOpen(t *testing.T) {
	f := newRecoveryFixture(t)
	_, err := recoverIndex(f.directory, plentyOfSpace)
	require.NoError(t, err)
	p := f.open(t)
	_, err = p.RestoreCheckpoint(context.Background(), f.request)
	require.NoError(t, err)
	applyLegacyMessage(t, p, f.key, "M2", "B2", "R1", "U1", "replayed", time.Unix(200, 0), 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.completeStartup(ctx)
	require.ErrorContains(t, err, "validation query")
	require.FileExists(t, filepath.Join(f.directory, recoveryMarkerName))
	completion, err := p.index.GetInternal([]byte(completionInternalKey))
	require.NoError(t, err)
	require.Empty(t, completion)
	require.NoError(t, p.Close())
	requirePreviousIndexHas(t, f.directory, "M1")
}

func TestRecoverIndexKeepsPreviousWhenTheCurrentIndexNeverCompleted(t *testing.T) {
	f := newRecoveryFixture(t)
	_, err := recoverIndex(f.directory, plentyOfSpace)
	require.NoError(t, err)
	// Model an incomplete current index: the recovery marker is gone, but no
	// validated startup recorded the completion marker.
	p := f.open(t)
	_, err = p.RestoreCheckpoint(context.Background(), f.request)
	require.NoError(t, err)
	require.NoError(t, p.Close())
	require.NoError(t, os.Remove(filepath.Join(f.directory, recoveryMarkerName)))

	_, err = recoverIndex(f.directory, plentyOfSpace)
	require.ErrorContains(t, err, "never completed a validated startup")
	requirePreviousIndexHas(t, f.directory, "M1")
	require.NoFileExists(t, filepath.Join(f.directory, recoveryMarkerName))
}
