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

All event subscriptions are unified onto `live.server.>`. The `SERVER_EVENTS` stream's `RePublish` config forwards every accepted message from `server.>` to `live.server.>` after persistence, so `StreamMyEvents` in `core.go` only needs one `nc.ChanSubscribe("live.server.>")` to receive both durable (stream-stored) and transient (NATS Core) events. There is no per-connection JetStream consumer.

- **Durable events**: For data needing audit trail, ordering, or replay (messages, thread replies, room meta, server-level member events). Publish to `server.>` via `publishServerEvent()` / `publishServerEventWithAck()` / `publishServerEventWithOCC()`. JetStream republish automatically wires them into `live.server.>` for live delivery.
- **Transient events**: For real-time UI updates where KV is source of truth (reactions, typing, message edits/deletes, user/space/config notifications). Publish directly via NATS Core through `publishLiveServerEvent()` or `publishLiveEvent()`. No stream storage.
- **Do not double-publish.** Publishing the same conceptual event via BOTH `publishServerEvent` and `publishLiveServerEvent` will deliver it twice to every subscriber, because the stream-republish already covers the live path.
- **Adding new event types** requires:
  1. Core: choose durable vs transient and publish to the appropriate subject family. No `StreamMyEvents` change needed — the unified `live.server.>` subscription delivers it automatically.
  2. Authorization: room events are gated by membership in `filterLiveEvent`; user/space/config/member events go through `isAuthorizedForLiveEvent`. If your new event type fits neither, extend the appropriate switch.
  3. GraphQL: add a case to `unwrapEvent` in `event_helpers.go` so the typed variant flows through `myEvents`. Missing this case causes the event to silently fail at the GraphQL layer.
- **Avoid fan-out on publish**: When broadcasting to many users, do NOT iterate and publish per-recipient. Publish once to a scoped subject (e.g., `live.server.space.{spaceId}.updated`) and let `isAuthorizedForLiveEvent` filter on the subscriber side.

## Live Event Authorization

Non-room live events use subject pattern `live.server.{scope}.…` and are filtered by `isAuthorizedForLiveEvent` in `core.go`:

| Scope    | Subject Pattern                  | Delivered To                                                       |
| -------- | -------------------------------- | ------------------------------------------------------------------ |
| `user`   | `live.server.user.{userId}.*`    | Only that user (private events; `profile_updated` is broadcast)    |
| `space`  | `live.server.space.{spaceId}.*`  | All authenticated users (server membership is implicit)            |
| `config` | `live.server.config.*`           | All authenticated users (server config is public)                  |
| `member` | `live.server.member.{verb}`      | All authenticated users (server-level membership lifecycle)        |

Room events (`live.server.room.{kind}.{roomId}.…`) are filtered separately in `filterLiveEvent` using the per-subscription `memberRooms` cache — they never reach `isAuthorizedForLiveEvent`.

**Adding a new event type:**

1. Add protobuf message to the appropriate `*.proto` file and a oneof case to `event.proto` (`corev1.Event`)
2. Add to GraphQL schema in `events.graphqls` (type + `ServerEventType` union)
3. Add `IsServerEventType()` method in `pb/chatto/core/v1/graphql.go`
4. Add case in `unwrapEvent()` in `event_helpers.go`
5. Publish via one of `publishServerEvent` (durable) or `publishLiveEvent` / `publishLiveServerEvent` (transient) — choose ONE
6. Subscribe in frontend via `eventBus.svelte.ts` (or a handler registered through `useEvent`)

**When to create a live event:** Any time a user action changes state that other tabs/devices or other UI components need to reflect in real-time. Common triggers:
- User changes a preference or setting (notification level, follow state)
- Server-side auto-mutations (auto-follow on posting to a thread)
- Cross-tab sync needs (reading a room in one tab should update indicators in others)

If a mutation changes state visible in the UI and you don't publish a live event, the UI will be stale until refresh. Always consider: "Will other tabs or other components on the same page need to know about this change?"

**Broadcasting user events to everyone**: By default, user-scoped events are private (only delivered to that user). To broadcast an event to all authenticated users (e.g., profile updates since profiles are public), add an explicit check in `isAuthorizedForLiveEvent`:

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
- REST endpoints are acceptable only for: OAuth callbacks, webhooks, health checks, and pre-auth discovery (e.g., `GET /api/instance` for multi-server client probing before GraphQL setup; the URL is unchanged for now)

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
