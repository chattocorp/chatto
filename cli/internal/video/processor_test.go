package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

func TestHLSSegmentMetadata(t *testing.T) {
	tmp := t.TempDir()
	playlist := filepath.Join(tmp, "index.m3u8")
	if err := os.WriteFile(playlist, []byte("#EXTM3U\n#EXTINF:6.0,\none.ts\n#EXTINF:2.0,\ntwo.ts\n"), 0o600); err != nil {
		t.Fatalf("WriteFile playlist: %v", err)
	}
	segments := []string{filepath.Join(tmp, "one.ts"), filepath.Join(tmp, "two.ts")}
	if err := os.WriteFile(segments[0], make([]byte, 6000), 0o600); err != nil {
		t.Fatalf("WriteFile first segment: %v", err)
	}
	if err := os.WriteFile(segments[1], make([]byte, 4000), 0o600); err != nil {
		t.Fatalf("WriteFile second segment: %v", err)
	}
	got, durations, err := hlsSegmentMetadata(playlist, segments)
	if err != nil {
		t.Fatalf("hlsSegmentMetadata: %v", err)
	}
	if got != 16_000 {
		t.Fatalf("bandwidth = %d, want 16000", got)
	}
	if len(durations) != 2 || durations[0] != 6000 || durations[1] != 2000 {
		t.Fatalf("durations = %v, want [6000 2000]", durations)
	}
}

func TestVideoProcessingFinalizationContextSurvivesCancelledParent(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ctx, cancel := videoProcessingFinalizationContext(parent)
	defer cancel()
	if err := ctx.Err(); err != nil {
		t.Fatalf("finalization context inherited cancellation: %v", err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("finalization context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > videoProcessingFinalizationTimeout {
		t.Fatalf("finalization deadline remaining = %v, want within (0, %v]", remaining, videoProcessingFinalizationTimeout)
	}
}

func TestPackageHLSRenditionWithFFmpeg(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	tmp := t.TempDir()
	input := filepath.Join(tmp, "input.mp4")
	generate := exec.Command(
		ffmpegPath,
		"-f", "lavfi", "-i", "testsrc=size=160x90:rate=24:duration=13",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-force_key_frames", "expr:gte(t,n_forced*6)", "-sc_threshold", "0",
		"-y", input,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate ffmpeg fixture: %v\n%s", err, output)
	}

	service := &Service{ffmpegPath: ffmpegPath}
	playlistPath, segmentPaths, err := service.packageHLSRendition(context.Background(), input, filepath.Join(tmp, "hls"))
	if err != nil {
		t.Fatalf("packageHLSRendition: %v", err)
	}
	if len(segmentPaths) != 3 {
		t.Fatalf("segment count = %d, want 3", len(segmentPaths))
	}
	raw, err := os.ReadFile(playlistPath)
	if err != nil {
		t.Fatalf("ReadFile playlist: %v", err)
	}
	if !strings.Contains(string(raw), "#EXT-X-INDEPENDENT-SEGMENTS") || !strings.Contains(string(raw), "#EXT-X-ENDLIST") {
		t.Fatalf("unexpected media playlist: %s", raw)
	}
}

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

func TestVideoDisplayDimensions(t *testing.T) {
	tests := []struct {
		name       string
		stream     ffprobeStream
		wantWidth  int32
		wantHeight int32
	}{
		{
			name: "plain 16:9 stays unchanged",
			stream: ffprobeStream{
				Width:  1920,
				Height: 1080,
			},
			wantWidth:  1920,
			wantHeight: 1080,
		},
		{
			name: "display aspect ratio expands anamorphic storage pixels",
			stream: ffprobeStream{
				Width:              1440,
				Height:             1080,
				DisplayAspectRatio: "16:9",
			},
			wantWidth:  1920,
			wantHeight: 1080,
		},
		{
			name: "sample aspect ratio expands anamorphic storage pixels",
			stream: ffprobeStream{
				Width:             1440,
				Height:            1080,
				SampleAspectRatio: "4:3",
			},
			wantWidth:  1920,
			wantHeight: 1080,
		},
		{
			name: "quarter-turn rotation swaps display dimensions",
			stream: ffprobeStream{
				Width:  1920,
				Height: 1080,
				Tags:   map[string]string{"rotate": "90"},
			},
			wantWidth:  1080,
			wantHeight: 1920,
		},
		{
			name: "invalid aspect ratio falls back to storage dimensions",
			stream: ffprobeStream{
				Width:              1280,
				Height:             720,
				DisplayAspectRatio: "0:0",
				SampleAspectRatio:  "not-a-ratio",
			},
			wantWidth:  1280,
			wantHeight: 720,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWidth, gotHeight := videoDisplayDimensions(tt.stream)
			if gotWidth != tt.wantWidth || gotHeight != tt.wantHeight {
				t.Fatalf("videoDisplayDimensions() = %dx%d, want %dx%d", gotWidth, gotHeight, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}

func TestThumbnailDimensions(t *testing.T) {
	tests := []struct {
		name       string
		width      int32
		height     int32
		wantWidth  int32
		wantHeight int32
		wantOK     bool
	}{
		{
			name:       "16:9 display dimensions scale to square-pixel thumbnail",
			width:      1920,
			height:     1080,
			wantWidth:  640,
			wantHeight: 360,
			wantOK:     true,
		},
		{
			name:       "4:3 display dimensions stay 4:3",
			width:      1024,
			height:     768,
			wantWidth:  640,
			wantHeight: 480,
			wantOK:     true,
		},
		{
			name:       "small display dimensions are not upscaled",
			width:      320,
			height:     180,
			wantWidth:  320,
			wantHeight: 180,
			wantOK:     true,
		},
		{
			name:   "invalid display dimensions fall back to legacy filter",
			width:  0,
			height: 1080,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWidth, gotHeight, gotOK := thumbnailDimensions(tt.width, tt.height)
			if gotOK != tt.wantOK {
				t.Fatalf("thumbnailDimensions() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotOK && (gotWidth != tt.wantWidth || gotHeight != tt.wantHeight) {
				t.Fatalf("thumbnailDimensions() = %dx%d, want %dx%d", gotWidth, gotHeight, tt.wantWidth, tt.wantHeight)
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
