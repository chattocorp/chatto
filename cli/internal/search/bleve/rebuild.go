package bleve

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	blevesearch "github.com/blevesearch/bleve/v2"
	bolt "go.etcd.io/bbolt"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const directoryLockName = ".chatto-search.lock"
const rebuildMarkerName = ".chatto-search-rebuild"
const rebuildMarker = "chatto-search-rebuild-v1\n"

// knownIndexContract recognizes formats shipped by Chatto, including a change
// to the analyzer set. Unknown future formats require operator intervention.
func knownIndexContract(id string) bool {
	base, fingerprint, ok := strings.Cut(id, "-v")
	if !ok || base != "bleve-message-index" {
		return false
	}
	version, fingerprint, ok := strings.Cut(fingerprint, "-")
	if !ok || len(fingerprint) != 16 {
		return false
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return false
	}
	switch version {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11":
		return true
	}
	return false
}

// lockDirectory holds an OS-backed lock through close/delete/create. The lock
// file stays inside the configured directory so dedicated volume mounts work.
func (p *Projection) lockDirectory() error {
	if err := os.MkdirAll(p.directory, 0o755); err != nil {
		return err
	}
	path := filepath.Join(p.directory, directoryLockName)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("search provider lock is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lock, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return fmt.Errorf("lock search index directory: %w; stop any other provider using this directory", err)
	}
	p.directoryLock = lock
	return nil
}

// validateRebuildDirectory limits deletion to a dedicated Bleve directory.
// Unrelated entries and links require an operator, even with a valid checkpoint.
func (p *Projection) validateRebuildDirectory() error {
	info, err := os.Lstat(p.directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("automatic search rebuild requires a real directory")
	}
	entries, err := os.ReadDir(p.directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "index_meta.json", directoryLockName, rebuildMarkerName:
			if !entry.Type().IsRegular() {
				return fmt.Errorf("automatic search rebuild requires regular metadata files")
			}
		case "store":
			if !entry.IsDir() {
				return fmt.Errorf("automatic search rebuild requires a real store directory")
			}
		default:
			return fmt.Errorf("search index directory contains unrelated entries; follow https://docs.chatto.run/guides/operations/search/#rebuild-the-search-index")
		}
	}
	return nil
}

func (p *Projection) rebuildIndex(request events.ProjectionCheckpointRequest) error {
	if err := p.validateRebuildDirectory(); err != nil {
		return err
	}
	p.logger.Info("Search index format changed; rebuilding from EVT", "stage", "index_rebuild")
	// Persist intent before deleting anything. On interruption the next start
	// discards the partial replacement and retries with the current mapping.
	marker, err := os.OpenFile(filepath.Join(p.directory, rebuildMarkerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := marker.WriteString(rebuildMarker)
	err = errors.Join(writeErr, marker.Sync(), marker.Close())
	if err != nil {
		return err
	}
	if err := p.syncDirectory(); err != nil {
		return err
	}
	if err := p.index.Close(); err != nil {
		p.index = nil
		return err
	}
	p.index = nil
	if err := p.resumeIndexRebuild(); err != nil {
		return err
	}
	p.deks = make(map[string]*evtv1.UserDEKGeneratedEvent)
	p.checkpoint = checkpointFromRequest(request)
	return nil
}

// resumeIndexRebuild is called under the directory lock, before opening Bleve.
// Keep the marker until a fresh index exists; retries never reuse a partial one.
func (p *Projection) resumeIndexRebuild() error {
	markerPath := filepath.Join(p.directory, rebuildMarkerName)
	data, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if string(data) != rebuildMarker {
		return fmt.Errorf("invalid search rebuild marker; follow https://docs.chatto.run/guides/operations/search/#rebuild-the-search-index")
	}
	if err := p.validateRebuildDirectory(); err != nil {
		return err
	}
	for _, name := range []string{"index_meta.json", "store"} {
		if err := os.RemoveAll(filepath.Join(p.directory, name)); err != nil {
			return err
		}
	}
	index, err := blevesearch.New(p.directory, newIndexMapping(p.languages))
	if err != nil {
		return err
	}
	p.index = index
	if err := p.syncDirectory(); err != nil {
		return err
	}
	if err := os.Remove(markerPath); err != nil {
		return err
	}
	return p.syncDirectory()
}

func (p *Projection) syncDirectory() error {
	directory, err := os.Open(p.directory)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
