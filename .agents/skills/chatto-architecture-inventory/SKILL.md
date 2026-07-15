---
name: "chatto-architecture-inventory"
description: "Maintain Chatto's current runtime architecture inventory in docs/architecture/. Use when adding, changing, removing, auditing, or documenting runtime components, projections or snapshots, NATS and JetStream resources, subject namespaces and event types, runtime-state keys, durable effects, ConnectRPC or Operator API mounts, or realtime delivery architecture. By default, inspect the codebase, apply all in-scope inventory fixes, and validate them; report without editing only when the user or parent workflow explicitly requests an audit, review, report, or propose-only result."
---

# Maintain the Chatto Architecture Inventory

Keep `docs/architecture/` aligned with the current codebase. Record what exists,
where it lives, who owns it, and its current operational contract.

## Documentation boundary

- Put current runtime facts in `docs/architecture/`.
- Put cross-cutting rationale, alternatives, and consequences in ADRs.
- Put feature behaviour and feature-specific design in FDRs.
- Put canonical definitions in `docs/GLOSSARY.md`.
- Put public RPC, frame, and message details in protobuf comments and generated
  API documentation.
- Put deployment procedures and configuration tutorials on the docs website.

Link relevant ADRs and FDRs from inventory files without restating their
rationale. Keep compatibility facts only while they constrain current reads,
writes, cleanup, mixed-version operation, or stored data.

## Action mode

Use **maintenance mode by default**. Requests such as "use this skill",
"refresh the architecture inventory", "update the architecture docs", or
"make the inventory current" authorize edits to the relevant documentation.
In maintenance mode:

1. Find in-scope drift.
2. Fix every in-scope inventory omission, stale entry, broken link, and
   misplaced explanation that can be resolved from repository evidence.
3. Validate the resulting files.
4. Finish with a concise summary of changes and checks, not a list of work the
   user still needs to ask the agent to perform.

Do not stop after identifying missing items. Findings are an intermediate work
list, not the maintenance-mode deliverable. Do not ask for approval before
ordinary inventory edits.

Use **report-only mode** only when the user explicitly asks for findings,
proposals, or an audit/review/check without changes, or when a parent workflow
explicitly says it is propose-only. An explicit request to fix, apply, update,
or refresh takes precedence over audit/review wording. In report-only mode,
make no edits and return the findings with suggested actions. For example,
`chatto-checkup` is intentionally propose-only and overrides this skill's
default maintenance mode.

Inventory maintenance does not authorize unrelated implementation changes. If
repository evidence reveals a likely code defect or a decision that requires a
new ADR/FDR, make the inventory accurately describe the current runtime, then
report the separate issue instead of silently changing product behaviour.

## Start here

1. Read `docs/architecture/INDEX.md`.
2. Identify the categories touched by the task or diff.
3. Read only the matching inventory files and authoritative sources below.
4. Read related ADRs or FDRs only when needed to preserve an existing contract.
5. Update the affected inventory files in place.

Do not load every inventory file for a category-scoped change. Perform a full
inventory audit only when the user explicitly requests an architecture audit,
checkup, or complete refresh.

## Category routing

### Runtime components

File: `docs/architecture/runtime-components.md`

Use `cli/internal/core/core.go`, `cli/internal/core/*_model.go`, runtime-unit
wiring, and worker/service constructors to inventory current models, facades,
publishers, and long-running components. Record stable diagnostic keys,
ownership, lifecycle, and responsibilities.

### Projections and snapshots

File: `docs/architecture/projections.md`

Use `NewChattoCore`, projector constructors, `Subjects()` methods,
`cli/internal/core/projection_subjects_test.go`, and snapshot codecs/storage
wiring. Inventory registered parent projectors, logical subject filters, nested
read models, primary readers, and snapshot support. Do not list nested read
models as independently registered projectors.

### NATS resources

File: `docs/architecture/nats-resources.md`

