package video

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// packagedRendition holds local HLS output. Nothing is uploaded until the entire
// reuse attempt passes its decode and alignment checks.
type packagedRendition struct {
	paths     []string
	durations []int64
	bandwidth int64
	starts    []float64
}

// reusableSource admits a deliberately small browser-compatible subset. Missing
// or ambiguous metadata falls back to encoding; client claims are never used.
func reusableSource(probe *ProbeResult, size int64) bool {
	if probe.DurationMs <= 0 || size <= 0 || float64(size)*8000/float64(probe.DurationMs) > 5_000_000 {
		return false
	}
	if !strings.Contains(","+probe.source.Format.Name+",", ",mp4,") {
		return false
	}
	videos, audios := 0, 0
	for _, stream := range probe.source.Streams {
		switch stream.CodecType {
		case "video":
			videos++
			if stream.CodecName != "h264" || stream.PixelFormat != "yuv420p" || stream.HasBFrames != 0 || stream.Level <= 0 || stream.Level > 42 {
				return false
			}
			switch stream.Profile {
			case "Constrained Baseline", "Baseline", "Main", "High":
			default:
				return false
			}
			if stream.Width < 2 || stream.Height < 2 || stream.Width%2 != 0 || stream.Height%2 != 0 || max(stream.Width, stream.Height) > 1920 || min(stream.Width, stream.Height) > 1080 {
				return false
			}
			if stream.SampleAspectRatio != "1:1" || stream.FieldOrder != "progressive" {
				return false
			}
			switch stream.ColorTransfer {
			case "", "unknown", "bt709", "smpte170m", "bt470bg":
			default:
				return false
			}
			// Display matrices can encode flips and arbitrary rotation. Require an
			// untransformed source; encoding remains responsible for normalization.
			if len(stream.SideDataList) > 0 || stream.Tags["rotate"] != "" {
				return false
			}
			rateParts := strings.Split(stream.FrameRate, "/")
			if len(rateParts) != 2 {
				return false
			}
			n, e1 := strconv.ParseFloat(rateParts[0], 64)
			d, e2 := strconv.ParseFloat(rateParts[1], 64)
			rate := n / d
			if e1 != nil || e2 != nil || !finite(rate) || rate < 1 || rate > 60 {
				return false
			}
			start, err := strconv.ParseFloat(stream.StartTime, 64)
			if err != nil || !finite(start) || math.Abs(start) > 0.001 {
				return false
			}
		case "audio":
			audios++
			start, err := strconv.ParseFloat(stream.StartTime, 64)
			if err != nil || !finite(start) || math.Abs(start) > 0.05 {
				return false
			}
			if stream.CodecName != "aac" || stream.Profile != "LC" || stream.Channels < 1 || stream.Channels > 2 || (stream.SampleRate != "44100" && stream.SampleRate != "48000") {
				return false
			}
		default:
			return false
		}
	}
	return videos == 1 && audios <= 1
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func (s *Service) packageRendition(ctx context.Context, input, directory string) (*packagedRendition, error) {
	playlist, paths, err := s.packageHLSRendition(ctx, input, directory)
	if err != nil {
		return nil, err
	}
	bandwidth, durations, err := hlsSegmentMetadata(playlist, paths)
	if err != nil {
		return nil, err
	}
	return &packagedRendition{paths: paths, durations: durations, bandwidth: bandwidth}, nil
}

// validateCopiedRendition checks bounded seek intervals and decodes each segment
// in a fresh decoder. A copied open GOP or damaged stream must not be published
// with the independent-segments promise. The attempt context bounds all probes.
func (s *Service) validateCopiedRendition(ctx context.Context, rendition *packagedRendition) error {
	if len(rendition.paths) > 600 {
		return fmt.Errorf("too many copied segments")
	}
	for i, path := range rendition.paths {
		if rendition.durations[i] > 12_000 {
			return fmt.Errorf("source keyframes are too far apart")
		}
		cmd := exec.CommandContext(ctx, s.ffmpegPath, "-v", "error", "-xerror", "-err_detect", "explode", "-i", path, "-map", "0:v:0", "-map", "0:a?", "-f", "null", "-")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("copied segment cannot decode independently: %w", err)
		}
	}
	return nil
}

