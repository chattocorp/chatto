package connectapi

import "context"

type realtimeDMThreadsCapabilityKey struct{}

// WithRealtimeDMThreads enables direct-message thread projection data for one
// realtime connection. Callers must use it only when the client advertises the
// matching protocol capability.
func WithRealtimeDMThreads(ctx context.Context) context.Context {
	return context.WithValue(ctx, realtimeDMThreadsCapabilityKey{}, true)
}

// RealtimeDMThreadsEnabled reports whether the caller opted in to
// direct-message thread projection data.
func RealtimeDMThreadsEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(realtimeDMThreadsCapabilityKey{}).(bool)
	return enabled
}
