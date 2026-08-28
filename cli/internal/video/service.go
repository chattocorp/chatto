// Package video provides the durable asset-processing runtime unit and its
// ffmpeg-backed video processor.
package video

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/charmbracelet/log"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// processRequest is the in-process shape passed to the worker after the
// asset has been resolved from the projection.
type processRequest struct {
	RoomID         string
	AssetID        string
	MessageEventID string
	ContentType    string
	Attachment     *evtv1.Attachment
}

// Service performs one ffmpeg-backed processing attempt at a time. The
// asset-processing runtime unit owns queue delivery, concurrency, and retry.
type Service struct {
	core        *core.ChattoCore
	config      config.AssetProcessingConfig
	logger      *log.Logger
	ffmpegPath  string
	ffprobePath string
}

// NewService creates an ffmpeg-backed video processor.
func NewService(chattoCore *core.ChattoCore, cfg config.AssetProcessingConfig, logger *log.Logger) (*Service, error) {
	s := &Service{
		core:   chattoCore,
		config: cfg,
		logger: logger,
	}
	if err := s.resolveTools(); err != nil {
		return nil, err
	}
	return s, nil
}

// ProcessAsset processes one durable queue delivery synchronously. Queue
// acknowledgement remains the runtime unit's responsibility.
func (s *Service) ProcessAsset(ctx context.Context, assetID, messageEventID string) error {
	return s.processAsset(ctx, assetID, messageEventID)
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
// The room comes from the upload-time AssetCreatedEvent. The durable queue fact
// is committed atomically with the owning message, and the unit waits for its
// AssetProjection through the delivery sequence before calling this method.
//
// messageEventID is carried on the request (it's the owning message, known to
// the scheduler) and stamped onto the terminal event so subscribers resolve
// it off the event rather than via a projection lookup that would race.
func (s *Service) processAsset(ctx context.Context, assetID, messageEventID string) error {
	declared := s.core.GetAssetState(assetID).Creation
	if declared == nil || declared.GetAsset() == nil {
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
