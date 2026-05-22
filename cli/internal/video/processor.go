package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ProbeResult contains metadata extracted from a video file via ffprobe.
type ProbeResult struct {
	DurationMs int64
	Width      int32
	Height     int32
	CodecInfo  string
	// VideoCodec is the video stream codec name (e.g., "h264", "hevc").
	VideoCodec string
	// AudioCodec is the audio stream codec name (e.g., "aac", "opus").
	AudioCodec string
}

// ffprobeOutput mirrors the relevant parts of ffprobe's JSON output.
type ffprobeOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

type ffprobeStream struct {
	CodecType string `json:"codec_type"`
	CodecName string `json:"codec_name"`
	Width     int32  `json:"width"`
	Height    int32  `json:"height"`
	Duration  string `json:"duration"`
}

type ffprobeFormat struct {
	Duration string `json:"duration"`
}

// probe extracts metadata from a video file using ffprobe.
// For GIF files, only stream metadata is read (skipping -show_format) because
// older ffprobe versions (4.4) may read all frames to calculate format duration,
// which hangs indefinitely on infinite-loop GIFs.
func (s *Service) probe(ctx context.Context, inputPath string, contentType string) (*ProbeResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// GIF files: skip -show_format to avoid ffprobe reading all frames for duration.
	// The duration isn't needed — we use a fixed cap for GIF transcoding.
	args := []string{"-v", "quiet", "-print_format", "json", "-show_streams"}
	if contentType != "image/gif" {
		args = append(args, "-show_format")
	}
	args = append(args, inputPath)

	cmd := exec.CommandContext(probeCtx, s.ffprobePath, args...)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probe ffprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	result := &ProbeResult{}

	// Parse format-level duration (seconds as string, e.g., "12.345000")
	if probe.Format.Duration != "" {
		var durationSec float64
		if _, err := fmt.Sscanf(probe.Format.Duration, "%f", &durationSec); err == nil {
			result.DurationMs = int64(durationSec * 1000)
		}
	}

	// Find video and audio streams
	var codecParts []string
	for _, stream := range probe.Streams {
		switch stream.CodecType {
		case "video":
			result.Width = stream.Width
			result.Height = stream.Height
			result.VideoCodec = stream.CodecName
			codecParts = append(codecParts, strings.ToUpper(stream.CodecName))
			// Use stream-level duration as fallback (e.g., when -show_format is
			// skipped for GIFs, the format duration is unavailable but the video
			// stream still reports its duration).
			if result.DurationMs == 0 && stream.Duration != "" {
				var durationSec float64
				if _, err := fmt.Sscanf(stream.Duration, "%f", &durationSec); err == nil {
					result.DurationMs = int64(durationSec * 1000)
				}
			}
		case "audio":
			result.AudioCodec = stream.CodecName
			codecParts = append(codecParts, strings.ToUpper(stream.CodecName))
		}
	}
	result.CodecInfo = strings.Join(codecParts, " / ")

	return result, nil
}

// generateThumbnail captures a frame from the video as a JPEG thumbnail.
// Captures at 1 second or 10% into the video, whichever is earlier.
// inputOpts are placed before -i (e.g., "-ignore_loop 1" for GIF input).
func (s *Service) generateThumbnail(ctx context.Context, inputPath, outputPath string, durationMs int64, inputOpts []string) error {
	// Seek to 1 second or 10% of duration, whichever is earlier
	seekMs := int64(1000)
	if tenPercent := durationMs / 10; tenPercent < seekMs && tenPercent > 0 {
		seekMs = tenPercent
	}
	seekTime := fmt.Sprintf("%.3f", float64(seekMs)/1000.0)

	args := append([]string{}, inputOpts...)
	args = append(args,
		"-ss", seekTime,
		"-i", inputPath,
		"-vframes", "1",
		"-vf", "scale='min(640,iw)':-2",
		"-q:v", "3",
		"-y",
		outputPath,
	)
	cmd := exec.CommandContext(ctx, s.ffmpegPath, args...)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("thumbnail generation failed: %w\noutput: %s", err, string(output))
	}

	return nil
}

// transcode converts a video to H.264 MP4 at the specified height.
// Uses -movflags +faststart for progressive download/seeking.
// inputOpts are placed before -i (e.g., "-ignore_loop 1" for GIF input).
func (s *Service) transcode(ctx context.Context, inputPath, outputPath string, height int, hasAudio bool, inputOpts []string) error {
	// yuv420p requires even dimensions; scale=-2 handles width, but height
	// can be odd when transcoding at the source's original resolution.
	if height%2 != 0 {
		height++
	}

	args := append([]string{}, inputOpts...)
	args = append(args,
		"-i", inputPath,
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
	)
	if hasAudio {
		args = append(args, "-c:a", "aac", "-b:a", "128k")
	} else {
		args = append(args, "-an")
	}
	args = append(args,
		"-vf", fmt.Sprintf("scale=-2:%d", height),
		"-movflags", "+faststart",
		"-max_muxing_queue_size", "1024",
		"-y",
		outputPath,
	)
	cmd := exec.CommandContext(ctx, s.ffmpegPath, args...)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("transcoding to %dp failed: %w\noutput: %s", height, err, string(output))
	}

	return nil
}

