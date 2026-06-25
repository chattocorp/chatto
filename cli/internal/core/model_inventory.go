package core

type modelRegistration struct {
	key              string
	name             string
	legacyServiceKey string
}

// ModelMetadata is stable metadata for a core model exposed in operator
// metrics and diagnostics.
type ModelMetadata struct {
	Key              string
	Name             string
	LegacyServiceKey string
}

// ModelMetadata returns the core model inventory registered by this process.
func (c *ChattoCore) ModelMetadata() []ModelMetadata {
	out := make([]ModelMetadata, 0, len(c.models))
	for _, model := range c.models {
		out = append(out, ModelMetadata{
			Key:              model.key,
			Name:             model.name,
			LegacyServiceKey: model.legacyServiceKeyOrDefault(),
		})
	}
	return out
}

func (r modelRegistration) legacyServiceKeyOrDefault() string {
	if r.legacyServiceKey != "" {
		return r.legacyServiceKey
	}
	return r.key
}
