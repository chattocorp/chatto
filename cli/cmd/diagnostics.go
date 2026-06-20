package cmd

import (
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"

	"github.com/charmbracelet/log"
)

func startStartupCPUProfile(path string) func() {
	path = strings.TrimSpace(path)
	if path == "" {
		return func() {}
	}

	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Error("Failed to create startup CPU profile directory", "path", path, "error", err)
			return func() {}
		}
	}

	f, err := os.Create(path)
	if err != nil {
		log.Error("Failed to create startup CPU profile", "path", path, "error", err)
		return func() {}
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		log.Error("Failed to start startup CPU profile", "path", path, "error", err)
		_ = f.Close()
		return func() {}
	}

	log.Info("Started startup CPU profile", "path", path)
	return func() {
		pprof.StopCPUProfile()
		if err := f.Close(); err != nil {
			log.Warn("Failed to close startup CPU profile", "path", path, "error", err)
			return
		}
		log.Info("Wrote startup CPU profile", "path", path)
	}
}
