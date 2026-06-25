package core

type serviceRegistration struct {
	key  string
	name string
}

// ServiceMetadata is stable metadata for a core service exposed in operator
// metrics.
type ServiceMetadata struct {
	Key  string
	Name string
}

// ServiceMetadata returns the core service inventory registered by this process.
func (c *ChattoCore) ServiceMetadata() []ServiceMetadata {
	out := make([]ServiceMetadata, 0, len(c.services))
	for _, service := range c.services {
		out = append(out, ServiceMetadata{
			Key:  service.key,
			Name: service.name,
		})
	}
	return out
}
