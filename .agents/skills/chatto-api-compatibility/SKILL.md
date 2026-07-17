---
name: chatto-api-compatibility
description: Review or design Chatto public API and protocol changes for temporal compatibility, capability discovery, version skew, migration impact, and generated artifacts. Use whenever changing or reviewing public protobufs, ConnectRPC handlers, discovery metadata, realtime frames or behaviour, public auth/error/pagination/visibility semantics, or bundled client/server compatibility.
---

# Review Chatto API Compatibility

Apply Chatto's experimental compatibility-by-default policy without treating
the pre-1.0 `v1` namespaces as a stability guarantee.

## Source Of Truth

Read these before classifying a change:

1. `docs/adr/ADR-045-public-api-stability-tiers.md`
2. Root `AGENTS.md` public API rules
3. `proto/AGENTS.md` and the affected package-local `AGENTS.md`
4. Relevant FDRs and generated public documentation

Do not duplicate or reinterpret ADR-045 inside review output.

## Covered Surfaces

- `chatto.auth.v1`
- `chatto.discovery.v1`
- `chatto.api.v1`
- `chatto.admin.v1`
- `chatto.realtime.v1`
- ConnectRPC/realtime handler semantics and public HTTP behaviour
- Bundled web-client capability and version-skew handling

Review `chatto.operator.v1` separately as a root-equivalent local API. Treat
persisted `chatto.core.v1` messages as non-breaking storage contracts, not as
experimental public APIs.

## Workflow

1. Diff the affected source against the target branch and, for release work,
   the relevant released tag.
2. Classify every client-visible change as:
   - **Additive**: an old client can continue using prior behaviour.
   - **Behavioural**: the wire shape remains compatible but documented errors,
     authorization, visibility, validation, ordering, or lifecycle changes.
   - **Deprecated**: the old contract remains usable and a replacement exists.
   - **Breaking**: an existing client must change to retain prior behaviour.
3. Evaluate both temporal directions:
   - older client → newer server;
   - newer client → older server.
4. Prefer additive evolution when it preserves a coherent API. If a break
   materially improves the experimental design, require an explicit rationale,
   migration plan, `api-breaking-change` label, generated updates, public docs,
   and release-note guidance.
5. Check whether new client behaviour needs a stable protocol capability.
   Keep protocol support separate from server configuration and viewer
   permissions. Use software versions only as a fallback for legacy servers.
6. Check mixed bundled client/server behaviour. Missing optional support should
   degrade only the affected feature. Required skew boundaries need an explicit
   minimum bundled-client version or a negotiated protocol version.
7. Verify protobuf comments, generated Go/TypeScript, generated API reference,
   tests, FDR/ADR links, and architecture inventory where applicable.

## Compatibility Checks

Inspect changes that schema tooling cannot fully protect:

- Connect error codes and absence semantics
- authentication, authorization, CORS, and visibility boundaries
- validation becoming stricter
- enum/string meaning changes and unknown-value handling
- pagination, cursor, ordering, and retry interpretation
- request fields silently ignored by older servers
- realtime hello, heartbeat, reconnect, and catch-up behaviour
- public capability-key names and meaning
- bundled client fallbacks when discovery metadata is absent

Run Buf breaking checks and the narrowest behavioural tests that exercise the
changed contract. Do not describe a Buf-clean change as semantically compatible
without reviewing these behaviours.

## Output

Report:

- classification and affected surfaces;
- older-client/newer-server impact;
- newer-client/older-server impact;
- capability or version-skew requirements;
- migration and release-note requirements;
- verification run and any remaining gaps.

For implementation requests, apply all in-scope compatible code, test, docs,
and generated-output changes. For audits or reviews, report findings without
editing unless the user asks for fixes.
