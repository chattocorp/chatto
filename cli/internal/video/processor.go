package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"hmans.de/chatto/internal/core"
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
	CodecType          string            `json:"codec_type"`
	CodecName          string            `json:"codec_name"`
	Width              int32             `json:"width"`
	Height             int32             `json:"height"`
	Duration           string            `json:"duration"`
	DisplayAspectRatio string            `json:"display_aspect_ratio"`
	SampleAspectRatio  string            `json:"sample_aspect_ratio"`
	Tags               map[string]string `json:"tags"`
	SideDataList       []ffprobeSideData `json:"side_data_list"`
}

type ffprobeSideData struct {
	Rotation json.RawMessage `json:"rotation"`
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
			result.Width, result.Height = videoDisplayDimensions(stream)
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

func videoDisplayDimensions(stream ffprobeStream) (int32, int32) {
	width := stream.Width
	height := stream.Height
	if width <= 0 || height <= 0 {
		return width, height
	}

	displayRatio := float64(width) / float64(height)
	if ratio, ok := parseAspectRatio(stream.DisplayAspectRatio); ok {
		displayRatio = ratio
	} else if ratio, ok := parseAspectRatio(stream.SampleAspectRatio); ok {
		displayRatio *= ratio
	}

	if displayRatio > 0 {
		rawRatio := float64(width) / float64(height)
		if displayRatio >= rawRatio {
			width = int32(math.Round(float64(height) * displayRatio))
		} else {
			height = int32(math.Round(float64(width) / displayRatio))
		}
	}

	if videoRotatedByQuarterTurn(stream) {
		width, height = height, width
	}

	return width, height
}

func parseAspectRatio(value string) (float64, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, false
	}
	numerator, err := strconv.ParseFloat(parts[0], 64)
	if err != nil || numerator <= 0 {
		return 0, false
	}
	denominator, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || denominator <= 0 {
		return 0, false
	}
	return numerator / denominator, true
}

func videoRotatedByQuarterTurn(stream ffprobeStream) bool {
	if stream.Tags != nil {
		if rotation, ok := parseRotation(stream.Tags["rotate"]); ok {
			return rotation%180 != 0
		}
	}
	for _, data := range stream.SideDataList {
		if rotation, ok := parseRotationRaw(data.Rotation); ok && rotation%180 != 0 {
			return true
		}
	}
	return false
}

func parseRotation(value string) (int, bool) {
	rotation, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return normalizeRotation(rotation), true
}

func parseRotationRaw(value json.RawMessage) (int, bool) {
	raw := strings.TrimSpace(string(value))
	if raw == "" || raw == "null" {
		return 0, false
	}
	var rotation int
	if err := json.Unmarshal(value, &rotation); err == nil {
		return normalizeRotation(rotation), true
	}
	var text string
	if err := json.Unmarshal(value, &text); err == nil {
		return parseRotation(text)
	}
	return 0, false
}

func normalizeRotation(rotation int) int {
	rotation %= 360
	if rotation < 0 {
		rotation += 360
	}
	return rotation
}

// generateThumbnail captures a frame from the video as a JPEG thumbnail.
// Captures at 1 second or 10% into the video, whichever is earlier.
// inputOpts are placed before -i (e.g., "-ignore_loop 1" for GIF input).
func (s *Service) generateThumbnail(ctx context.Context, inputPath, outputPath string, durationMs int64, inputOpts []string, displayWidth, displayHeight int32) error {
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
		"-vf", thumbnailScaleFilter(displayWidth, displayHeight),
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

func thumbnailScaleFilter(displayWidth, displayHeight int32) string {
	width, height, ok := thumbnailDimensions(displayWidth, displayHeight)
	if !ok {
		return "scale='min(640,iw)':-2"
	}
	return fmt.Sprintf("scale=%d:%d,setsar=1", width, height)
}

func thumbnailDimensions(displayWidth, displayHeight int32) (int32, int32, bool) {
	if displayWidth <= 0 || displayHeight <= 0 {
		return 0, 0, false
	}
	const maxWidth = 640
	scale := math.Min(float64(maxWidth)/float64(displayWidth), 1)
	return int32(math.Round(float64(displayWidth) * scale)), int32(math.Round(float64(displayHeight) * scale)), true
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
		"-force_key_frames", "expr:gte(t,n_forced*6)",
		"-sc_threshold", "0",
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

type processedVariantOutput struct {
	path    string
	quality string
	width   int32
	height  int32
	size    int64
}

// packageHLSRendition segments a temporary MP4 rendition without re-encoding
// it. The MP4 encoder forces aligned six-second keyframes, so
// rendition switches occur at the same media boundaries.
func (s *Service) packageHLSRendition(ctx context.Context, inputPath, outputDir string) (string, []string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create HLS output directory: %w", err)
	}
	playlistPath := filepath.Join(outputDir, "index.m3u8")
	segmentPattern := filepath.Join(outputDir, "segment-%05d.ts")
	args := []string{
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a?",
		"-c", "copy",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", segmentPattern,
		"-y",
		playlistPath,
	}
	if output, err := exec.CommandContext(ctx, s.ffmpegPath, args...).CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("HLS packaging failed: %w\noutput: %s", err, string(output))
	}
	segmentPaths, err := filepath.Glob(filepath.Join(outputDir, "segment-*.ts"))
	if err != nil {
		return "", nil, fmt.Errorf("list HLS segments: %w", err)
	}
	sort.Strings(segmentPaths)
	if len(segmentPaths) == 0 {
		return "", nil, fmt.Errorf("HLS packaging produced no segments")
	}
	return playlistPath, segmentPaths, nil
}

