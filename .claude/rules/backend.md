---
paths: ["cli/**"]
---

# Backend Development

## ⚠️ DEPLOYMENT TOPOLOGY — READ THIS BEFORE DESIGNING ANY MUTATION ⚠️

**Chatto is designed to run as multiple processes in parallel.** It can also run as a single process (embedded NATS, dev mode), but multi-process is the deployment model that constraints must satisfy. Implications you MUST internalise:

- **Never rely on process-local serialization for correctness.** In-process mutexes, single-goroutine writers, or "the manager owns this" patterns are NOT sufficient to enforce cross-cluster invariants. Two replicas can both pass any in-process check.
- **Atomicity and uniqueness MUST come from NATS primitives.** Use JetStream OCC (`Nats-Expected-Last-Sequence`, `Nats-Expected-Last-Subject-Sequence`, optionally with `Nats-Expected-Last-Subject-Sequence-Subject` for wildcard filters via `WithExpectLastSequenceForSubject(seq, "subject.filter.>")`) or KV's atomic `Create` / revision-based `Update`. These are cluster-global.
- **Any read can race with a concurrent write from another process.** A projection read followed by a publish is a TOCTOU window; close it with OCC, not with a lock.
- **No "single-writer" assumptions.** Every aggregate may have N concurrent writers across replicas. Design for that.

## ⚠️ NATS IS THE PRIMARY DATA STORE — NOT A MESSAGE BUS ⚠️

NATS JetStream KV buckets and event streams hold Chatto's persisted state. NATS is not "just" a pubsub layer, and it is not an "eventually-consistent cache in front of a real database." There is no other database. Treat NATS reads/writes with the same care you would treat a Postgres transaction.

## Architecture

- `ChattoCore` handles all domain operations (spaces, users, rooms, messages)
- `EVT` is the durable source of truth for event-sourced domain state; `RUNTIME_STATE` holds persisted latest-value runtime state
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
- **Single durable EVT stream**: Event-sourced domain facts live in `EVT`. Legacy `SERVER_EVENTS` is opened only when present for pre-ES imports and inspection; new runtime writes must not mirror to it.

## Room Event Query Behavior

`GetRoomEvents` uses three optimized code paths based on room size and query type:

- **Small room fast path**: Uses `stream.Info(WithSubjectFilter)` to check total event count. If ≤ `limit`, fetches everything in one consumer with `DeliverAllPolicy`
- **Initial load (large rooms)**: Uses `GetLastMsgForSubject` (O(1) lookup) to find the room's last event sequence, then starts a consumer near the end using `DeliverByStartSequencePolicy` with progressive multipliers (3×, 10×, 50×). Falls back to `DeliverAllPolicy` if needed
- **Pagination (large rooms)**: Uses `beforeTime` as the cursor. Tries a single 30-day window, falling back to `DeliverAllPolicy` if insufficient events are found

**Important**: `room_last_msg_at` in the RUNTIME bucket only tracks MESSAGE timestamps, not join/leave events. This is intentional for sorting by "recent activity" (conversations with recent messages). Don't rely on this field to determine when the most recent _event_ occurred.

## Event Patterns

Event subscriptions are unified in `StreamMyEvents`, which consumes NATS Core subjects rather than holding per-connection JetStream consumers. There are two internal roots:

- `live.sync.>` — transient `corev1.LiveEvent` pubsub signals with no stream storage.
- `live.evt.>` — raw singleton republish of committed EVT facts.

`live.evt.>` is not UI-safe by itself. `StreamMyEvents` reads the republished JetStream sequence, waits for the relevant local projections, applies per-user authorization, and only then emits the GraphQL event.