// readSegmentStarts verifies the absolute presentation timeline as well as the
// playlist lengths. Equal segment durations alone do not establish alignment.
func (s *Service) readSegmentStarts(ctx context.Context, rendition *packagedRendition) error {
	for _, path := range rendition.paths {
		cmd := exec.CommandContext(ctx, s.ffprobePath, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=start_time", "-of", "json", path)
		output, err := cmd.Output()
		if err != nil {
			return err
		}
		var probe ffprobeOutput
		if err := json.Unmarshal(output, &probe); err != nil {
			return err
		}
		if len(probe.Streams) != 1 {
			return fmt.Errorf("missing segment video timeline")
		}
		start, err := strconv.ParseFloat(probe.Streams[0].StartTime, 64)
		if err != nil || !finite(start) {
			return fmt.Errorf("invalid segment video timeline")
		}
		rendition.starts = append(rendition.starts, start)
	}
	return nil
}

// alignedRenditions compares cumulative boundaries, so rounding errors cannot
// accumulate across a long clip. Fifty milliseconds permits audio priming and
// frame-time rounding without accepting a different keyframe schedule.
func alignedRenditions(source, candidate *packagedRendition) bool {
	if len(source.durations) != len(candidate.durations) || len(source.starts) != len(source.durations) || len(candidate.starts) != len(candidate.durations) {
		return false
	}
	var sourceTime, candidateTime int64
	for i, duration := range source.durations {
		if math.Abs(source.starts[i]-candidate.starts[i]) > 0.05 {
			return false
		}
		sourceTime += duration
		candidateTime += candidate.durations[i]
		if math.Abs(float64(sourceTime-candidateTime)) > 50 {
			return false
		}
	}
	return true
}

// prepareVideoVariants tries source reuse entirely on disk. Any unsuitable
// input, failed preflight, or misaligned lower rendition returns to the normal
// ladder. Cancellation must not start another expensive encoding attempt.
func (s *Service) prepareVideoVariants(ctx context.Context, input, tmp string, probe *ProbeResult, inputOpts []string) []processedVariantOutput {
	if outputs, err := s.tryReuseVideo(ctx, input, tmp, probe); err == nil {
		return outputs
	}
	if ctx.Err() != nil {
		return nil
	}
	return s.encodeVariants(ctx, input, tmp, selectVariantHeights(probe.Height), probe.AudioCodec != "", inputOpts, false)
}

func (s *Service) tryReuseVideo(ctx context.Context, input, tmp string, probe *ProbeResult) ([]processedVariantOutput, error) {
	info, err := os.Stat(input)
	if err != nil {
		return nil, err
	}
	if !reusableSource(probe, info.Size()) {
		return nil, fmt.Errorf("source requires normalization")
	}
	source, err := s.packageRendition(ctx, input, filepath.Join(tmp, "reuse-source"))
	if err != nil {
		return nil, err
	}
	if err := s.validateCopiedRendition(ctx, source); err != nil {
		return nil, err
	}
	if err := s.readSegmentStarts(ctx, source); err != nil {
		return nil, err
	}
	outputs := []processedVariantOutput{{path: input, quality: fmt.Sprintf("%dp", probe.Height), width: probe.Width, height: probe.Height, size: info.Size(), packaged: source}}
	var heights []int
	for _, height := range selectVariantHeights(probe.Height) {
		if height < int(probe.Height) {
			heights = append(heights, height)
		}
	}
	lower := s.encodeVariants(ctx, input, tmp, heights, probe.AudioCodec != "", nil, true)
	// A failed lower encode should use the normal ladder rather than silently
	// removing adaptive choices solely because the reuse path was selected.
	if len(lower) != len(heights) {
		return nil, fmt.Errorf("lower rendition encoding failed")
	}
	for i := range lower {
		packaged, err := s.packageRendition(ctx, lower[i].path, filepath.Join(tmp, fmt.Sprintf("reuse-%d", i)))
		if err != nil {
			return nil, err
		}
		if err := s.readSegmentStarts(ctx, packaged); err != nil {
			return nil, err
		}
		if !alignedRenditions(source, packaged) {
			return nil, fmt.Errorf("copied and encoded segment boundaries differ")
		}
		lower[i].packaged = packaged
	}
	return append(outputs, lower...), nil
}
