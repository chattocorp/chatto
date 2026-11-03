package brainview

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// modificationTimes reads only frontmatter to associate stored IDs with file
// timestamps. IDs are not filenames (notably for conversation journals). Mirror
// means' hidden-file, symlink, and nested-collection boundaries; means itself
// remains responsible for deciding which documents are valid and unique.
func modificationTimes(ctx context.Context, directory string) (map[string]time.Time, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, errRead
	}
	defer root.Close()
	times := map[string]time.Time{}
	err = fs.WalkDir(root.FS(), ".", func(filename string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
		if filename != "." && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if filename != "." && entry.IsDir() {
			_, err := root.Lstat(path.Join(filename, "MEANS.md"))
			if err == nil {
				return fs.SkipDir
			}
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		if !entry.Type().IsRegular() || !strings.HasSuffix(filename, ".md") || filename == "MEANS.md" {
			return nil
		}
		file, err := root.Open(filename)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		} // concurrent deletion; refresh reconciles the list
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		var header struct {
			ID string `yaml:"id"`
		}
		// A malformed or exceptionally large header is not allowed to make
		// this auxiliary timestamp scan read an unbounded document body.
		if yaml.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&header) == nil && header.ID != "" {
			times[header.ID] = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return nil, errRead
	}
	return times, nil
}
