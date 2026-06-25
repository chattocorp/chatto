package core

// MediaModel owns attachment/media storage, media URL generation, and resize
// cache operations.
//
// It currently embeds ChattoCore so the model boundary can be introduced
// without copying the full core dependency graph. As more models settle, this
// can shrink to explicit dependencies in the same direction as RoomModel and
// PresenceModel.
type MediaModel struct {
	*ChattoCore
}

func NewMediaModel(core *ChattoCore) *MediaModel {
	return &MediaModel{ChattoCore: core}
}

func (c *ChattoCore) media() *MediaModel {
	if c.mediaModel == nil {
		c.mediaModel = NewMediaModel(c)
	}
	return c.mediaModel
}
