package bleve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	blevesearch "github.com/blevesearch/bleve/v2"
	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
)

// Index recovery (FDR-033, FDR-057) rebuilds the search index from EVT and
// keeps the previous index for diagnosis. The operator starts it offline with
// RecoverIndex while no provider uses the directory. The provider then builds
// a fresh root index, resumes it from its own checkpoint after a restart, and
// ends the recovery only after a real query succeeds (completeStartup).
//
// Directory layout during and after a recovery:
//
//	<directory>/index_meta.json, store/  the index that the provider uses
//	<directory>/previous/                 the one retained previous index
//	<directory>/.chatto-search-recovery   present until the recovery completes
//
// Nothing deletes the previous index automatically. The next RecoverIndex
// replaces it, and DiscardPreviousIndex removes it.
const (
	previousIndexName  = "previous"
	recoveryMarkerName = ".chatto-search-recovery"
	recoveryMarker     = "chatto-search-recovery-v1\n"
	// completionInternalKey holds the checkpoint at which the provider last
	// proved, with a real query, that the index completed its startup replay.
	// FDR-057 uses it to identify a usable serving index.
	completionInternalKey = "chatto/search/complete"
)

// indexEntries are the root entries of a Bleve index. Recovery moves store
// first and index_meta.json last, so a present previous/index_meta.json
// proves a complete move.
var indexEntries = []string{"store", "index_meta.json"}

// ErrInsufficientDiskSpace reports that the index volume cannot hold the
// previous index and a new index with its merge reserve.
var ErrInsufficientDiskSpace = errors.New("insufficient free disk space for search index recovery")

// RecoveryResult describes what RecoverIndex did.
type RecoveryResult struct {
	// InProgress is true when an earlier recovery has not completed. Then
	// RecoverIndex changes nothing, and the next provider start resumes the
	// earlier recovery from its checkpoint.
	InProgress bool
	// IndexBytes is the size of the index that became the previous index.
	IndexBytes uint64
	// DiscardedPreviousBytes is the size of the earlier previous index that
	// this recovery deleted.
	DiscardedPreviousBytes uint64
}

// RecoverIndex prepares a recovery of the index in directory. It keeps the
// current index as the previous index; the next provider start builds a new
// index from EVT. It needs the directory lock, so every provider that uses
// the directory must be stopped. It refuses when the volume does not have
// free space for 1.5 times the current index (FDR-057, decision 7).
func RecoverIndex(directory string) (RecoveryResult, error) {
	return recoverIndex(directory, availableDiskSpace)
}

