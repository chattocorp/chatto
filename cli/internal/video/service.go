// Package video provides asynchronous video processing.
//
// The service consumes process requests off a NATS Core queue group, transcodes
// uploaded videos to web-friendly MP4 with ffmpeg, generates thumbnails, and
// emits AssetProcessingSucceeded / AssetProcessingFailed events. It implements
// service.Service for lifecycle management.
//
// Architecture: subscriber-side bounded concurrency via semaphore; multi-process
// deployments share load via the queue group. PostMessage publishes a request
// onto `chatto.video.process` and returns immediately so the GraphQL mutation
// never blocks on ffmpeg.
package video

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// processRequest is the in-process shape passed to the worker after the
// asset has been resolved from the projection.
type processRequest struct {
	RoomID      string
	AssetID     string
	ContentType string
	Attachment  *corev1.Attachment
}

// Service processes video attachments asynchronously off a NATS queue.
type Service struct {
	core        *core.ChattoCore
	nc          *nats.Conn
	config      config.VideoConfig
	logger      *log.Logger
	ffmpegPath  string
	ffprobePath string
}

// NewService creates a new video processing service. Pass the NATS connection
// so the worker can subscribe to the process-request queue group.
func NewService(chattoCore *core.ChattoCore, nc *nats.Conn, cfg config.VideoConfig, logger *log.Logger) *Service {
	return &Service{
		core:   chattoCore,
		nc:     nc,
		config: cfg,
		logger: logger,
	}
}

// Run starts the video processing service. Blocks until ctx is cancelled.
// Implements service.Service.
func (s *Service) Run(ctx context.Context) error {
	if err := s.resolveTools(); err != nil {
		s.logger.Error("ffmpeg not found — video processing disabled", "error", err)
		s.logger.Error("Install ffmpeg: brew install ffmpeg (macOS) or apk add ffmpeg (Alpine)")
		return nil // Don't crash the server, just disable video processing
	}

	maxConcurrent := s.config.MaxConcurrentOrDefault()
	s.logger.Info("Video processing service started",
		"ffmpeg", s.ffmpegPath,
		"ffprobe", s.ffprobePath,
		"max_concurrent", maxConcurrent,
	)

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	sub, err := s.nc.QueueSubscribe(core.SubjectVideoProcess, "video-workers", func(msg *nats.Msg) {
		var req core.VideoProcessRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			s.logger.Error("Failed to unmarshal video process request", "error", err)
			return
		}
		if req.AssetID == "" {
			s.logger.Error("Video process request missing asset_id")
			return
		}

		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.processAsset(ctx, req.AssetID); err != nil {
				s.logger.Error("Video processing failed", "asset_id", req.AssetID, "error", err)
			}
		}()
	})
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", core.SubjectVideoProcess, err)
	}
	defer sub.Unsubscribe()

	// Recover any in-flight assets that were enqueued by a prior process
	// but have no terminal manifest yet. The projection has to be caught up
	// before we can look anything up, so wait for boot first.
	go func() {
		if err := s.core.WaitForBoot(ctx); err != nil {
			return
		}
		s.core.RecoverUnmanifestedVideoAttachments(ctx)
	}()

	<-ctx.Done()
	s.logger.Info("Shutting down video processing service, waiting for in-flight jobs...")
	wg.Wait()
	s.logger.Info("Video processing service stopped")

	return nil
}

func (s *Service) resolveTools() error {
	ffmpegPath, err := resolveExecutable(s.config.FFmpegPath, "ffmpeg")
	if err != nil {
		return err
	}
	ffprobePath, err := resolveExecutable(s.config.FFprobePath, "ffprobe")
	if err != nil {
		return err
	}
	s.ffmpegPath = ffmpegPath
	s.ffprobePath = ffprobePath
	return nil
}

// processAsset resolves the asset from the projection and runs ffmpeg.
func (s *Service) processAsset(ctx context.Context, assetID string) error {
	declared, ok := s.core.RoomTimeline.AssetCreation(assetID)
	if !ok || declared.GetAsset() == nil {
		return fmt.Errorf("asset %s is not declared", assetID)
	}
	if declared.GetRoomId() == "" || declared.GetMessageEventId() == "" {
		return fmt.Errorf("asset %s is not a message-owned video asset", assetID)
	}
	req := processRequest{
		RoomID:      declared.GetRoomId(),
		AssetID:     assetID,
		ContentType: declared.GetAsset().GetContentType(),
		Attachment:  core.AttachmentFromAsset(declared.GetAsset()),
	}
	return s.processVideo(ctx, req)
}

// resolveExecutable finds the path to an executable, using the provided path
// or falling back to PATH lookup.
func resolveExecutable(configPath, name string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH: %w", name, err)
	}
	return path, nil
}
