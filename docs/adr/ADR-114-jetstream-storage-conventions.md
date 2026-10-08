# ADR-114: JetStream Storage Conventions

**Date:** 2026-10-08

**Status:** Accepted

## Context

Chatto and Authling keep their data in NATS JetStream
([ADR-001](ADR-001-nats-jetstream-as-primary-data-store.md),
[ADR-073](ADR-073-define-the-loom-architecture.md)). Earlier decisions and
`cli/AGENTS.md` tell contributors how to read and write safely: optimistic
concurrency control (OCC), leader reads, watcher filters, and durable consumer
lifecycle. [ADR-036](ADR-036-runtime-state-kv-boundary.md) and
[ADR-084](ADR-084-separate-internal-protobufs-by-storage-contract.md) tell
contributors which bucket and which protobuf package a record belongs to.

No decision told contributors how to shape new storage. An audit of the
current code found these results:

- `RUNTIME_STATE` holds JSON, protobuf, and raw binary values side by side.
  Most credential and workflow records are JSON, although ADR-084 lists
  `RUNTIME_STATE` as the home of `chatto.core.runtime_state.v1` records.
  `MEMORY_CACHE` also mixes protobuf and JSON.
- Most resource names were string literals in many files.
- `chatto keys import` had its own copy of the `ENCRYPTION_KEYS`
  configuration.
- The backup skip list, the test reset, and the provisioning code each had
  their own resource list. The lists did not agree.
- The operator diagnostics did not report one durable consumer.
- Some code classified OCC conflicts with `jetstream.ErrKeyExists` only, and
  many files had their own copy of the "key is absent" check.

Servers in use have data in all of these shapes. Replicas of different
versions can run at the same time during a rolling upgrade. Thus, a cleanup
must not change data that is already stored.

## Decision

### Rules for each Loom application

These rules apply to Chatto and Authling. Each application applies them in
its own code and records its exceptions in its own documentation.

1. **Persisted contracts.** Resource names, resource configurations, stream
   subjects, key shapes, value encodings, and durable consumer names are
   persisted contracts. Do not change one of them as a refactor. A change
   needs a compatibility decision and a migration that is safe when replicas
   of different versions run at the same time. For a value encoding, first
   release a version that reads both encodings, then release a version that
   writes the new encoding.
2. **Use an existing resource first.** Add a stream, key-value bucket, or
   Object Store only when the data needs a lifecycle, retention, storage type,
   or backup policy that no existing resource gives. ADR-073 requires an
   explicit reason for a secondary event log. Record each new resource in an
   ADR or in the architecture inventory.
3. **Declare each resource fact once.** Declare the name, the configuration,
   and the backup policy of each resource in one place each, for example a
   registry for names and backup policies and one configuration function for
   each resource. The server, operator commands, backup, restore, and test
   resets use these declarations. A test pins every configuration, because
   each replica updates the resource to its own configuration at startup.
4. **Encode new records as protobuf.** A new record type uses protobuf from
   the package that names its storage contract (ADR-084). Do not add new JSON
   record types. A storage protobuf does not import a public API protobuf.
   Existing JSON and raw binary records stay as they are until a planned
   migration replaces them.
5. **Build keys in one place.** Put the record-family prefix first. Then put
   owner identifiers from the widest to the narrowest scope, separated by
   dots. Each key family has one builder in the model that owns it, and
   watcher filters come from the same prefix constant. A key contains secret
   input only as a keyed hash. Each application selects one letter case for
   prefixes and one hash encoding, and uses them for all of its key families.
6. **Classify errors with shared helpers.** Classify OCC conflicts with
   `jetstreamutil.IsSequenceConflict`. After a conflict from `Create`, read the
   key again before you use the stored record, because a write in progress can
   still fail. A raw create-once publish is different: only
   `jetstream.ErrKeyExists` proves that the record is stored. A key-value
   read reports a missing or removed key as `jetstream.ErrKeyNotFound`, so
   that check is sufficient. Do not add checks for `jetstream.ErrKeyDeleted`.
7. **Report every durable consumer.** Declare each durable consumer name once
   and include it in the operator diagnostics.

### Chatto

- `cli/internal/natsresources` is the resource registry. It holds each
  resource name, kind, and backup policy. It also lists legacy buckets that
  earlier versions created. Nothing deletes these buckets, so upgraded servers
  can still have them, and backups must continue to skip them.
- `cli/internal/core/storage.go` holds one configuration function for each
  resource. `TestStorageConfigsArePinned` pins each configuration.
  `TestNewChattoCoreProvisionsExactlyTheRegisteredResources` makes sure that
  core creates exactly the registered resources.
- `durableWorkerDiagnosticSpecs` lists each durable consumer.
  `TestDurableWorkerAdminStatusesCoverEveryCoreConsumer` makes sure that the
  list includes each durable consumer that core creates.
- Key prefixes use snake_case. Keys that contain secret input use the hex
  HMAC from `runtime_token_keys.go`.
- Pass the bound `*jetstreamutil.KeyValue` handle in production code. A
  model can accept the `jetstream.KeyValue` interface so that tests can inject
  faults, but production wiring must pass the bound handle.

These known exceptions stay as they are, because they are persisted
contracts:

- JSON credential, token, workflow, upload, and push-owner records in
  `RUNTIME_STATE`, as listed in the
  [runtime state inventory](../architecture/runtime-state.md).
- Raw binary read markers and notification boundaries in `RUNTIME_STATE`.
- JSON worker leases and call reconciliation state in `MEMORY_CACHE`.
- `runtime_state.v1.PresencePreference` uses the public
  `chatto.api.v1.PresenceStatus` enum.
- LiveKit call E2EE keys use the `key_material.v1.UserKeyEncryptionKey`
  message.
- Push subscription and push endpoint owner keys use an unkeyed SHA-256 hash
  of the push endpoint. The subscription key uses only the first 8 bytes.

This ADR does not decide whether runtime records must be encrypted at rest.

## Consequences

New storage has one shape, and reviewers can check new storage against these
rules. A new resource that is not in the registry, a configuration change, or
a durable consumer that the diagnostics do not report makes a test fail.

The rules do not remove the existing exceptions. Contributors must not copy an
exception into new code. A planned migration can remove an exception later.

Each application keeps its own key style, so Chatto and Authling keys can look
different. The rules require consistency in one application, not between
applications.

## Related

- [ADR-001](ADR-001-nats-jetstream-as-primary-data-store.md)
- [ADR-036](ADR-036-runtime-state-kv-boundary.md)
- [ADR-069](ADR-069-explicit-durable-consumer-lifecycle.md)
- [ADR-073](ADR-073-define-the-loom-architecture.md)
- [ADR-084](ADR-084-separate-internal-protobufs-by-storage-contract.md)
