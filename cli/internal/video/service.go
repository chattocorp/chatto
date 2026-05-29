// Package video provides asynchronous video processing.
//
// The service registers a process-local callback with core, transcodes uploaded
// videos to web-friendly MP4 with ffmpeg, generates thumbnails, and emits
// AssetProcessingSucceeded / AssetProcessingFailed events. It implements
// service.Service for lifecycle management.
//
// Architecture: process-local bounded concurrency via semaphore. PostMessage
// asks this service to spawn local work and returns immediately so the GraphQL
// mutation never blocks on ffmpeg. This intentionally remains best-effort until
// a real durable task queue exists.
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

// processRequest is the in-process shape passed to the worker after the
// asset has been resolved from the projection.
type processRequest struct {
	RoomID         string
	AssetID        string
	MessageEventID string
	ContentType    string
	Attachment     *corev1.Attachment
}

// Service processes video attachments asynchronously inside this process.
type Service struct {
	core        *core.ChattoCore
	config      config.VideoConfig
	logger      *log.Logger
	ffmpegPath  string
	ffprobePath string
	sem         chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.Mutex
	stopped     bool
}

// NewService creates a new process-local video processing service and registers
// it as core's best-effort video processing handler.
func NewService(chattoCore *core.ChattoCore, cfg config.VideoConfig, logger *log.Logger) (*Service, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		core:   chattoCore,
		config: cfg,
		logger: logger,
		sem:    make(chan struct{}, cfg.MaxConcurrentOrDefault()),
		ctx:    ctx,
		cancel: cancel,
	}
	if err := s.resolveTools(); err != nil {
		cancel()
		return nil, err
	}
	chattoCore.OnVideoProcessingRequested = s.StartProcessing
	return s, nil
}

// Run starts the video processing service. Blocks until ctx is cancelled.
// Implements service.Service.
func (s *Service) Run(ctx context.Context) error {
	maxConcurrent := s.config.MaxConcurrentOrDefault()
	s.logger.Info("Video processing service started",
		"ffmpeg", s.ffmpegPath,
		"ffprobe", s.ffprobePath,
		"max_concurrent", maxConcurrent,
	)

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

	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()

	s.logger.Info("Video processing service stopped")

	return nil
}

// StartProcessing schedules one asset for local processing and returns
// immediately. The actual ffmpeg work happens in a goroutine under the
// service's concurrency limit.
func (s *Service) StartProcessing(_ context.Context, assetID, messageEventID string) error {
	if assetID == "" {
		return fmt.Errorf("video processing missing asset id")
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("video processing service stopped")
	}
	s.wg.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		select {
		case s.sem <- struct{}{}:
			defer func() { <-s.sem }()
		case <-s.ctx.Done():
			return
		}
		if err := s.processAsset(s.ctx, assetID, messageEventID); err != nil {
			s.logger.Error("Video processing failed", "asset_id", assetID, "error", err)
		}
	}()
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
//
// The room comes from the upload-time AssetCreatedEvent, which is the only
// asset fact guaranteed to be projected by the time local work starts:
// PostMessage schedules processing *before* the MessagePosted event is durably
// appended, so message ownership may not be visible yet. We don't re-check
// message ownership here — a request only exists because PostMessage (or boot
// recovery) scheduled it for a message-owned video attachment.
//
// messageEventID is carried on the request (it's the owning message, known to
// the scheduler) and stamped onto the terminal event so subscribers resolve
// it off the event rather than via a projection lookup that would race.
func (s *Service) processAsset(ctx context.Context, assetID, messageEventID string) error {
	declared, ok := s.core.RoomTimeline.AssetCreation(assetID)
	if !ok || declared.GetAsset() == nil {
		return fmt.Errorf("asset %s is not declared", assetID)
	}
	if declared.GetRoomId() == "" {
		return fmt.Errorf("asset %s has no room scope", assetID)
	}
	req := processRequest{
		RoomID:         declared.GetRoomId(),
		AssetID:        assetID,
		MessageEventID: messageEventID,
		ContentType:    declared.GetAsset().GetContentType(),
		Attachment:     core.AttachmentFromAsset(declared.GetAsset()),
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