Find current `CreateOrUpdateStream`, `CreateOrUpdateKeyValue`, and
`CreateOrUpdateObjectStore` calls. Record resource name, type, storage,
retention/TTL role, backup status, and owner. Do not reintroduce retired
resource inventories merely because old backups or compatibility readers can
contain them.

### Subjects and events

File: `docs/architecture/subjects-and-events.md`

Use `cli/internal/events/subjects.go`,
`cli/internal/core/subjects/subjects.go`, durable event protobufs, and live
publishers. Inventory envelope boundaries, subject grammar, aggregate families,
durable event-token-to-protobuf mappings, and transient/live roots. Existing
durable protobuf field numbers and subject tokens are persistence contracts.

### Runtime state

File: `docs/architecture/runtime-state.md`

Search the owning models for KV `Get`, `Put`, `Create`, `Update`, `Delete`, and
watch calls and for Object Store or S3 key construction. Record key/object
shape, encoded value, owner, TTL, persistence, backup status, and security
properties. Preserve an explicit unavailable state where operational data can
be absent; do not document missing data as a healthy zero value.

### Durable effects

File: `docs/architecture/durable-effects.md`

Inventory work that crosses from a durable fact into another store or external
service. For each effect, record its durable trigger or invariant, immediate
execution, restart and multi-replica recovery, idempotency boundary, and known
gap. Verify claims against worker, lease, cursor, retry, and focused test code.

### Interfaces

File: `docs/architecture/interfaces.md`

Use `cli/internal/connectapi/api.go`, HTTP mount code, and public proto service
declarations. Inventory transports, packages, mounted services, service-level
auth policy, public versus Operator listener boundaries, reflection, and
exceptional CORS/GET behaviour.

Do not maintain a per-RPC table here. Individual methods, request/response
shapes, and method documentation belong in protobuf comments and the generated
docs website API reference. Instead, compare declared public services with the
handlers returned by `API.Handlers()` and `API.OperatorHandlers()`. In
maintenance mode, fix inventory drift immediately; report a suspected
source-registration defect separately when correcting it would require an
implementation decision.

### Realtime delivery

File: `docs/architecture/realtime-delivery.md`

Use the realtime proto, HTTP handler, `MyEventsModel`/`MyEventsHub`, presence
fanout, and client event bus. Inventory the handshake and transport only at the
level needed to explain server architecture; focus on ingress roots,
classification, projection waits, authorization, queue/failure behaviour, and
projected-read catch-up. Leave exhaustive frame documentation in protobuf
comments and generated API docs.

## Editing rules

- Keep each file independently understandable but avoid repeating another
  category's tables or prose.
- Prefer compact tables for repeated facts and short prose for invariants.
- Add 2-5 authoritative relative source links near the start of each file.
- Use repository terminology and update `docs/GLOSSARY.md` if canonical
  vocabulary changes.
- Remove stale facts in place; do not append correction notes.
- Keep `docs/ARCHITECTURE.md` as a short compatibility landing page.
- Update `docs/architecture/INDEX.md` when adding, removing, or renaming a
  category.

## Validation

Run checks proportional to the categories touched:

- Verify every documented source link exists.
- Compare runtime components and registered projections with `NewChattoCore`.
- Compare projection filters with `projection_subjects_test.go`.
- Compare current streams, KV buckets, and Object Stores with creation calls.
- Compare durable event tokens with concrete persisted event protobuf variants.
- Compare transient subjects with live subject helpers and publishers.
- Compare mounted service names and auth policies with `API.Handlers()` and
  `API.OperatorHandlers()`; do not reconstruct an endpoint reference.
- Verify relevant ADR/FDR links and `docs/architecture/INDEX.md` navigation.
- Run `mise license-check` when files were added or moved across license
  boundaries.

Report exactly which categories and checks were covered. Never describe a
category-scoped refresh as a full architecture audit.

In maintenance mode, completion means the affected inventory files have been
edited and validated and no known in-scope documentation drift remains. A
findings-only response is incomplete unless report-only mode applies.
