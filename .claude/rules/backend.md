---
paths: ["cli/**"]
---

# Backend Development

## Architecture

- `ChattoCore` handles all domain operations (spaces, users, rooms, messages)
- KV buckets are source of truth; event streams provide audit trail
- IDs: 14-char NanoID via `helpers.NewID()` (~83.4 bits entropy)
- When adding streams/KV buckets, update `docs/ARCHITECTURE.md` inventory

## NATS/JetStream Tips

- `kv.Create()` for atomic insert (fails if key exists) vs `kv.Put()` for upsert
- `kv.ListKeysFiltered(ctx, ...filters)` for efficient key queries
- Consumer names can't have dots; KV keys use dots (act as NATS subjects)
- Streams/KV are immediately consistent within the cluster
- KV keys can't contain arbitrary Unicode; use named identifiers (e.g., `thumbsup` not `👍`)
- **Optimistic locking**: When updating KV entries that may have concurrent writers, use revision-based updates:
  1. Get current entry with `kv.Get()` to obtain its revision
  2. Use `kv.Update(ctx, key, value, revision)` for atomic update (fails if revision changed)
  3. For new keys, use `kv.Create()` instead (fails if key exists)
  4. Retry on `jetstream.ErrKeyExists` up to a max attempts (e.g., 5 retries)
- **Subject structure changes are high-risk**: Changes to NATS subject patterns cascade into stream configs, consumer filters, and query logic (e.g., `GetLastMsgForSubject`, `WithSubjectFilter`). They need careful end-to-end verification including e2e tests.
- **Single unified event stream**: All room events (channels and DMs) live in `SERVER_EVENTS`, created up-front at boot. `getSpaceStream()` returns the same stream regardless of input — the `kind` segment in subjects (`server.room.channel.*` / `server.room.dm.*`) is what disambiguates. The pre-Phase-4 lazy per-space stream cache is gone.

## Room Event Query Behavior

`GetRoomEvents` uses three optimized code paths based on room size and query type:

- **Small room fast path**: Uses `stream.Info(WithSubjectFilter)` to check total event count. If ≤ `limit`, fetches everything in one consumer with `DeliverAllPolicy`
- **Initial load (large rooms)**: Uses `GetLastMsgForSubject` (O(1) lookup) to find the room's last event sequence, then starts a consumer near the end using `DeliverByStartSequencePolicy` with progressive multipliers (3×, 10×, 50×). Falls back to `DeliverAllPolicy` if needed
- **Pagination (large rooms)**: Uses `beforeTime` as the cursor. Tries a single 30-day window, falling back to `DeliverAllPolicy` if insufficient events are found

**Important**: `room_last_msg_at` in the RUNTIME bucket only tracks MESSAGE timestamps, not join/leave events. This is intentional for sorting by "recent activity" (conversations with recent messages). Don't rely on this field to determine when the most recent _event_ occurred.

## Event Patterns

- **JetStream events**: For data needing audit trail, ordering, or replay (messages, memberships)
- **Live-only events**: For transient UI updates where KV is source of truth (reactions, typing indicators)
  - Publish to `live.` subject prefix via `publishLiveEvent()`
  - Bypasses JetStream storage entirely
- **Adding new live event types** requires updates in TWO places:
  1. Core handler: Add case in the `liveRoomMsgChan` switch inside `StreamMyServerEvents` (room-scoped) or in `StreamMyInstanceLiveEvents` (instance-scoped) — both in `core.go`
  2. GraphQL resolver: Add case in `liveEventResolver.Event` (`events.resolvers.go`)
  - Missing the resolver case causes events to silently fail at the GraphQL layer
- **Avoid fan-out on publish**: When broadcasting events to multiple users (e.g., server updates), do NOT iterate through recipients and publish to each. Instead:
  - Publish once to a scoped subject (e.g., `instance.space.{spaceId}.updated`)
  - Use server-side authorization filtering in the subscription handler
  - This scales to large numbers of users without N publish operations

## Instance Event Authorization

