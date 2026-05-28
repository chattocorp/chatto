package model

// VideoProcessing represents the processing state of a video attachment.
// RoomID, OriginAttachmentID, and ThumbnailAttachmentID are internal
// fields used by the thumbnailUrl resolver to build a signed locator URL.
type VideoProcessing struct {
	Status                VideoProcessingStatus
	DurationMs            *int64
	Width                 *int32
	Height                *int32
	ReasonCode            *string
	SourceAvailable       bool
	Variants              []*VideoVariant
	RoomID                string // internal: for locator
	OriginAttachmentID    string // internal: parent video's attachment id (locator's VideoOrigin)
	ThumbnailAttachmentID string // internal: for thumbnailUrl resolver
}

// VideoVariant represents a transcoded quality variant of a video.
// RoomID, OriginAttachmentID, and AttachmentID are internal fields used
// by the url resolver to build a signed locator URL.
type VideoVariant struct {
	Quality            string
	Width              int32
	Height             int32
	Size               int64
	RoomID             string // internal: for locator
	OriginAttachmentID string // internal: parent video's attachment id (locator's VideoOrigin)
	AttachmentID       string // internal: this variant's attachment id
}
