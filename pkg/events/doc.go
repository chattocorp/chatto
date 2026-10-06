// Package events provides envelope-neutral event-sourcing mechanics backed by
// NATS JetStream.
//
// It owns opaque publication with OCC guards, an explicit unguarded path for
// state-independent facts, selectable subject or whole-stream mutation
// boundaries, ordered projection replay, readiness barriers, projection
// handles, optional snapshot/checkpoint lifecycles, exact stream-message reads
// with optional process-local caching, and bounded durable pull-worker
// execution. Applications own event codecs, subject policy, projection
// catch-up, authorization, consumer contracts, and stream identity.
//
// Diagnostics go to a *slog.Logger; a nil logger discards them. Constructors
// panic when a required argument is missing or malformed and return an error
// when their options are invalid.
//
// This package is an independently versioned incubation module. Its API is not
// yet covered by a stability promise.
package events
