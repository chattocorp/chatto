package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStartStartupCPUProfileWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles", "startup.pprof")

	stop := startStartupCPUProfile(path)
	deadline := time.Now().Add(150 * time.Millisecond)
	var n uint64
	for time.Now().Before(deadline) {
		n++
	}
	if n == 0 {
		t.Fatal("expected CPU work loop to run")
	}
	stop()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat startup profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("startup profile file is empty")
	}
}

func TestStartStartupCPUProfileEmptyPathIsNoop(t *testing.T) {
	startStartupCPUProfile("")()
}
