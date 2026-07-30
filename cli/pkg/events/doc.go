// Package events provides envelope-neutral event-sourcing mechanics backed by
// NATS JetStream.
//
// It owns opaque OCC publication, ordered projection replay, readiness
// barriers, projection handles, and optional snapshot/checkpoint lifecycles.
// Applications own event codecs, subject policy, and stream identity.
//
// This package is an incubation surface for a possible standalone module. Its
// API is not yet covered by a stability promise.
package events
