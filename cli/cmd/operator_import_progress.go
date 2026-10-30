package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
)

// importProgress stores opaque mappings and upload state. Identity and upload
// checkpoints are synced before dependent requests. Results protected by durable
// server receipts can share a sync. The process lock permits crash-tail repair.
type importProgress struct {
	file      *os.File
	values    map[string]string
	namespace uuid.UUID
	unsynced  int
}

// Bound recovery work after a lost checkpoint without a sync per history RPC.
const importProgressSyncEvery = 128

type progressEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func openImportProgress(path, binding string) (*importProgress, error) {
	if path == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256([]byte(binding))
		dir := filepath.Join(cache, "chatto", "imports")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		path = filepath.Join(dir, hex.EncodeToString(sum[:])+".jsonl")
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	link, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(info, link) || link.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("import: progress must be a private regular file")
	}
	if err = lockImportProgress(f); err != nil {
		return nil, err
	}
	p := &importProgress{file: f, values: map[string]string{}}
	reader := bufio.NewReaderSize(f, 64<<10)
	var offset int64
	for {
		line, err := reader.ReadSlice('\n')
		if err == io.EOF {
			if len(line) > 0 {
				if err = f.Truncate(offset); err != nil {
					return nil, err
				}
			}
			break
		}
		if err != nil {
			return nil, err
		}
		var entry progressEntry
		if json.Unmarshal(line, &entry) != nil {
			return nil, errors.New("import: invalid progress journal")
		}
		p.values[entry.Key] = entry.Value
		offset += int64(len(line))
	}
	if _, err = f.Seek(0, io.SeekEnd); err != nil {
		return nil, err
	}
	if old := p.values["binding"]; old != "" && old != binding {
		return nil, errors.New("import: progress belongs to another archive, destination, or naming options")
	}
	if p.values["binding"] == "" {
		if err = p.put("binding", binding); err != nil {
			return nil, err
		}
	}
	if p.values["namespace"] == "" {
		if err = p.put("namespace", uuid.NewString()); err != nil {
			return nil, err
		}
	}
	p.namespace, err = uuid.Parse(p.values["namespace"])
	if err != nil {
		return nil, errors.New("import: invalid progress namespace")
	}
	// A previous process can leave complete but unsynced lines in the page
	// cache. Checkpoint them (and any tail repair) before using their results.
	if err = f.Sync(); err != nil {
		return nil, err
	}
	ok = true
	return p, nil
}

func (p *importProgress) put(key, value string) error {
	return p.write(key, value, true)
}

// putReplayable records a successful operation whose result can be recovered
// from a durable server receipt with the same operation key and arguments.
// The namespace and all non-replayable dependencies must already be synced.
// Never use this for upload sessions, identity claims, or the completion marker.
func (p *importProgress) putReplayable(key, value string) error {
	return p.write(key, value, false)
}

func (p *importProgress) write(key, value string, syncNow bool) error {
	if p.values[key] == value {
		return nil
	}
	if err := json.NewEncoder(p.file).Encode(progressEntry{key, value}); err != nil {
		return err
	}
	p.unsynced++
	if syncNow || p.unsynced >= importProgressSyncEvery {
		if err := p.file.Sync(); err != nil {
			return err
		}
		p.unsynced = 0
	}
	p.values[key] = value
	return nil
}

func (p *importProgress) operation(key string) string {
	return uuid.NewSHA1(p.namespace, []byte(key)).String()
}
