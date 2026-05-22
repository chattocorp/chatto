// Package video provides a NATS-based video processing service.
//
// The service subscribes to video processing requests, transcodes uploaded videos
// to web-friendly MP4 format using ffmpeg, generates thumbnails, and publishes
// completion events. It implements service.Service for lifecycle management.
//
// Architecture: This runs in-process by default but is designed for future extraction
// into a standalone service. It communicates with the rest of Chatto exclusively
// through NATS (subscribe for requests, publish completion events) and core methods
// (read/write attachments and KV state).
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
)

// ProcessRequest is the payload published to request video processing.
//
// The video processing subject (`chatto.video.process`) is a NATS Core
// subscription, not JetStream — there are no persisted requests to worry
// about. The legacy `space_id` field was dropped in ADR-030 Phase 4; any
// stale in-flight request from a pre-Phase-4 binary will simply have its
// `space_id` ignored (unknown JSON fields are dropped silently) and the
// downstream code paths handle the empty-spaceID case.
type ProcessRequest struct {
	RoomID        string `json:"room_id"`
	AttachmentID  string `json:"attachment_id"`
	ContentType   string `json:"content_type"`
	MessageBodyID string `json:"message_body_id"`
}

// Service processes video attachments asynchronously.
// It subscribes to a NATS subject for processing requests and manages
// a pool of concurrent ffmpeg workers.
type Service struct {
	core        *core.ChattoCore
	nc          *nats.Conn
	config      config.VideoConfig
	logger      *log.Logger
	ffmpegPath  string
	ffprobePath string
}

// NewService creates a new video processing service.
func NewService(chattoCore *core.ChattoCore, nc *nats.Conn, cfg config.VideoConfig, logger *log.Logger) *Service {
	return &Service{
		core:   chattoCore,
		nc:     nc,
		config: cfg,
		logger: logger,
	}
}

// Run starts the video processing service. It blocks until ctx is cancelled.
// Implements service.Service.
func (s *Service) Run(ctx context.Context) error {
	// Resolve ffmpeg/ffprobe paths
	var err error
	s.ffmpegPath, err = resolveExecutable(s.config.FFmpegPath, "ffmpeg")
	if err != nil {
		s.logger.Error("ffmpeg not found — video processing disabled", "error", err)
		s.logger.Error("Install ffmpeg: brew install ffmpeg (macOS) or apk add ffmpeg (Alpine)")
		return nil // Don't crash the server, just disable video processing
	}
	s.ffprobePath, err = resolveExecutable(s.config.FFprobePath, "ffprobe")
	if err != nil {
		s.logger.Error("ffprobe not found — video processing disabled", "error", err)
		return nil
	}

	s.logger.Info("Video processing service started",
		"ffmpeg", s.ffmpegPath,
		"ffprobe", s.ffprobePath,
		"max_concurrent", s.config.MaxConcurrentOrDefault(),
	)

	// Semaphore for bounding concurrent processing
	sem := make(chan struct{}, s.config.MaxConcurrentOrDefault())
	var wg sync.WaitGroup

	// Subscribe with queue group for future multi-server support
	sub, err := s.nc.QueueSubscribe(core.SubjectVideoProcess, "video-workers", func(msg *nats.Msg) {
		var req ProcessRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			s.logger.Error("Failed to unmarshal video processing request", "error", err)
			return
		}

		s.logger.Info("Received video processing request",
			"attachment_id", req.AttachmentID,
		)

		// Acquire semaphore slot (bounded concurrency)
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			if err := s.processVideo(ctx, req); err != nil {
				s.logger.Error("Video processing failed",
					"attachment_id", req.AttachmentID,
					"error", err,
				)
			}
		}()
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", core.SubjectVideoProcess, err)
	}
	defer sub.Unsubscribe()

	// Block until context is cancelled
	<-ctx.Done()

	// Drain in-flight processing
	s.logger.Info("Shutting down video processing service, waiting for in-flight jobs...")
	wg.Wait()
	s.logger.Info("Video processing service stopped")

	return nil
}

// resolveExecutable finds the path to an executable, using the provided path or
// falling back to PATH lookup.
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
