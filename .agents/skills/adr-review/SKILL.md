---
name: adr-review
description: "Run a full review of all Architecture Decision Records for implementation drift, contradictions, stale decisions, missing supersession notes, weak rationale, and missing cross-references. Use when the user asks for an ADR review, ADR audit, stale ADR check, architecture decision consistency check, or wants to verify ADRs against the current codebase, feature/design records, architecture docs, glossary, or public API state."
---

# ADR Review

Review ADRs as architectural records that must remain useful to future maintainers. An ADR review is an audit and recommendation pass, not a rewrite pass unless the user explicitly asks for edits.

## Ground Rules

- Read `docs/adr/INDEX.md` first.
- Always audit the full ADR set.
- Read individual ADR files according to the full-audit plan; read related ADRs together when they form a decision chain.
- Treat reviews as propose-only by default: report findings and concrete proposed edits, but do not modify ADRs, related records, or other docs unless the user asks.
- Use current code and docs as evidence. Do not mark an ADR stale only because a newer preference exists; show the contradiction, missing supersession, or drift.
- Do not use partial file lists to decide what to review.
- If editing is requested, also use the `adr` skill's update/supersede workflow.

## Full ADR Audit

1. Read `docs/adr/INDEX.md`.
2. Group ADRs by topic so related decisions are checked together.
3. Prioritize recent ADRs, superseded/superseding chains, and ADRs likely to affect current architecture:
   - storage, data models, events, jobs, queues, caches, indexes, and projections
   - public APIs, wire protocols, compatibility, and generated clients
   - authorization, identity, tenancy, privacy, and encryption
   - frontend/client architecture, localization, delivery, and deployment
4. Audit in parallel with subagents when available; otherwise use local searches and read-only shell commands.
5. Write a consolidated report rather than scattering findings across ADRs.
6. Search for related feature records, design records, architecture docs, and user-facing docs that cite or should cite each ADR.
7. Check the current implementation and relevant docs for every ADR topic.

## Evidence Sources

Use these sources according to the ADR topic. Adapt paths to the project if its documentation layout differs:

- ADR inventory: `docs/adr/INDEX.md`
- Related decision/feature records: `docs/fdr/`, `docs/rfc/`, `docs/design/`, `docs/features/`, or equivalent project-specific directories when present
- Current architecture inventory: `docs/architecture/` (with
  `docs/ARCHITECTURE.md` retained only as a compatibility landing page), or the
  repository's equivalent
- Canonical vocabulary: `docs/GLOSSARY.md`, terminology docs, or domain model docs when present
- Public APIs and protocols: API schemas, IDLs, OpenAPI specs, protobuf/Thrift/GraphQL schemas, REST/RPC route definitions, WebSocket protocols, and generated clients
- Core implementation: service wiring, domain models, persistence code, background jobs, event handlers, migrations, and integration boundaries
- Client implementation: frontend, mobile, SDK, CLI, or other consumer code affected by the decision
- Deployment and operations: infrastructure config, runtime configuration, observability, backup/restore, and rollout docs
- Existing repo rules: root `AGENTS.md` and path-specific `AGENTS.md` files when present

## What To Check

For every ADR, check:

- **Existence and index:** file exists, index row points to the right file, title and date match.
- **Decision clarity:** the Decision section states a concrete architectural choice, not only intent or background.
- **Context currency:** the motivating problem still makes sense or is clearly historical.
- **Implementation drift:** current code, schemas, generated APIs, runtime resources, deployment config, or client structure contradict the ADR.
- **Supersession:** superseded ADRs are marked clearly, replacement ADRs reference the old decisions, and the index title makes supersession discoverable when appropriate.
- **Consequences:** consequences name real tradeoffs, migration costs, compatibility constraints, operational effects, or maintenance burdens.
- **Public compatibility:** storage, schemas, discovery, API, protocol, and mixed-version implications are explicit when relevant.
- **Cross-references:** related feature/design records, architecture docs, and user-facing docs cite or align with the ADR where appropriate.
- **Vocabulary:** terms match the project's glossary, domain model, and current product or architecture naming.
- **Documentation drift:** architecture docs, docs website pages, related decision records, and feature docs do not describe a different current architecture.

## Finding Categories

Classify findings with one of these labels:

- **Contradiction:** the ADR conflicts with current code or another active ADR.
- **Stale:** the ADR describes old architecture without a supersession/update note.
- **Missing Supersession:** a newer ADR or implementation replaced the decision, but the old ADR does not say so.
- **Weak Decision:** the ADR does not record a concrete choice.
- **Weak Consequences:** important tradeoffs, compatibility impact, or operational costs are omitted.
- **Missing Cross-Reference:** related records or docs should cite or align with the ADR.
- **Index Issue:** `docs/adr/INDEX.md` is missing, mislabeling, or mislinking an ADR.
- **No Issue:** checked and no material update is needed.

## Report Format

Start with findings, ordered by severity. Use file links and concrete evidence.

```markdown
## Findings

- **Contradiction:** [ADR-042](docs/adr/ADR-042-example-decision.md) says ...
  Evidence: `src/...` now ...
  Proposed fix: ...

## Clean / Low-Risk ADRs

- ADR-044: checked against the relevant implementation and docs; no material drift found.

## Open Questions

- ...

## Proposed Edits

- Update ADR-...
- Add supersession note to ADR-...
- Update related feature/design record references.
```

Keep the report concise. For large audits, include a summary table and save detailed notes in `.context/adr-review-YYYY-MM-DD.md` when useful for collaboration with other agents.

## Applying Fixes

Only apply fixes when the user asks.

When applying fixes:

1. Use the `adr` skill workflow for updating or superseding ADRs.
2. Preserve ADR numbering and filenames unless creating a new ADR.
3. Update `docs/adr/INDEX.md` if a title changes or a new ADR is added.
4. Run a cross-reference sweep: scan related feature/design record indexes and update ADR reference lines when an ADR becomes relevant.
5. Update architecture docs, glossary/terminology docs, or docs website pages only when the review finding requires it and the user approved edits.
6. Run targeted verification for edited docs, such as markdown link checks or focused grep checks for references.
