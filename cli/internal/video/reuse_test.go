package video

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/log"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func suitableProbe() *ProbeResult {
	return &ProbeResult{DurationMs: 13_000, Width: 1920, Height: 1080, VideoCodec: "h264", AudioCodec: "aac", source: ffprobeOutput{
		Format: ffprobeFormat{Name: "mov,mp4,m4a,3gp,3g2,mj2"},
		Streams: []ffprobeStream{
			{CodecType: "video", CodecName: "h264", Profile: "High", Level: 40, PixelFormat: "yuv420p", Width: 1920, Height: 1080, SampleAspectRatio: "1:1", FieldOrder: "progressive", FrameRate: "30/1", StartTime: "0"},
			{CodecType: "audio", CodecName: "aac", Profile: "LC", Channels: 2, SampleRate: "48000", StartTime: "0"},
		},
	}}
}

func TestReusableSource(t *testing.T) {
	tests := []struct {
		name   string
		change func(*ProbeResult)
		want   bool
	}{
		{"compatible", func(*ProbeResult) {}, true},
		{"missing aspect ratio", func(p *ProbeResult) { p.source.Streams[0].SampleAspectRatio = "" }, true},
		{"silent", func(p *ProbeResult) { p.source.Streams = p.source.Streams[:1] }, true},
		{"webm", func(p *ProbeResult) { p.source.Format.Name = "matroska,webm" }, false},
		{"hevc", func(p *ProbeResult) { p.source.Streams[0].CodecName = "hevc" }, false},
		{"ten bit", func(p *ProbeResult) { p.source.Streams[0].PixelFormat = "yuv420p10le" }, false},
		{"HDR", func(p *ProbeResult) { p.source.Streams[0].ColorTransfer = "smpte2084" }, false},
		{"rotation", func(p *ProbeResult) {
			p.source.Streams[0].SideDataList = []ffprobeSideData{{Rotation: json.RawMessage("90")}}
		}, false},
		{"anamorphic", func(p *ProbeResult) { p.source.Streams[0].SampleAspectRatio = "2:1" }, false},
		{"4K", func(p *ProbeResult) { p.source.Streams[0].Width = 3840 }, false},
		{"odd", func(p *ProbeResult) { p.source.Streams[0].Width = 1919 }, false},
		{"interlaced", func(p *ProbeResult) { p.source.Streams[0].FieldOrder = "tt" }, false},
		{"reordered frames", func(p *ProbeResult) { p.source.Streams[0].HasBFrames = 2 }, false},
		{"unknown rate", func(p *ProbeResult) { p.source.Streams[0].FrameRate = "0/0" }, false},
		{"high rate", func(p *ProbeResult) { p.source.Streams[0].FrameRate = "120/1" }, false},
		{"offset", func(p *ProbeResult) { p.source.Streams[0].StartTime = "5" }, false},
		{"mono audio", func(p *ProbeResult) { p.source.Streams[1].Channels = 1 }, false},
		{"audio layout", func(p *ProbeResult) { p.source.Streams[1].Channels = 4 }, false},
		{"audio codec", func(p *ProbeResult) { p.source.Streams[1].CodecName = "opus" }, false},
		{"extra stream", func(p *ProbeResult) { p.source.Streams = append(p.source.Streams, p.source.Streams[0]) }, false},
		{"high bitrate", func(p *ProbeResult) { p.DurationMs = 1 }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := suitableProbe()
			tt.change(p)
			if got := reusableSource(p, 1_000_000); got != tt.want {
				t.Fatalf("reusable = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlignedRenditions(t *testing.T) {
	source := &packagedRendition{durations: []int64{6000, 6000, 1000}, starts: []float64{1.4, 7.4, 13.4}}
	for _, tc := range []struct {
		name      string
		durations []int64
		want      bool
	}{
		{"aligned", []int64{6000, 6000, 1000}, true},
		{"audio rounding", []int64{6021, 6000, 979}, true},
		{"different count", []int64{10000, 3000}, false},
		{"different boundary", []int64{5000, 6000, 2000}, false},
		{"cumulative drift", []int64{6030, 6030, 940}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := alignedRenditions(source, &packagedRendition{durations: tc.durations, starts: []float64{1.4, 7.4, 13.4}}); got != tc.want {
				t.Fatalf("aligned = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAlignedRenditionsRejectsShiftedTimeline(t *testing.T) {
	source := &packagedRendition{durations: []int64{6000}, starts: []float64{1.4}}
	shifted := &packagedRendition{durations: []int64{6000}, starts: []float64{2.4}}
	if alignedRenditions(source, shifted) {
		t.Fatal("equal durations must not hide shifted timestamps")
	}
}

func TestCancelledPreparationDoesNotStartFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// No executable or logger: starting fallback work would fail this test.
	service := &Service{}
	if got := service.prepareVideoVariants(ctx, "asset-test-123", filepath.Join(t.TempDir(), "absent"), t.TempDir(), suitableProbe(), nil); len(got) != 0 {
		t.Fatal("cancelled attempt produced output")
	}
}

func TestReuseVideoWithFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	service := &Service{ffmpegPath: ffmpeg, ffprobePath: ffprobe, logger: log.New(io.Discard)}
	for _, tc := range []struct {
		name      string
		gop       string
		wantReuse bool
		height    int
		variants  int
		rate      int
		sar       int
	}{
		{"missing aspect ratio", "48", true, 1080, 3, 24, 0},
		{"frequent keyframes", "48", true, 1080, 3, 24, 1},
		{"long GOP at 60 fps", "300", true, 1080, 3, 60, 1},
		{"long GOP at 30 fps", "300", true, 1080, 3, 30, 1},
		{"sparse keyframes", "10000", false, 1080, 2, 24, 1},
		{"small source needs no encoder", "48", true, 360, 1, 24, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			service.logger = log.New(&logs)
			tmp := t.TempDir()
			input := filepath.Join(tmp, "source.mp4")
			cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=192x%d:rate=%d:duration=13", tc.height, tc.rate), "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=48000:duration=13", "-vf", fmt.Sprintf("setsar=%d", tc.sar), "-c:v", "libx264", "-preset", "ultrafast", "-crf", "28", "-g", tc.gop, "-keyint_min", tc.gop, "-sc_threshold", "0", "-bf", "0", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", "-y", input)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fixture: %v\n%s", err, out)
			}
			probe, err := service.probe(ctx, input, "video/mp4")
			if err != nil {
				t.Fatal(err)
			}
			if tc.sar == 0 && probe.source.Streams[0].SampleAspectRatio != "" {
				t.Fatalf("fixture has an aspect ratio tag: %q", probe.source.Streams[0].SampleAspectRatio)
			}
			outputs := service.prepareVideoVariants(ctx, "asset-test-123", input, tmp, probe, nil)
			if len(outputs) == 0 {
				t.Fatal("no output")
			}
			reused := outputs[0].path == input
			if reused != tc.wantReuse {
				t.Fatalf("reused = %v, want %v; probe=%+v", reused, tc.wantReuse, probe.source)
			}
			if !reused && (!strings.Contains(logs.String(), "Video source reuse fell back") || !strings.Contains(logs.String(), "reason=independent_decode")) {
				t.Fatalf("missing fallback log: %s", logs.String())
			}

			if tc.variants > 1 && (!strings.Contains(logs.String(), "Transcoding variant") || !strings.Contains(logs.String(), "asset_id=asset-test-123")) {
				t.Fatalf("missing asset-scoped variant log: %s", logs.String())
			}

			if reused {
				if len(outputs) != tc.variants || outputs[0].height != int32(tc.height) {
					t.Fatalf("want source plus lower renditions, got %+v", outputs)
				}
				for _, output := range outputs {
					if output.packaged == nil || !alignedRenditions(outputs[0].packaged, output.packaged) {
						t.Fatal("unaligned output")
					}
				}
				if err := os.WriteFile(outputs[0].packaged.paths[0], []byte("damaged segment"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := service.validateCopiedRendition(ctx, outputs[0].packaged); err == nil {
					t.Fatal("damaged copied segment passed validation")
				}
			} else if outputs[0].height != 720 {
				t.Fatalf("fallback height = %d", outputs[0].height)
			}
		})
	}
}
