package http_server

import (
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/core/remoteimage"
)

func TestRemoteImageGeneration(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	first := remoteimage.SourceID("https://a.example/logo.png")
	second := remoteimage.SourceID("https://b.example/logo.png")

	// A generation lasts remoteImageGenerationLength for every source.
	for _, id := range []string{first, second} {
		generation := remoteImageGeneration(id, now)
		if got := remoteImageGeneration(id, now.Add(remoteImageGenerationLength)); got != generation+1 {
			t.Errorf("generation after one length = %d, want %d", got, generation+1)
		}
	}

	// Sources start their generations at different times of day, so their
	// refreshes do not all happen at once.
	boundary := func(id string) time.Duration {
		start := remoteImageGeneration(id, now)
		for step := time.Duration(0); step <= remoteImageGenerationLength; step += time.Minute {
			if remoteImageGeneration(id, now.Add(step)) != start {
				return step
			}
		}
		t.Fatalf("no generation boundary within one length")
		return 0
	}
	if boundary(first) == boundary(second) {
		t.Error("two sources refresh at the same time")
	}
}