Instance events use subject pattern `live.instance.{scope}.{id}.{eventType}` and are filtered by `isAuthorizedForInstanceEvent` in `core.go`:

| Scope   | Subject Pattern                   | Delivered To                    |
| ------- | --------------------------------- | ------------------------------- |
| `user`  | `live.instance.user.{userId}.*`   | Only that user (private events) |
| `space` | `live.instance.space.{spaceId}.*` | All space members               |

**Adding a new instance event type:**

1. Add protobuf message to `live_event.proto`
2. Add to GraphQL schema in `events.graphqls` (type + union)
3. Add `IsInstanceEventType()` method in `pb/chatto/core/v1/graphql.go`
4. Add case in `unwrapInstanceEvent()` in `event_helpers.go`
5. Publish using `subjects.InstanceUserEvent()` or `subjects.InstanceSpaceEvent()`
6. Subscribe in frontend via `instanceEventBus.svelte.ts`

**When to create a live event:** Any time a user action changes state that other tabs/devices or other UI components need to reflect in real-time. Common triggers:
- User changes a preference or setting (notification level, follow state)
- Server-side auto-mutations (auto-follow on posting to a thread)
- Cross-tab sync needs (reading a room in one tab should update indicators in others)

If a mutation changes state visible in the UI and you don't publish a live event, the UI will be stale until refresh. Always consider: "Will other tabs or other components on the same page need to know about this change?"

**Broadcasting user events to everyone**: By default, user-scoped events are private (only delivered to that user). To broadcast an event to all authenticated users (e.g., profile updates since profiles are public), add an explicit check in `isAuthorizedForInstanceEvent`:

```go
case "user":
    if eventType == "profile_updated" {
        return true  // Broadcast to all
    }
    return scopeID == userID  // Private to user
```

## Image Processing

- **nativewebp is lossless only**: `github.com/HugoSmits86/nativewebp` encodes VP8L (lossless WebP). There is no lossy quality option — the `Options` struct only has `UseExtendedFormat` for metadata containers. If lossy WebP is needed in the future, a different library would be required.
- **Thumbnail encoding is format-aware**: `TransformImage()` picks the output format based on the input:
  - **Animated GIF** → WebP (lossless, with proper frame compositing and disposal handling)
  - **Transparent static** → WebP (lossless, preserves alpha)
  - **Opaque static** → JPEG (lossy q80, smaller files)

  Opaque static images use JPEG rather than WebP because nativewebp is lossless-only, which would produce larger files for photos.
- **Image cache stores raw bytes without format metadata**. Use `DetectImageContentType()` (magic bytes) when serving cached images — never hardcode a content type.

## Service Lifecycle

- Long-running services use `Run(ctx context.Context) error` — blocks until ctx cancelled
- Use `signal.NotifyContext` for shutdown signals (not manual goroutine + channel)
- Use `errgroup` to coordinate multiple concurrent blocking services

## API Design

- Use GraphQL for all client-facing APIs - avoid REST endpoints for application logic
- gqlgen supports file uploads via the `Upload` scalar ([docs](https://gqlgen.com/reference/file-upload/))
- REST endpoints are acceptable only for: OAuth callbacks, webhooks, health checks, and pre-auth discovery (e.g., `GET /api/instance` for multi-instance client probing before GraphQL setup)

## Dataloaders

- Dataloaders are injected for **HTTP requests only**, not WebSocket connections
- WebSocket connections are long-lived; dataloader caches would become stale across subscription events (e.g., user updates display name mid-session)
- Subscription resolvers fall back to direct `core.Get*()` calls via helper methods like `r.getUser()`
- This is intentional: HTTP requests benefit from batching (loading room history with many reactions), while subscription events arrive one at a time and don't benefit from batching anyway

## Security

- All GraphQL mutations must check permissions via `core.RequirePermission()`

## Known Test Issues

- `TestAuthRoutes_TestEmailEndpoint` in `cli/internal/http_server/` is a pre-existing failure — do not investigate as a regression.

## Cost Reference

Hetzner volumes €53/TB with R3 replication (3x storage)