func hlsSegmentMetadata(playlistPath string, segmentPaths []string) (int64, []int64, error) {
	raw, err := os.ReadFile(playlistPath)
	if err != nil {
		return 0, nil, fmt.Errorf("read HLS media playlist: %w", err)
	}
	var durations []float64
	for line := range strings.SplitSeq(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "#EXTINF:")
		if !ok {
			continue
		}
		value = strings.TrimSuffix(value, ",")
		duration, err := strconv.ParseFloat(value, 64)
		if err != nil || duration <= 0 {
			return 0, nil, fmt.Errorf("invalid HLS segment duration %q", value)
		}
		durations = append(durations, duration)
	}
	if len(durations) != len(segmentPaths) {
		return 0, nil, fmt.Errorf("HLS playlist contains %d segment durations, want %d", len(durations), len(segmentPaths))
	}

	var peak int64
	durationMillis := make([]int64, len(durations))
	for i, segmentPath := range segmentPaths {
		durationMillis[i] = int64(math.Round(durations[i] * 1000))
		if durationMillis[i] < 1 {
			return 0, nil, fmt.Errorf("HLS segment duration rounds to zero")
		}
		info, err := os.Stat(segmentPath)
		if err != nil {
			return 0, nil, fmt.Errorf("stat HLS segment: %w", err)
		}
		bandwidth := int64(math.Ceil(float64(info.Size()*8) / durations[i]))
		if bandwidth > peak {
			peak = bandwidth
		}
	}
	if peak < 1 {
		return 0, nil, fmt.Errorf("HLS peak bandwidth is zero")
	}
	return peak, durationMillis, nil
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
func (s *Service) processVideo(ctx context.Context, req processRequest) error {
	// Per-job timeout prevents any single ffmpeg invocation from hanging forever.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Create temp directory for this job
	tmpDir, err := os.MkdirTemp(s.config.TempDir, "chatto-video-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download original from asset store
	inputPath := filepath.Join(tmpDir, "input")
	if err := s.downloadAttachment(ctx, req.Attachment, inputPath); err != nil {
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
		"asset_id", req.AssetID,
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
	if err := s.generateThumbnail(ctx, inputPath, thumbPath, probeResult.DurationMs, inputOpts, probeResult.Width, probeResult.Height); err != nil {
		s.logger.Warn("Thumbnail generation failed, continuing without thumbnail", "error", err)
		thumbPath = "" // Non-fatal
	}

	// Upload thumbnail as a derivative asset of the original. Each derivative
	// upload writes its own AssetCreatedEvent with parent_asset_id set, so
	// the projection knows immediately that this asset is a child of the
	// original — no separate "claim as derivative" step downstream.
	var thumbnailAttachment *corev1.Attachment
	if thumbPath != "" {
		thumb, err := s.uploadDerivativeFile(ctx, req.AssetID, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_THUMBNAIL, req.RoomID, "thumbnail.jpg", "image/jpeg", thumbPath)
		if err != nil {
			s.logger.Warn("Failed to upload thumbnail", "error", err)
		} else {
			thumbnailAttachment = thumb
		}
	}

	// Transcode variants
	heights := selectVariantHeights(probeResult.Height)
	hasAudio := probeResult.AudioCodec != ""
	var variants []*corev1.VideoVariant
	var variantOutputs []processedVariantOutput

	for _, h := range heights {
		outputPath := filepath.Join(tmpDir, fmt.Sprintf("%dp.mp4", h))
		s.logger.Info("Transcoding variant",
			"asset_id", req.AssetID,
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

		variantProbe, err := s.probe(ctx, outputPath, "video/mp4")
		if err != nil {
			s.logger.Error("Failed to probe transcoded variant", "height", h, "error", err)
			continue
		}

		quality := fmt.Sprintf("%dp", h)
		output := processedVariantOutput{
			path: outputPath, quality: quality, width: variantProbe.Width,
			height: variantProbe.Height, size: info.Size(),
		}
		variantOutputs = append(variantOutputs, output)
	}

	if len(variantOutputs) == 0 {
		return s.failProcessing(ctx, req, fmt.Errorf("all variant transcodes failed"))
	}

	var hls *corev1.AssetProcessedHLS
	if probeResult.VideoCodec == "gif" {
		// Converted GIFs intentionally keep a single MP4 derivative: native
		// autoplay/loop playback is simpler and does not need seeking support.
		for _, output := range variantOutputs {
			filename := fmt.Sprintf("%s_%s.mp4", strings.TrimSuffix(req.AssetID, filepath.Ext(req.AssetID)), output.quality)
			attachment, err := s.uploadDerivativeFileWithDimensions(ctx, req.AssetID, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_VIDEO_VARIANT, req.RoomID, filename, "video/mp4", output.path, output.width, output.height)
			if err != nil {
				var uploaded []*corev1.Attachment
				if thumbnailAttachment != nil {
					uploaded = append(uploaded, thumbnailAttachment)
				}
				for _, variant := range variants {
					uploaded = append(uploaded, variant.GetAttachment())
				}
				s.cleanupUploadedDerivatives(ctx, req, uploaded)
				return s.failProcessing(ctx, req, fmt.Errorf("upload GIF video variant: %w", err))
			}
			variants = append(variants, &corev1.VideoVariant{AttachmentId: attachment.Id, Quality: output.quality, Width: output.width, Height: output.height, Size: output.size, Attachment: attachment})
		}
	} else {
		hls, err = s.generateAndUploadHLS(ctx, req, tmpDir, variantOutputs)
		if err != nil {
			if thumbnailAttachment != nil {
				s.cleanupUploadedDerivatives(ctx, req, []*corev1.Attachment{thumbnailAttachment})
			}
			return s.failProcessing(ctx, req, err)
		}
	}

	// Publish durable manifest. The original upload is retained as source
	// content for future re-encoding; generated variants are derivatives.
	kind, err := s.core.FindRoomKind(ctx, req.RoomID)
	if err != nil {
		s.logger.Warn("Failed to resolve room kind for video processed event", "error", err)
	} else if err := s.core.RecordAssetProcessedWithHLS(ctx, core.SystemActorID, kind, req.RoomID, req.MessageEventID, req.AssetID, probeResult.DurationMs, probeResult.Width, probeResult.Height, thumbnailAttachment, variants, hls); err != nil {
		s.logger.Warn("Failed to publish video processed event", "error", err)
	}

	s.logger.Info("Video processing completed",
		"asset_id", req.AssetID,
		"mp4_variants", len(variants),
		"hls_renditions", len(hls.GetRenditions()),
	)

	return nil
}

func (s *Service) generateAndUploadHLS(ctx context.Context, req processRequest, tmpDir string, outputs []processedVariantOutput) (*corev1.AssetProcessedHLS, error) {
	hlsDir := filepath.Join(tmpDir, "hls")
	manifest := &corev1.AssetProcessedHLS{}
	var uploaded []*corev1.Attachment
	cleanupOnError := func() {
		s.cleanupUploadedDerivatives(ctx, req, uploaded)
	}

	for i, output := range outputs {
		renditionDir := filepath.Join(hlsDir, fmt.Sprintf("rendition-%d", i))
		playlistPath, segmentPaths, err := s.packageHLSRendition(ctx, output.path, renditionDir)
		if err != nil {
			cleanupOnError()
			return nil, err
		}
		bandwidth, durations, err := hlsSegmentMetadata(playlistPath, segmentPaths)
		if err != nil {
			cleanupOnError()
			return nil, err
		}

		rendition := &corev1.AssetHLSRendition{
			Quality:   output.quality,
			Width:     output.width,
			Height:    output.height,
			Bandwidth: bandwidth,
		}
		for segmentIndex, segmentPath := range segmentPaths {
			segment, err := s.uploadDerivativeFile(ctx, req.AssetID, corev1.AssetDerivativeRole_ASSET_DERIVATIVE_ROLE_HLS_MEDIA_SEGMENT, req.RoomID, fmt.Sprintf("%s-%05d.ts", rendition.GetQuality(), segmentIndex), "video/mp2t", segmentPath)
			if err != nil {
				cleanupOnError()
				return nil, fmt.Errorf("upload HLS segment: %w", err)
			}
			uploaded = append(uploaded, segment)
			rendition.Segments = append(rendition.Segments, &corev1.AssetHLSSegment{AssetId: segment.GetId(), DurationMs: durations[segmentIndex]})
		}
		manifest.Renditions = append(manifest.Renditions, rendition)
	}

	if len(manifest.GetRenditions()) == 0 {
		return nil, fmt.Errorf("HLS generation produced no renditions")
	}
	return manifest, nil
}

func (s *Service) cleanupUploadedDerivatives(ctx context.Context, req processRequest, attachments []*corev1.Attachment) {
	kind, err := s.core.FindRoomKind(ctx, req.RoomID)
	if err != nil {
		s.logger.Warn("Failed to resolve room kind while cleaning partial HLS output", "asset_id", req.AssetID, "error", err)
		return
	}
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if err := s.core.RecordAssetDeleted(ctx, core.SystemActorID, kind, req.RoomID, attachment.GetId()); err != nil {
			s.logger.Warn("Failed to tombstone partial HLS derivative", "asset_id", attachment.GetId(), "error", err)
			continue
		}
		if err := s.core.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
			s.logger.Warn("Failed to delete partial HLS derivative", "asset_id", attachment.GetId(), "error", err)
		}
	}
}

// failProcessing records a durable failed outcome and returns the original error.
func (s *Service) failProcessing(ctx context.Context, req processRequest, originalErr error) error {
	// Log the full error for server-side debugging (may contain file paths, ffmpeg output, etc.)
	s.logger.Error("Video processing failed",
		"asset_id", req.AssetID,
		"error", originalErr)

	// Publish durable failure event even on failure so frontend can update
	// and replay can reconstruct the terminal state.
	kind, kindErr := s.core.FindRoomKind(ctx, req.RoomID)
	if kindErr != nil {
		s.logger.Warn("Failed to resolve room kind for video-failed event", "error", kindErr)
	} else if err := s.core.RecordAssetProcessingFailed(ctx, core.SystemActorID, kind, req.RoomID, req.MessageEventID, req.AssetID, corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED); err != nil {
		s.logger.Warn("Failed to publish video processing failed event", "error", err)
	}
	return originalErr
}

// downloadAttachment downloads an attachment from the asset store to a local file.
func (s *Service) downloadAttachment(ctx context.Context, attachment *corev1.Attachment, destPath string) error {
	if attachment == nil {
		return fmt.Errorf("attachment is nil")
	}
	reader, _, err := s.core.GetAttachmentReader(ctx, attachment)
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

// uploadDerivativeFile uploads a local file produced by the worker (thumbnail
// or transcoded variant) as a derivative of `parentAssetID`. The single
// AssetCreatedEvent emitted carries the parent + role so the projection
// links the derivative to its origin immediately.
func (s *Service) uploadDerivativeFile(ctx context.Context, parentAssetID string, derivativeRole corev1.AssetDerivativeRole, roomID, filename, contentType, srcPath string) (*corev1.Attachment, error) {
	return s.uploadDerivativeFileWithDimensions(ctx, parentAssetID, derivativeRole, roomID, filename, contentType, srcPath, 0, 0)
}

func (s *Service) uploadDerivativeFileWithDimensions(ctx context.Context, parentAssetID string, derivativeRole corev1.AssetDerivativeRole, roomID, filename, contentType, srcPath string, width, height int32) (*corev1.Attachment, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return s.core.UploadDerivativeAttachmentWithDimensions(ctx, parentAssetID, derivativeRole, roomID, filename, contentType, f, width, height)
}
