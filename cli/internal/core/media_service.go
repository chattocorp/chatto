package core

// MediaService owns attachment/media storage, media URL generation, resize
// cache operations, and asset-processing event writes.
//
// It currently embeds ChattoCore so the service boundary can be introduced
// without copying the full core dependency graph. As more services settle, this
// can shrink to explicit dependencies in the same direction as RoomService and
// PresenceService.
type MediaService struct {
	*ChattoCore
}

func NewMediaService(core *ChattoCore) *MediaService {
	return &MediaService{ChattoCore: core}
}

func (c *ChattoCore) media() *MediaService {
	if c.mediaService == nil {
		c.mediaService = NewMediaService(c)
	}
	return c.mediaService
}
