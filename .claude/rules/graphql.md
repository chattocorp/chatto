---
paths: ["**/*.graphqls", "cli/internal/graph/**", "frontend/src/lib/gql/**"]
---

# GraphQL Development

gqlgen is schema-first. Follow this workflow for GraphQL changes:

1. Update schema files (`*.graphqls`) first
2. Run `mise codegen-cli` to regenerate Go types/resolvers
3. After frontend query changes, run `mise codegen-frontend`

## Schema Documentation

- Every type must have a type-level description
- Every field must have a description, even "obvious" relationship fields
- Every enum value must have a description explaining its meaning
- Descriptions should be concise (one sentence preferred)
- Include format examples for non-obvious string values (e.g., `"Round-trip time (e.g., '1.234ms')."`)
- Schema descriptions are user-facing API documentation. Do not mention backend implementation details like KV buckets, streams, internal projections, storage names, migration paths, or webhook plumbing unless they are intentionally part of the public contract.

## Schema Directives

Use gqlgen directives to control code generation:

### `@public`

GraphQL fields require authentication by default. Add `@public` only to fields
that intentionally allow anonymous callers. In practice this should be limited
to server identity, branding, and login metadata needed before the client has
attached an authenticated server session.

Do not add `@public` to fields that expose user, room, message, admin,
mutation, viewer-scoped, permission, or capability data, even if the resolver
would return `null`, `false`, or an empty collection for anonymous callers. Keep
permission checks, room membership checks, and self-vs-target rules in resolver
helpers where the resolver has the needed context; `@public`
only controls the anonymous/authenticated boundary.

```graphql
type Query {
  server: Server! @public @goField(forceResolver: true)
  viewer: Viewer # Authenticated by default
  user(userId: ID!): User # Authenticated by default
}
```

### `@goField(forceResolver: true)`

Add this to fields that have custom resolvers. This:

- Silences "adding resolver method for X, nothing matched" warnings
- Documents that the field requires a resolver (not auto-bound from protobuf)
- Required for fields that are computed, lazy-loaded, or need authorization

```graphql
type Space {
  id: ID! # Auto-bound from protobuf
  rooms: [Room!]! @goField(forceResolver: true) # Requires resolver
  viewerIsMember: Boolean! @goField(forceResolver: true) # Computed field
}
```

### `@goModel(model: "...")`

Bind a GraphQL type to a specific Go type:

```graphql
scalar Time @goModel(model: "hmans.de/chatto/internal/graph.Time")
```

## Unions vs Interfaces

**Prefer unions over interfaces** for polymorphic types. This project uses unions (like `EventType`, `NotificationItem`) rather than interfaces:

- **Union**: Simpler Go models - only need `IsTypeName()` marker method
- **Interface**: Requires getter methods for all shared fields in Go

With unions, clients check `__typename` and use inline fragments to query fields:

```graphql
# Union requires inline fragments for ALL fields (including shared ones)
query {
  notifications {
    __typename
    ... on DMMessageNotificationItem {
      id
      createdAt
      actor {
        id
      }
      summary
      room {
        id
      }
    }
    ... on MentionNotificationItem {
      id
      createdAt
      actor {
        id
      }
      summary
      space {
        id
      }
      room {
        id
      }
    }
  }
}
```

## Pagination

The schema uses three pagination shapes today, each chosen for its access pattern. Match the shape to the use case rather than reaching for one universal style:

| Shape | Arguments | When to use | Example |
| --- | --- | --- | --- |
| **Opaque cursors** | `limit: Int`, `before: String`, `after: String` | Long, append-only timelines where the client needs to walk both directions. Returns a `*Connection` type with `startCursor` / `endCursor` / `hasOlder` / `hasNewer`. | `Room.events`, `Room.eventsAround` |
| **Offset + total** | `search: String`, `limit: Int`, `offset: Int` | Admin/directory listings where the total count drives UI (`N members · page 2 of 4`). Returns a `*Connection` type with `totalCount` and `hasMore`. | `Server.members` |
| **First-N preview** | `first: Int` only | Capped previews that never paginate ("first 5 thread participants", "first 3 reactions"). Returns a bare list; no cursor or count. | `FollowedThread.threadParticipants`, `MessagePostedEvent.threadParticipants` |

**Heuristic when adding a new paginated field:**

1. Is the client ever going to display the total? → offset + total.
2. Is the underlying source append-only and potentially long (event streams)? → cursors.
3. Is this a small fixed-size preview the client will never page past? → `first` only.

Don't introduce a fourth shape without weighing this list first. If a use case genuinely doesn't fit, add the new shape here so the rule keeps reflecting reality.

## Custom Model Files

When gqlgen's auto-binding doesn't work (e.g., types needing internal fields for resolvers), create custom models in `cli/internal/graph/model/`. Keep these minimal:

```go
type MyCustomType struct {
    ID      string `json:"id"`
    // Internal fields for resolvers (not exposed in GraphQL)
    ActorID string `json:"-"`
}

func (MyCustomType) IsMyUnion() {}  // Union marker method
```

