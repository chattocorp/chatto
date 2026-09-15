# FDR-001: Standalone Account Runtime

**Status:** Experimental
**Last reviewed:** 2026-09-15

## Overview

Authling can run its first durable domain model as a standalone process. This
foundation gives operators a simple local storage option and gives signup and
identity features an opaque account lifecycle that survives process restarts.
FDR-002 extends it with verified-email signup; this record remains the contract
for the underlying structural runtime.

## Behavior

- Operators start the runtime with the `authling run` command.
- Configuration comes from `authling.toml` with `AUTHLING_*` environment
  variables taking precedence.
- Unknown configuration fields and invalid deployment combinations prevent
  startup.
- Simple single-process deployments may opt into a private embedded NATS
  server whose data persists in a configured directory.
- External NATS deployments must provide credentials for Authling's dedicated
  NATS account.
- Authling becomes ready only after all required projections and the browser
  session inventory have replayed their startup state, and issuer and OIDC
  initialization have succeeded.
- Once ready, Authling serves its browser identity flows, OpenID Connect
  endpoints, and embedded assets from the configured HTTP listener.
- Creating an account produces an opaque account identifier and is visible to
  the creating operation only after the local model reflects the committed
  fact.
- Accounts are restored from durable history after a full process and embedded
  storage restart.

## Design Decisions

### 1. Begin with structural accounts

**Decision:** The first account fact contains only an opaque account identifier
and creation time.

**Why:** This exercises the event-sourced lifecycle from ADR-001 without
prematurely introducing email addresses, password verifiers, or encryption-key
workflows.

**Tradeoff:** A structural account alone has no public creation path. Verified
email signup adds the credential and key hierarchy needed for local login.

### 2. Make embedded storage opt-in

**Decision:** Operators explicitly choose either private embedded NATS or a
credentialed external NATS account.

**Why:** Embedded storage keeps simple self-hosted deployments approachable,
while an explicit choice avoids silently creating production data in an
unexpected local directory.

**Tradeoff:** A new installation needs configuration before it can start.

### 3. Gate readiness on replay

**Decision:** The process does not report readiness until all required
projections, the browser session inventory, and issuer and OIDC initialization
are ready.

**Why:** Serving from a partial identity model would make absence and
uniqueness decisions unsafe.

**Tradeoff:** Startup time grows with retained history until safe projection
snapshots are implemented.

## Related

- **ADRs:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md),
  [ADR-003](../adr/ADR-003-server-rendered-templ-ui.md),
  [Root ADR-058](../../../docs/adr/ADR-058-application-neutral-embedded-nats-runtime.md),
  [Root ADR-061](../../../docs/adr/ADR-061-application-neutral-configuration-loading.md)
- **Features:** [FDR-002](FDR-002-verified-email-signup.md),
  [FDR-003](FDR-003-local-login-and-browser-sessions.md)

## Open Questions

- Which operator-facing diagnostics and health interfaces should expose
  readiness?