- **Durable legacy events**: Do not add new `server.>` publishers. `SERVER_EVENTS` is legacy storage/import infrastructure only and does not participate in live delivery.
- **Durable EVT events**: For event-sourced aggregates, publish to `evt.>` via `EventPublisher`. JetStream republish automatically wires them into `live.evt.>`; `StreamMyEvents` is responsible for projection catch-up and authorization before GraphQL delivery.
- **Transient events**: For real-time UI updates where latest-value runtime state is authoritative (typing, notification sync, preference sync, user/config notifications). Publish a `corev1.LiveEvent` directly via NATS Core through `publishLiveEvent()` on `live.sync.>`. No stream storage.
- **Event-sourced room edits/retracts**: Message edits and retractions use the canonical durable `MessageEditedEvent` / `MessageRetractedEvent` shapes. `myEvents` receives them from `live.evt.>` after projection catch-up; do not synthesize legacy `MessageUpdatedEvent` / `MessageDeletedEvent` for new delivery.
- **Do not publish from projectors**: Projectors run locally in every Chatto replica. Publishing live events from `Projection.Apply` would multiply one committed EVT event by the number of replicas. Use stream `RePublish` for the raw EVT feed and let `StreamMyEvents` handle readiness/auth.
- **Do not double-publish.** Publishing the same conceptual event via BOTH `EventPublisher` and `publishLiveEvent` will deliver it twice to subscribers if the event is deliverable from `live.evt.>`. Durable facts belong in EVT; transient sync signals belong in LiveEvent.
- **Projection subjects default broad.** A projection should generally subscribe to the aggregate namespace it owns (for example `evt.user.>` for `UserProjection`, `evt.rbac.>` for `RBACProjection`, or `evt.room.>` for room-derived projections), plus any extra subjects from other aggregates that it also needs. This preserves the projection contract that `Apply` ignores unknown events and avoids subtle maintenance bugs where a new event is added to `Apply` but forgotten in `Subjects`. Limit subscriptions to individual event-type subjects only for very focused projections where the subscribed subjects are intentionally sufficient, such as a tiny derived index over a stable event set.
- **Adding new event types** requires:
  1. Core: choose durable legacy, durable EVT, or transient and publish to the appropriate subject family.
  2. Authorization: room events are gated by membership in `filterLiveEvent`; user/config/member events go through `isAuthorizedForLiveEvent`. New EVT room event variants must be added to the live-EVT deliverability/readiness switch if they should reach `myEvents`.
  3. GraphQL: add a case to `unwrapEvent` in `event_helpers.go` so the typed variant flows through `myEvents`. Missing this case causes the event to silently fail at the GraphQL layer.
- **Avoid fan-out on publish**: When broadcasting to many users, do NOT iterate and publish per-recipient. Publish once to a scoped subject (e.g., `live.sync.config.server_updated`) and let `isAuthorizedForLiveEvent` filter on the subscriber side.

## Live Event Authorization

Non-room live events use subject pattern `live.sync.{scope}.…` and are filtered by `isAuthorizedForLiveEvent` in `core.go`:

| Scope    | Subject Pattern                         | Delivered To                                                       |
| -------- | --------------------------------------- | ------------------------------------------------------------------ |
| `user`   | `live.sync.user.{userId}.*`             | Only that user (private events; `profile_updated` is broadcast)    |
| `config` | `live.sync.config.*`                    | All authenticated users (server config, branding, room layout — public to every member) |
| `member` | `live.sync.member.{verb}`               | All authenticated users (server-level membership lifecycle)        |

Room events (`live.sync.room.{kind}.{roomId}.…` plus deliverable EVT room facts from `live.evt.>`) are filtered separately in `filterLiveEvent` using the per-subscription `memberRooms` cache — they never reach `isAuthorizedForLiveEvent`.

**Adding a new event type:**

1. Add protobuf message to the appropriate `*.proto` file and a oneof case to `event.proto` (`corev1.Event`) for durable facts, or to `live_events.proto` (`corev1.LiveEvent`) for transient pubsub signals
2. Add to GraphQL schema in `events.graphqls` (type + `ServerEventType` union)
3. Add `IsServerEventType()` method in `pb/chatto/core/v1/graphql.go`
4. Add case in `unwrapEvent()` in `event_helpers.go`
5. Publish via `EventPublisher` for durable EVT facts or `publishLiveEvent` for transient LiveEvent signals — choose ONE conceptual delivery path
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
- REST endpoints are acceptable only for: OAuth callbacks, webhooks, health checks, and pre-auth discovery (e.g., `GET /api/server` for multi-server client probing before GraphQL setup)

## Dataloaders

- Dataloaders are injected for **HTTP requests only**, not WebSocket connections
- WebSocket connections are long-lived; dataloader caches would become stale across subscription events (e.g., user updates display name mid-session)
- Subscription resolvers fall back to direct `core.Get*()` calls via helper methods like `r.getUser()`
- This is intentional: HTTP requests benefit from batching (loading room history with many reactions), while subscription events arrive one at a time and don't benefit from batching anyway

## Security

- All GraphQL mutations must check permissions via `core.RequirePermission()`

## Known Test Issues

- `cli/internal/http_server` tests that hit `/auth/test/*` or `/webhooks/test/*` endpoints require the `test_endpoints` build tag. Plain `go test ./...` will fail `TestAuthRoutes_TestEmailEndpoint` with a 404 because those routes are compiled out. For targeted runs, use `mise x -- go test -tags test_endpoints ./internal/http_server -run TestX -timeout 30s`; for full backend checkpoints, use `mise test-cli`, which sets the tag for the suite.

## Cost Reference

Hetzner volumes €53/TB with R3 replication (3x storage)