func recoverIndex(directory string, freeSpace func(string) (uint64, error)) (RecoveryResult, error) {
	directory, err := cleanIndexDirectory(directory)
	if err != nil {
		return RecoveryResult{}, err
	}
	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		return RecoveryResult{}, fmt.Errorf("no search index at %q; start the provider to build one", directory)
	}
	lock, err := lockSearchDirectory(directory)
	if err != nil {
		return RecoveryResult{}, err
	}
	defer lock.Close()
	if err := validateIndexDirectory(directory); err != nil {
		return RecoveryResult{}, err
	}
	if exists(filepath.Join(directory, rebuildMarkerName)) {
		return RecoveryResult{}, fmt.Errorf("an automatic search index rebuild is not complete; start the provider, then try again")
	}
	if exists(filepath.Join(directory, recoveryMarkerName)) {
		if err := finishRecoveryMove(directory); err != nil {
			return RecoveryResult{}, err
		}
		return RecoveryResult{InProgress: true}, nil
	}
	if !exists(filepath.Join(directory, "index_meta.json")) {
		if exists(filepath.Join(directory, "store")) {
			// Without index_meta.json, a later start cannot tell a moved
			// index from a new one. Leave this rare case to the operator.
			return RecoveryResult{}, fmt.Errorf("the search index in %q has no index_meta.json; stop the provider, move the directory %q aside, and start the provider to build a new index", directory, filepath.Join(directory, "store"))
		}
		return RecoveryResult{}, fmt.Errorf("no search index at %q; start the provider to build one", directory)
	}

	previous := filepath.Join(directory, previousIndexName)
	if exists(previous) && !indexCompleted(directory) {
		// The current index never passed a validated startup, for example
		// after an interrupted automatic rebuild. Then the previous index
		// can be the last complete one; do not replace it implicitly.
		return RecoveryResult{}, fmt.Errorf("the current search index in %q never completed a validated startup, so the previous index can be the last complete one; start the provider and wait until Search is ready, or delete the previous index with chatto search-provider discard-previous, then try again", directory)
	}

	var indexBytes uint64
	for _, name := range indexEntries {
		size, err := pathSize(filepath.Join(directory, name))
		if err != nil {
			return RecoveryResult{}, err
		}
		indexBytes += size
	}
	previousBytes, err := pathSize(previous)
	if err != nil {
		return RecoveryResult{}, err
	}
	free, err := freeSpace(directory)
	if err != nil {
		return RecoveryResult{}, fmt.Errorf("check free disk space: %w", err)
	}
	// The new index needs about the size of the current one, plus half of it
	// for Bleve segment merges. The earlier previous index is deleted first.
	required := indexBytes + indexBytes/2
	if free+previousBytes < required {
		return RecoveryResult{}, fmt.Errorf("%w: need %s, have %s (free %s plus %s from the earlier previous index); add space to the volume of %q and try again",
			ErrInsufficientDiskSpace, formatBytes(required), formatBytes(free+previousBytes), formatBytes(free), formatBytes(previousBytes), directory)
	}

	// Order matters for a crash at any step. Remove the earlier previous
	// index before the marker exists, so that with a marker, previous/ holds
	// only entries from this move. Then persist intent, then move.
	if err := os.RemoveAll(previous); err != nil {
		return RecoveryResult{}, err
	}
	if err := syncPath(directory); err != nil {
		return RecoveryResult{}, err
	}
	if err := writeMarker(directory, recoveryMarkerName, recoveryMarker); err != nil {
		return RecoveryResult{}, err
	}
	if err := finishRecoveryMove(directory); err != nil {
		return RecoveryResult{}, err
	}
	return RecoveryResult{IndexBytes: indexBytes, DiscardedPreviousBytes: previousBytes}, nil
}

// indexCompleted reports whether the root index has the completion marker.
// An index that Bleve cannot open counts as not completed.
func indexCompleted(directory string) bool {
	index, err := blevesearch.OpenUsing(directory, map[string]any{"read_only": true})
	if err != nil {
		return false
	}
	defer index.Close()
	marker, err := index.GetInternal([]byte(completionInternalKey))
	return err == nil && len(marker) > 0
}

// DiscardPreviousIndex deletes the previous index that a recovery kept. It
// needs the directory lock. It refuses while a recovery is not complete,
// because the previous index can then be the last complete index.
func DiscardPreviousIndex(directory string) (uint64, error) {
	directory, err := cleanIndexDirectory(directory)
	if err != nil {
		return 0, err
	}
	previous := filepath.Join(directory, previousIndexName)
	if !exists(previous) {
		return 0, nil
	}
	lock, err := lockSearchDirectory(directory)
	if err != nil {
		return 0, err
	}
	defer lock.Close()
	if err := validateIndexDirectory(directory); err != nil {
		return 0, err
	}
	if exists(filepath.Join(directory, recoveryMarkerName)) {
		return 0, fmt.Errorf("a search index recovery is not complete; start the provider and wait until Search is ready, then try again")
	}
	size, err := pathSize(previous)
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(previous); err != nil {
		return 0, err
	}
	return size, syncPath(directory)
}

// finishRecoveryMove moves the root index entries into previous/. It is
// idempotent, and both RecoverIndex and provider startup call it under the
// directory lock while the recovery marker exists.
func finishRecoveryMove(directory string) error {
	previous := filepath.Join(directory, previousIndexName)
	if exists(filepath.Join(previous, "index_meta.json")) {
		return nil
	}
	if err := os.MkdirAll(previous, 0o700); err != nil {
		return err
	}
	for _, name := range indexEntries {
		target := filepath.Join(previous, name)
		if exists(target) {
			continue
		}
		if err := os.Rename(filepath.Join(directory, name), target); err != nil {
			return fmt.Errorf("keep previous search index: %w", err)
		}
	}
	return errors.Join(syncPath(previous), syncPath(directory))
}

