// Package video provides in-process video processing.
//
// The service transcodes uploaded videos to web-friendly MP4 format using
// ffmpeg, generates thumbnails, and publishes completion events. It implements
// service.Service for lifecycle management and installs a direct callback on
// ChattoCore for command-side processing.
//
// Architecture: This intentionally runs in-process for now. A future durable
// queue/worker setup should replace the direct callback when we need separate
// video worker processes.
package video

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/charmbracelet/log"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ProcessRequest is the in-process request shape for video processing.
type ProcessRequest struct {
	RoomID      string
	AssetID     string
	ContentType string
	Attachment  *corev1.Attachment
}

// Service processes video attachments in-process.
type Service struct {
	core        *core.ChattoCore
	config      config.VideoConfig
	logger      *log.Logger
	ffmpegPath  string
	ffprobePath string
	initMu      sync.Mutex
	initialized bool
}

// NewService creates a new video processing service.
func NewService(chattoCore *core.ChattoCore, cfg config.VideoConfig, logger *log.Logger) *Service {
	s := &Service{
		core:   chattoCore,
		config: cfg,
		logger: logger,
	}
	chattoCore.OnVideoProcessingRequested = s.ProcessAsset
	return s
}

// Run starts the video processing service. It blocks until ctx is cancelled.
// Implements service.Service.
func (s *Service) Run(ctx context.Context) error {
	if err := s.ensureInitialized(); err != nil {
		s.logger.Error("ffmpeg not found — video processing disabled", "error", err)
		s.logger.Error("Install ffmpeg: brew install ffmpeg (macOS) or apk add ffmpeg (Alpine)")
		s.core.OnVideoProcessingRequested = nil
		return nil // Don't crash the server, just disable video processing
	}

	s.logger.Info("Video processing service started",
		"ffmpeg", s.ffmpegPath,
		"ffprobe", s.ffprobePath,
	)

	go func() {
		if err := s.core.WaitForBoot(ctx); err != nil {
			return
		}
		s.core.RecoverUnmanifestedVideoAttachments(ctx)
	}()

	// Block until context is cancelled
	<-ctx.Done()
	s.logger.Info("Video processing service stopped")

	return nil
}

func (s *Service) ensureInitialized() error {
	s.initMu.Lock()
	defer s.initMu.Unlock()
	if s.initialized {
		return nil
	}

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
	s.initialized = true
	return nil
}

// ProcessAsset processes one asset synchronously in this process.
func (s *Service) ProcessAsset(ctx context.Context, assetID string) error {
	if err := s.ensureInitialized(); err != nil {
		return err
	}
	declared, ok := s.core.RoomTimeline.AssetCreation(assetID)
	if !ok || declared.GetAsset() == nil {
		return fmt.Errorf("asset %s is not declared", assetID)
	}
	owner := declared.GetMessage()
	if owner == nil || owner.GetRoomId() == "" {
		return fmt.Errorf("asset %s is not a message-owned video asset", assetID)
	}
	req := ProcessRequest{
		RoomID:      owner.GetRoomId(),
		AssetID:     assetID,
		ContentType: declared.GetAsset().GetContentType(),
		Attachment:  core.AttachmentFromAsset(declared.GetAsset()),
	}
	return s.processVideo(ctx, req)
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
