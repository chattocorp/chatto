# Proto Compatibility During ES Rollout

Until the community-server event-sourcing rollout is complete, protobuf
wire compatibility is locked down more tightly than usual.

Boot importers read pre-ES KV records and legacy `SERVER_EVENTS` payloads
by unmarshalling them into the current generated protobuf types. The same
event protos are then written to `EVT`. A wire-incompatible proto change
between the currently deployed pre-ES binaries and the ES rollout build can
silently corrupt or drop imported data.

Rules:

- Do not renumber fields on any proto message that appears in legacy KV,
  `SERVER_EVENTS`, or `EVT`.
- Do not change a field's type at an existing tag. Add a new tag instead.
- Removing a field requires both `reserved <tag>` and `reserved "<name>"`
  unless the field was never persisted. Before reserving it, verify no boot
  importer needs to read it.
- Renames are wire-safe but code-breaking; keep them scoped and update all
  generated consumers in the same change.
- If an importer used to derive state from a removed field, reconstruct that
  state from the KV key, stream subject, or another still-present field.
- Treat importer unmarshal warnings as rollout blockers until investigated.

Lift this rule only after every live instance has booted successfully on the
ES build and the imported projections have been smoke-tested.
