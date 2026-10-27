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

// importProgress stores opaque mappings and upload state. Complete lines are
// synced before dependent requests. The process lock permits crash-tail repair.
type importProgress struct {
	file      *os.File
	values    map[string]string
	namespace uuid.UUID
}
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
	ok = true
	return p, nil
}

func (p *importProgress) put(key, value string) error {
	if p.values[key] == value {
		return nil
	}
	if err := json.NewEncoder(p.file).Encode(progressEntry{key, value}); err != nil {
		return err
	}
	if err := p.file.Sync(); err != nil {
		return err
	}
	p.values[key] = value
	return nil
}

func (p *importProgress) operation(key string) string {
	return uuid.NewSHA1(p.namespace, []byte(key)).String()
}
