## Project Maturity

Chatto has public servers running with real user data. Data migrations are required for any breaking changes to storage schemas or APIs. Never assume data can be discarded.

## Breaking vs. Non-Breaking Changes

- Any changes to the `proto/` definitions are considered **SIGNIFICANT BREAKING CHANGES**. These affect compatibility with existing clients and servers, and require careful coordination for deployment.
- During the event-sourcing rollout, proto compatibility is under an extra
  import-safety lockdown. Read `.claude/rules/proto-compat.md` before
  changing `proto/`.
- Any changes to the GraphQL schema files in the `cli/internal/graph/` package should be considered **POTENTIALLY BREAKING CHANGES**. GraphQL is our primary API; at this point in the project we don't have external clients, though, but we should still tread carefully.
- Any changes to **NATS JetStream stream/KV schemas** (stream names, subject patterns, KV key formats) are **BREAKING CHANGES** that require a data migration path. Existing servers must be migrated on upgrade without data loss.
- All other changes can be considered non-breaking.
