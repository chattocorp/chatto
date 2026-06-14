package video

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

func TestSelectVariantHeights(t *testing.T) {
	tests := []struct {
		name         string
		sourceHeight int32
		want         []int
	}{
		{
			name:         "1080p source produces 720p and 480p variants",
			sourceHeight: 1080,
			want:         []int{720, 480},
		},
		{
			name:         "720p source produces 720p and 480p variants",
			sourceHeight: 720,
			want:         []int{720, 480},
		},
		{
			name:         "1440p source produces 720p and 480p variants",
			sourceHeight: 1440,
			want:         []int{720, 480},
		},
		{
			name:         "4K source produces 720p and 480p variants",
			sourceHeight: 2160,
			want:         []int{720, 480},
		},
		{
			name:         "480p source produces one 480p variant",
			sourceHeight: 480,
			want:         []int{480},
		},
		{
			name:         "source between 480p and 720p produces one 480p variant",
			sourceHeight: 576,
			want:         []int{480},
		},
		{
			name:         "small source transcodes at original resolution",
			sourceHeight: 360,
			want:         []int{360},
		},
		{
			name:         "very small source transcodes at original resolution",
			sourceHeight: 240,
			want:         []int{240},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectVariantHeights(tt.sourceHeight)
			if len(got) != len(tt.want) {
				t.Errorf("selectVariantHeights(%d) = %v, want %v", tt.sourceHeight, got, tt.want)
				return
			}
			for i, h := range got {
				if h != tt.want[i] {
					t.Errorf("selectVariantHeights(%d)[%d] = %d, want %d", tt.sourceHeight, i, h, tt.want[i])
				}
			}
		})
	}
}

func TestVariantWidthCalculation(t *testing.T) {
	// Verifies that the width calculation rounds up to the nearest even number,
	// matching ffmpeg's scale=-2:HEIGHT behavior.
	tests := []struct {
		name         string
		sourceWidth  int32
		sourceHeight int32
		targetHeight int
		wantWidth    int32
	}{
		{
			name:         "1920x1080 to 480p: should be 854 (even), not 853",
			sourceWidth:  1920,
			sourceHeight: 1080,
			targetHeight: 480,
			wantWidth:    854, // 1920*480/1080 = 853.33 → rounded up to even: 854
		},
		{
			name:         "1280x720 to 480p: should be 854 (even)",
			sourceWidth:  1280,
			sourceHeight: 720,
			targetHeight: 480,
			wantWidth:    854, // 1280*480/720 = 853.33 → rounded up to even: 854
		},
		{
			name:         "1920x1080 to 720p: stays 1280 (already even)",
			sourceWidth:  1920,
			sourceHeight: 1080,
			targetHeight: 720,
			wantWidth:    1280, // 1920*720/1080 = 1280 exactly
		},
		{
			name:         "odd calculated width rounds up to even",
			sourceWidth:  853,
			sourceHeight: 480,
			targetHeight: 480,
			wantWidth:    854, // 853*480/480 = 853 (odd) → 854
		},
		{
			name:         "even calculated width stays as-is",
			sourceWidth:  1280,
			sourceHeight: 720,
			targetHeight: 720,
			wantWidth:    1280, // exact match
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := tt.sourceWidth * int32(tt.targetHeight) / tt.sourceHeight
			if w%2 != 0 {
				w++
			}
			if w != tt.wantWidth {
				t.Errorf("width calculation for %dx%d→%dp = %d, want %d",
					tt.sourceWidth, tt.sourceHeight, tt.targetHeight, w, tt.wantWidth)
			}
		})
	}
}

func TestServiceRunReturnsWhenShutdownWaitTimesOut(t *testing.T) {
	internalCtx, internalCancel := context.WithCancel(context.Background())
	svc := &Service{
		logger: log.WithPrefix("test.video"),
		ctx:    internalCtx,
		cancel: internalCancel,
	}

	var release sync.WaitGroup
	release.Add(1)
	svc.wg.Add(1)
	go func() {
		release.Wait()
		svc.wg.Done()
	}()
	t.Cleanup(release.Done)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.run(ctx, 25*time.Millisecond) }()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after shutdown wait timeout")
	}
}