// selectVariantHeights decides which quality levels to produce based on source resolution.
func selectVariantHeights(sourceHeight int32) []int {
	switch {
	case sourceHeight >= 1080:
		return []int{720, 480}
	case sourceHeight >= 720:
		return []int{720, 480}
	case sourceHeight >= 480:
		return []int{480}
	default:
		// Source is smaller than 480p — transcode at original resolution
		return []int{int(sourceHeight)}
	}
}

// processVideo handles the full processing pipeline for a single video.
func (s *Service) processVideo(ctx context.Context, req ProcessRequest) error {
	// Per-job timeout prevents any single ffmpeg invocation from hanging forever.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Create temp directory for this job
	tmpDir, err := os.MkdirTemp(s.config.TempDir, "chatto-video-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Update state to PROCESSING
	if err := s.core.SetVideoProcessingState(ctx, req.AttachmentID, &corev1.VideoProcessingState{
		Status: corev1.VideoStatus_VIDEO_STATUS_PROCESSING,
	}); err != nil {
		return fmt.Errorf("failed to set processing state: %w", err)
	}

	// Download original from asset store
	inputPath := filepath.Join(tmpDir, "input")
	if err := s.downloadAttachment(ctx, req.AttachmentID, inputPath); err != nil {
		return s.failProcessing(ctx, req, fmt.Errorf("failed to download original: %w", err))
	}

	// Probe metadata
	probeResult, err := s.probe(ctx, inputPath, req.ContentType)
	if err != nil {
		return s.failProcessing(ctx, req, fmt.Errorf("failed to probe video: %w", err))
	}
	if probeResult.Height == 0 {
		return s.failProcessing(ctx, req, fmt.Errorf("no video stream found in file"))
	}

	s.logger.Info("Video probed",
		"attachment_id", req.AttachmentID,
		"duration_ms", probeResult.DurationMs,
		"resolution", fmt.Sprintf("%dx%d", probeResult.Width, probeResult.Height),
		"codec", probeResult.CodecInfo,
	)

	// GIF inputs need special handling to prevent ffmpeg from looping the animation
	// infinitely (older ffmpeg versions like 4.4 respect the GIF loop count).
	// Belt-and-suspenders approach:
	// 1. -ignore_loop 1 tells the demuxer to play once (may not work on all versions)
	// 2. -t caps input reading to the probed duration (or 30s fallback)
	var inputOpts []string
	if probeResult.VideoCodec == "gif" {
		durationSec := float64(probeResult.DurationMs) / 1000.0
		if durationSec <= 0 {
			durationSec = 30 // Fallback: cap at 30 seconds if probe couldn't determine duration
		}
		inputOpts = []string{"-ignore_loop", "1", "-t", fmt.Sprintf("%.3f", durationSec)}
	}

	// Generate thumbnail
	thumbPath := filepath.Join(tmpDir, "thumb.jpg")
	if err := s.generateThumbnail(ctx, inputPath, thumbPath, probeResult.DurationMs, inputOpts); err != nil {
		s.logger.Warn("Thumbnail generation failed, continuing without thumbnail", "error", err)
		thumbPath = "" // Non-fatal
	}

	// Upload thumbnail as attachment
	var thumbnailAttachmentID string
	if thumbPath != "" {
		thumbID, err := s.uploadFile(ctx, req.RoomID, "thumbnail.jpg", "image/jpeg", thumbPath)
		if err != nil {
			s.logger.Warn("Failed to upload thumbnail", "error", err)
		} else {
			thumbnailAttachmentID = thumbID
		}
	}

	// Transcode variants
	heights := selectVariantHeights(probeResult.Height)
	hasAudio := probeResult.AudioCodec != ""
	var variants []*corev1.VideoVariant

	for _, h := range heights {
		outputPath := filepath.Join(tmpDir, fmt.Sprintf("%dp.mp4", h))
		s.logger.Info("Transcoding variant",
			"attachment_id", req.AttachmentID,
			"height", h,
		)

		if err := s.transcode(ctx, inputPath, outputPath, h, hasAudio, inputOpts); err != nil {
			s.logger.Error("Variant transcoding failed", "height", h, "error", err)
			continue // Try remaining variants
		}

		// Get file info for size
		info, err := os.Stat(outputPath)
		if err != nil {
			s.logger.Error("Failed to stat transcoded file", "error", err)
			continue
		}

		// Upload variant as attachment
		quality := fmt.Sprintf("%dp", h)
		filename := fmt.Sprintf("%s_%s.mp4", strings.TrimSuffix(req.AttachmentID, filepath.Ext(req.AttachmentID)), quality)
		variantID, err := s.uploadFile(ctx, req.RoomID, filename, "video/mp4", outputPath)
		if err != nil {
			s.logger.Error("Failed to upload variant", "height", h, "error", err)
			continue
		}

		// Calculate output width, rounding up to the nearest even number to match
		// ffmpeg's scale=-2:HEIGHT behavior (which always outputs even dimensions).
		variantWidth := probeResult.Width * int32(h) / probeResult.Height
		if variantWidth%2 != 0 {
			variantWidth++
		}

		variants = append(variants, &corev1.VideoVariant{
			AttachmentId: variantID,
			Quality:      quality,
			Width:        variantWidth,
			Height:       int32(h),
			Size:         info.Size(),
		})
	}

	if len(variants) == 0 {
		return s.failProcessing(ctx, req, fmt.Errorf("all variant transcodes failed"))
	}

	// Update state to COMPLETED
	state := &corev1.VideoProcessingState{
		Status:                corev1.VideoStatus_VIDEO_STATUS_COMPLETED,
		ThumbnailAttachmentId: thumbnailAttachmentID,
		DurationMs:            probeResult.DurationMs,
		Width:                 probeResult.Width,
		Height:                probeResult.Height,
		Variants:              variants,
	}
	if err := s.core.SetVideoProcessingState(ctx, req.AttachmentID, state); err != nil {
		return fmt.Errorf("failed to set completed state: %w", err)
	}

	// Delete the original attachment (save storage — variants replace it).
	// Post-ADR-030-Phase-4 originals live at the kind-less S3 key
	// (`attachments/{id}`); passing an empty spaceID picks that layout.
	if err := s.core.DeleteAttachmentFromStorageByID(ctx, "", req.AttachmentID); err != nil {
		s.logger.Warn("Failed to delete original after transcoding", "error", err)
		// Non-fatal — the variants are already uploaded
	}

	// Publish live event
	kind, err := s.core.FindRoomKind(ctx, req.RoomID)
	if err != nil {
		s.logger.Warn("Failed to resolve room kind for video-completed event", "error", err)
	} else if err := s.core.PublishVideoProcessingCompleted(ctx, kind, req.RoomID, req.AttachmentID, req.MessageBodyID); err != nil {
		s.logger.Warn("Failed to publish video processing completed event", "error", err)
	}

	s.logger.Info("Video processing completed",
		"attachment_id", req.AttachmentID,
		"variants", len(variants),
	)

	return nil
}

// failProcessing updates the processing state to FAILED and returns the original error.
func (s *Service) failProcessing(ctx context.Context, req ProcessRequest, originalErr error) error {
	// Log the full error for server-side debugging (may contain file paths, ffmpeg output, etc.)
	s.logger.Error("Video processing failed",
		"attachment_id", req.AttachmentID,
		"error", originalErr)

	state := &corev1.VideoProcessingState{
		Status:       corev1.VideoStatus_VIDEO_STATUS_FAILED,
		ErrorMessage: "Video processing failed. Please try uploading again.",
	}
	if err := s.core.SetVideoProcessingState(ctx, req.AttachmentID, state); err != nil {
		s.logger.Error("Failed to set error state", "error", err)
	}
	// Publish live event even on failure so frontend can update
	kind, kindErr := s.core.FindRoomKind(ctx, req.RoomID)
	if kindErr != nil {
		s.logger.Warn("Failed to resolve room kind for video-failed event", "error", kindErr)
	} else if err := s.core.PublishVideoProcessingCompleted(ctx, kind, req.RoomID, req.AttachmentID, req.MessageBodyID); err != nil {
		s.logger.Warn("Failed to publish video processing failed event", "error", err)
	}
	return originalErr
}

// downloadAttachment downloads an attachment from the asset store to a local file.
func (s *Service) downloadAttachment(ctx context.Context, attachmentID, destPath string) error {
	// New uploads use the kind-less S3 layout (empty spaceID).
	reader, _, err := s.core.GetAttachmentFromAnyBackend(ctx, "", attachmentID)
	if err != nil {
		return err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return err
	}

	return nil
}

// uploadFile uploads a local file as an attachment and returns the new attachment ID.
func (s *Service) uploadFile(ctx context.Context, roomID, filename, contentType, srcPath string) (string, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	att, err := s.core.UploadAttachment(ctx, roomID, filename, contentType, f)
	if err != nil {
		return "", err
	}

	return att.Id, nil
}
