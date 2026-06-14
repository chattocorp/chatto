# Proto Compatibility

The event-sourcing rollout is complete and the pre-0.1 boot importers are gone,
but protobuf wire compatibility still matters because `EVT`, `RUNTIME_STATE`,
`ENCRYPTION_KEYS`, and other JetStream-backed resources persist protobuf
payloads across deploys, backups, and restores.

Rules:

- Do not renumber fields on any proto message that is persisted in JetStream
  streams, KV buckets, or object metadata.
- Do not change a field's type at an existing tag. Add a new tag instead.
- Removing a field requires both `reserved <tag>` and `reserved "<name>"`
  unless the field was never persisted.
- Renames are wire-safe but code-breaking; keep them scoped and update all
  generated consumers in the same change.
- For persisted records, prefer additive schema evolution and explicit repair
  or migration code when existing data must change shape.
- Transient-only live event protos are less stable than persisted records, but
  still consider GraphQL/API behavior and mixed-version clients before changing
  their wire shape.
