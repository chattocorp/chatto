// Package events provides envelope-neutral event-sourcing mechanics backed by
// NATS JetStream.
//
// It owns opaque OCC publication, selectable subject or whole-stream mutation
// boundaries, ordered projection replay, readiness barriers, projection
// handles, and optional snapshot/checkpoint lifecycles. Applications own event
// codecs, subject policy, projection catch-up, authorization, and stream
// identity.
//
// This package is an independently versioned incubation module. Its API is not
// yet covered by a stability promise.
package events
