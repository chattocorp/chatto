// Package events provides envelope-neutral event-sourcing mechanics backed by
// NATS JetStream.
//
// It owns opaque OCC publication, selectable subject or whole-stream mutation
// boundaries, ordered projection replay, readiness barriers, projection
// handles, optional snapshot/checkpoint lifecycles, and bounded durable
// pull-worker execution. Applications own event codecs, subject policy,
// projection catch-up, authorization, consumer contracts, and stream identity.
//
// This package is an independently versioned incubation module. Its API is not
// yet covered by a stability promise.
package events