## Nullability Must Match Resolver Failure Mode

A `!` (non-null) field is a promise the resolver can always produce a value. If the resolver does a *lookup* — projection, KV, dataloader — that can legitimately return "not found" under normal lifecycle, the field MUST be nullable. Otherwise a single missing reference blanks the entire enclosing structure.

The chain-of-non-null trap: `roomId: ID!` on an event in a `[RoomEvent!]!` connection. The resolver errors when the referenced asset has been deleted; GraphQL can't null the `ID!`, so it propagates to `event: RoomEventType!`, then to the `RoomEvent` itself, then to the non-null list, then to the connection. One stale reference → empty channel. This is silent on the client unless you read the `errors` array, which we don't surface in the UI.

**Rules:**

- **Derived fields whose lookup can decay → nullable.** If the field models a *reference to something that can be deleted independently* (asset behind an event, user behind a deleted account, room behind a notification), declare it `Foo` not `Foo!`. Intrinsic identity that lives on the payload itself (an event's own id, a known field on the proto) can stay non-null.
- **Don't return errors for expected absence.** A `not found` result from a projection / KV is a normal lifecycle state, not a failure. Return nil / zero / empty string and let the nullable field carry the absence. Reserve resolver errors for auth denials, validation, and infrastructure faults — things the client genuinely cannot proceed past.
- **Helpers shouldn't return `(*T, error)` for "missing" either.** A helper like `assetCreationForProcessing(id)` that returns nil-or-found is easier to use safely than one that returns nil-and-error; the caller can't accidentally promote the absence into a non-null violation.
- **When in doubt, look at the list.** Any `[T!]!` in the chain above the field amplifies a single resolver error into a total list wipeout. If even one element's resolver can fail, the safer schema is `[T!]` or `[T]!` — but better still is the nullability/no-error discipline above so the failure never reaches GraphQL.

## Type Compatibility

When autobind can't match protobuf types to GraphQL types, you'll see warnings like:

- `Time is incompatible with *timestamppb.Timestamp` → Use custom scalar with `@goModel`
- `ID is incompatible with uint64` → Add resolver to convert types
- `adding resolver method for X, nothing matched` → Add `@goField(forceResolver: true)`

## Optimistic UI vs Backend Enforcement

For permission-based UI gating (e.g., "can viewer manage this user?"):

- **Frontend handles optimistic checks** using locally available data (permissions, roles, membership status)
- **Backend enforces authorization** on mutations - the actual security boundary

Don't add `viewer*` boolean fields that require backend round-trips when the frontend already has the data to compute them. Instead:

| ❌ Avoid                                 | ✅ Prefer                              |
| ---------------------------------------- | -------------------------------------- |
| `SpaceMember.viewerCanManage: Boolean!`  | Frontend computes from already-loaded permissions/roles |
| Fetching permissions for every list item | Query permissions once, apply locally  |

Backend `viewer*` fields are still useful for:

- Complex authorization logic the frontend can't replicate
- Fallback when local data isn't available
- API-level authorization (e.g., `Space.viewerCanManageUser(userId)` for mutations)

## Don't Duplicate Permission Checks Across Resolvers

When the same resolver-side merge appears in two places that ought to behave identically (e.g. unified room lists that append the caller's DM rooms), extract the shared behavior into a helper in `*_helpers.go`. Each resolver is then a one-liner; membership-filtered DM listing can't drift between GraphQL entry points.

This bit us once in #330 phase 3 when two room-list resolvers had subtly different DM gates. Since ADR-037, DMs do not have a read permission: room membership is the read boundary, and message permissions gate starting/sending DMs. Keep resolver helpers aligned with that split.

General principle: if you find yourself copy-pasting an authorization branch between resolvers, that's the moment to extract — before the copies drift.

## Prefer Core Types Over Wrapper Types

Avoid creating wrapper types that just add fields to existing types. Instead, add scoped fields to the core type:

| ❌ Avoid                                         | ✅ Prefer                                         |
| ------------------------------------------------ | ------------------------------------------------- |
| `SpaceMember { user: User!, roles: [String!]! }` | `User.spaceRoles(spaceId: ID!): [String!]!`       |
| `Space.members: [SpaceMember!]!`                 | `Space.members: [User!]!` with `spaceRoles` field |

Benefits:

- Simpler schema with fewer types
- Consistent data shape - a User is always a User
- Easier caching and normalization in clients
- Fields can be queried in any context where the User is available

## Fragment Type Assertions

When using gql.tada/graphql-codegen with fragments, the generated TypeScript types use `$fragmentRefs` markers that don't expose fields like `id` directly. The fields exist at runtime, but TypeScript requires assertions:

```typescript
// TypeScript error: Property 'id' does not exist on type '{ __typename?: "User" } & { $fragmentRefs?: ... }'
const actorId = participant.id;

// Works - field exists at runtime, assertion satisfies TypeScript
const actorId = (participant as { id?: string })?.id;
```

This commonly occurs when working with fragment wrapper types from subscriptions or when comparing actors/participants by ID.
