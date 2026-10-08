# ID interning

`hmans.de/chatto/pkg/events/intern` stores opaque strings behind compact
`ID[Kind]` handles backed by `uint32`. It is an optional projection utility with only standard
library imports. It does not import the parent `events` package or NATS.

## Choose a table

- Use `Table[Kind]` when one model owns the IDs and supplies its own lock. Its zero
  value is ready for use.
- Use `NewConcurrentTable[Kind]()` when models with independent locks share IDs.
  The table synchronizes insertion and lookup. Its zero value is not ready
  for use. Reads from a published handle do not take a lock.
- Use `Arena` when a model needs compact string storage without a lookup
  index or deduplication. Its zero value is ready for use. Serialize `Add`
  calls and synchronize publication of locations to readers.

Both tables provide `Intern`, `Lookup`, `Resolve`, `Len`, and `EstimatedBytes`.
`Arena` provides `Add`, `String`, and `EstimatedBytes`; `Location.Len` reports
the stored byte length. Estimates are diagnostic approximations.

```go
type AccountKind struct{}
var ids intern.Table[AccountKind]
handle := ids.Intern("account-42")
counts := map[intern.ID[AccountKind]]int{handle: 3}

if handle, found := ids.Lookup("account-42"); found {
    fmt.Println(ids.Resolve(handle), counts[handle])
}
```

## Ownership and lifetime

Declare distinct named kind types for separate namespaces. Go rejects
assignment or resolution of an `ID[UserKind]` through a `Table[RoomKind]`.
Kinds take no space in the ID. Every ID is a four-byte, pointer-free integer;
zero checks, map keys, and dense slice indexing need no wrapper allocation.
Conversion from a string remains explicit at the model boundary.

Kinds do not identify table instances. Two tables with the same kind can issue
the same numeric handle for different strings. Keep each ID with its owning
table. Explicit numeric conversions can bypass type checking and must be
limited to indexing the owning table's dense records. There is no automatic
string conversion or global registry.

Applications choose table scope and sharing. A table contains no model state
or replay position. An ID can exist in a shared table before a model has
processed it. Check the model's own state to determine whether it exists there.

Zero means the empty string. Other handles and locations belong only to the
table or arena that issued them. Do not copy a `ConcurrentTable` or `Arena`
after first use. A copy of a `Table` shares storage with the original. After a
copy, use only one of the two values.
Do not construct handles or locations from arbitrary numbers.

Storage is append-only. Entries are never removed and handles are never
reused. Release the complete table when its owning models no longer need it.
Returned arena strings keep their bytes alive even after the arena is released.
Do not store secrets or personal data that must be erased during this lifetime.

Tables support at most `2^32 - 1` nonempty IDs. The concurrent table and arena
also limit each ID to `2^24 - 1` bytes and the arena to `2^24` chunks. An
insertion that exceeds a limit panics. Callers must validate untrusted input
before insertion. The local table has no separate ID-length limit.

## Snapshots

Store strings in snapshots and intern them again during restore. Handles and
locations are process-local implementation details. Restore order can produce
different handles for the same strings. Sort by the resolved strings when a
snapshot requires deterministic ordering.

The package supplies no snapshot codec, projector, persistence, or background
worker. These remain application choices.
