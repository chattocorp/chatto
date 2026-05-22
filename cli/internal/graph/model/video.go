package model

// VideoProcessing represents the processing state of a video attachment.
// ThumbnailAttachmentID is an internal field used by the thumbnailUrl
// resolver.
type VideoProcessing struct {
	Status                VideoProcessingStatus
	DurationMs            *int64
	Width                 *int32
	Height                *int32
	ErrorMessage          *string
	Variants              []*VideoVariant
	ThumbnailAttachmentID string // internal: for thumbnailUrl resolver
}

// VideoVariant represents a transcoded quality variant of a video.
// AttachmentID is an internal field used by the url resolver.
type VideoVariant struct {
	Quality      string
	Width        int32
	Height       int32
	Size         int64
	AttachmentID string // internal: for building URL
}
