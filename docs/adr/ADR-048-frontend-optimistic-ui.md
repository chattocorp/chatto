# ADR-048: Frontend Optimistic UI Uses Scoped Provisional Patches

**Date:** 2026-07-09

## Context

Chatto's web client increasingly needs interactions that feel instant, such as
message reactions and small per-viewer state changes. The authoritative state
still comes from ConnectRPC projected rows and live/refetch delivery. If
optimistic UI code mutates projected rows directly without a shared convention,
independent pending actions can invalidate each other or roll back unrelated
message state that changed while an RPC was in flight.

## Decision

Frontend optimistic updates are provisional patches layered onto the currently
loaded client state. Each pending mutation is keyed by the smallest state slice
it owns, such as rendered message row plus reaction emoji, and receives a token
so stale RPC success/failure handlers cannot update a newer optimistic patch.

Optimistic rollback restores only the state slice touched by that mutation. It
must not replace an entire projected row when only one field or summary was
optimistically changed. When projected server rows are fetched or refetched, the
projected row remains authoritative and clears pending optimistic mutations for
the affected row.

For state such as room unread markers, the provisional patch may be a render
overlay rather than a mutation of the underlying server fact. This lets a failed
read reveal the previous unread state without allowing rollback to erase a newer
message or an authoritative read event.

Components should route a given optimistic action family through one shared
frontend action path instead of mixing direct RPC calls with local patches.

## Consequences

- Optimistic interactions can feel immediate while preserving the server's
  projected state as the source of truth.
- Concurrent optimistic actions on the same rendered resource can settle
  independently when their keys differ.
- Rollbacks are more work than whole-row restoration because each optimistic
  action must define the exact state slice it owns.
- Shared helpers can provide mutation tokens and stale checks, but domain
  modules still own their patch, reconcile, and rollback semantics.