// resumeIndexRecovery runs under the directory lock, before Bleve opens the
// root index. It completes an interrupted move, and reports a kept previous
// index, which later deletions and key shredding do not change.
func (p *Projection) resumeIndexRecovery() error {
	if exists(filepath.Join(p.directory, recoveryMarkerName)) {
		if err := validateIndexDirectory(p.directory); err != nil {
			return err
		}
		if err := finishRecoveryMove(p.directory); err != nil {
			return err
		}
		p.logger.Info("Search index recovery in progress; building the index from EVT", "stage", "index_recovery")
	}
	if exists(filepath.Join(p.directory, previousIndexName)) {
		p.logger.Warn("A previous search index is kept; it contains message text that later retractions and key shredding do not remove; delete it with chatto search-provider discard-previous when you no longer need it", "stage", "previous_index_kept")
	}
	return nil
}

// completeStartup runs after the startup replay. It verifies a real query on
// the index, records the completion marker in the index, and ends a
// recovery. The bool reports that a recovery ended.
func (p *Projection) completeStartup(ctx context.Context) (bool, error) {
	if err := p.validateQuery(ctx); err != nil {
		return false, fmt.Errorf("search index validation query: %w", err)
	}
	if err := p.recordCompletion(); err != nil {
		return false, err
	}
	marker := filepath.Join(p.directory, recoveryMarkerName)
	if !exists(marker) {
		return false, nil
	}
	if err := os.Remove(marker); err != nil {
		return false, err
	}
	return true, p.syncDirectory()
}

// validateQuery proves that the index answers real queries. It runs the
// normal query builder path, and, when the index has documents, a search that
// must return one stored document. Neither query uses member data.
func (p *Projection) validateQuery(ctx context.Context) error {
	if _, err := p.query(ctx, &searchv1.QueryRequest{RoomIds: []string{queryProbeRoomID}, Order: searchv1.SearchOrder_SEARCH_ORDER_NEWEST, PageSize: 1}); err != nil {
		return err
	}
	done := p.diagnostics.beginRead()
	p.mu.RLock()
	defer p.mu.RUnlock()
	defer done()()
	if p.index == nil {
		return fmt.Errorf("search index is closed")
	}
	count, err := p.index.DocCount()
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	request := blevesearch.NewSearchRequestOptions(blevesearch.NewMatchAllQuery(), 1, 0, false)
	request.Fields = []string{"room_id"}
	result, err := p.index.SearchInContext(ctx, request)
	if err != nil {
		return err
	}
	if len(result.Hits) == 0 {
		return fmt.Errorf("index has %d documents but a search returned none", count)
	}
	if roomID, _ := result.Hits[0].Fields["room_id"].(string); roomID == "" {
		return fmt.Errorf("search returned a document without stored fields")
	}
	return nil
}

// recordCompletion stores the completion marker, unless the index already has
// it. Its value is the checkpoint record at the time of the proof.
func (p *Projection) recordCompletion() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index == nil {
		return fmt.Errorf("search index is closed")
	}
	existing, err := p.index.GetInternal([]byte(completionInternalKey))
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	data, err := json.Marshal(p.checkpoint)
	if err != nil {
		return err
	}
	return p.index.SetInternal([]byte(completionInternalKey), data)
}

// cleanIndexDirectory rejects paths that cannot be a dedicated directory.
func cleanIndexDirectory(directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	clean := filepath.Clean(directory)
	if directory == "" || clean == "." || clean == filepath.VolumeName(clean)+string(filepath.Separator) {
		return "", fmt.Errorf("search index requires a dedicated directory")
	}
	return clean, nil
}

func writeMarker(directory, name, content string) error {
	marker, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := marker.WriteString(content)
	if err := errors.Join(writeErr, marker.Sync(), marker.Close()); err != nil {
		return err
	}
	return syncPath(directory)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// pathSize returns the total size of the regular files below path, or zero
// when path does not exist.
func pathSize(path string) (uint64, error) {
	var total uint64
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += uint64(info.Size())
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return total, err
}

func formatBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
