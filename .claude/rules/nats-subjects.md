# NATS Subject Patterns

## Design Principles

When designing NATS subject patterns, follow these principles:

### 1. Unified Namespaces for Related Events

Group related events under a common prefix so a single wildcard subscription captures all of them:

```
# Good: All messages (root + thread) under msg.>
server.room.{kind}.{r}.msg.{eventId}                    # Root message
server.room.{kind}.{r}.msg.{rootId}.replies.{eventId}   # Thread reply

# Bad: Separate namespaces require multiple subscriptions
server.room.{kind}.{r}.msg.{eventId}                    # Root message
server.room.{kind}.{r}.thread.{rootId}.{eventId}        # Thread reply
```

### 2. Semantic Markers for Disambiguation

Use explicit semantic tokens (like `.replies.`) to distinguish subject types, rather than relying on part counts alone:

```
# Good: Clear semantic marker
msg.{rootId}.replies.{eventId}   # "replies" explicitly marks thread messages

# Less clear: Only part count differs
msg.{eventId}                    # Root (6 parts)
thread.{rootId}.{eventId}        # Thread (7 parts)
```

### 3. Hierarchical Nesting

Structure subjects so children nest under parents in the namespace:

```
# Good: Threads nest under their root message
msg.{rootId}.replies.{eventId}

# Less intuitive: Separate top-level namespace
thread.{rootId}.{eventId}
```

### 4. Live Delivery Uses Raw EVT Republish Plus an Authorized API Gate

The legacy `SERVER_EVENTS` stream is storage/import infrastructure during the
event-sourcing migration. It does not republish to a live subject, and new live
delivery must not use `live.server.>`.

Transient signals publish as `corev1.LiveEvent` on `live.sync.>` through
`publishLiveEvent`. This root is for live UI sync where runtime/KV state is
source of truth (typing, voice-call presence, notifications, preferences,
server/config invalidations) and has no stream storage.

Event-sourced aggregates write durable state to `EVT`
(`evt.{aggregateType}.{aggregateId}.{eventType}`). The stream's
`RePublish` config forwards committed events once onto `live.evt.>`.
Treat `live.evt.>` as a raw internal committed-event feed, not a
browser/client contract: GraphQL `myEvents` consumes it server-side, waits
for the local projections needed by authorization and follow-up resolvers,
then filters by the subscribing user before emitting the event.

Do not publish live events from ordinary projection `Apply` methods. Every
Chatto replica maintains its own local projectors, so projector-side publish
effects multiply one committed EVT event by the replica count.

During the remaining ES migration window, some EVT-backed mutations still
may have legacy Event-envelope adapter shapes in protobuf/GraphQL, but live
delivery should come from either `live.evt.>` or `live.sync.>`.

**Anti-pattern: do not double-publish.** Calling both `EventPublisher.Append(...)` *and* `publishLiveEvent(...)` for the same conceptual UI event can deliver it twice if that EVT fact is deliverable from `live.evt.>`. Choose one based on whether the event is durable history or transient sync.

When extending the subject parsers (`subjects.go`), accept durable (`server.>`)
and transient sync (`live.sync.>`) room shapes via the shared normalization
helper so they share one canonical form. `live.evt.>` has its own parser in the
events package. New parsers should follow the existing pattern
(`ParseRoomIDFromSubject`, `IsThreadSubject`, etc.).

### 5. Encode Filter Discriminators in the Key Prefix

When a single bucket (or stream) holds records of multiple kinds, put the kind in the key prefix so listing operations can prefix-filter without loading and deserializing every record. This applies to KV keys (which are subjects under the hood) just as much as stream subjects.

```
# Good: kind in key prefix → fast prefix scans
SERVER_CONFIG:
  room.channel.{roomId}                        # filter `room.channel.*`
  room.dm.{roomId}                             # filter `room.dm.*`
  room_membership.channel.{roomId}.{userId}    # filter `room_membership.channel.{roomId}.*`
  room_membership.dm.{roomId}.{userId}

# Less efficient: kind on the proto, not the key
SERVER_CONFIG:
  room.{roomId}             # have to load + deserialize each room to filter
  room_membership.{u}.{r}   # have to look up the room to know its kind
```

Same outer-to-inner scope ordering across related keys: `room.{kind}.{roomId}` and `room_membership.{kind}.{roomId}.{userId}` both put the kind first, then the room (the entity being described), then per-room detail. Symmetric and predictable.

The kind segment is then **the** source of truth — don't also store it on the proto. One canonical representation per piece of information.

## Filtering Patterns Reference

For room messages, these wildcard patterns enable efficient filtering:

| Pattern | Matches |
|---------|---------|
| `msg.>` | All messages (root + threads) |
| `msg.*` | Root messages only |
| `msg.*.replies.>` | All thread replies (any thread) |
| `msg.{rootId}.replies.>` | Replies in a specific thread |
| `msg.*.replies.{eventId}` | Lookup thread reply by event ID |

For kind-prefixed KV keys (`SERVER_CONFIG`):

| Pattern | Matches |
|---------|---------|
| `room.channel.*` | Channel rooms only |
| `room.dm.*` | DM rooms only |
| `room.*.*` | All rooms regardless of kind |
| `room_membership.{kind}.{roomId}.*` | Members of one room (pure prefix) |
| `room_membership.{kind}.*.{userId}` | A user's memberships of one kind (server-side wildcard) |
| `room_membership.{kind}.>` | All memberships of one kind |

## Subject Refactoring Checklist

When changing subject patterns:

1. **Update construction functions** in `subjects.go` (e.g., `RoomThread`)
2. **Update parsing functions** in `subjects.go` (e.g., `IsThreadSubject`, `ParseEventIDFromSubject`)
3. **Update all test expectations** in `subjects_test.go`
4. **Update comments** in files that reference the patterns (e.g., `rooms.go`)
5. **Update `docs/ARCHITECTURE.md`** subject tables and filtering examples
6. **Run full test suite** including e2e tests - subject changes cascade through the entire system

Subject changes are high-risk because they affect:
- JetStream stream configs and filters
- Consumer subscriptions
- `GetLastMsgForSubject` lookups
- Event routing and delivery
