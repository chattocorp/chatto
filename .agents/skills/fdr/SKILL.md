---
name: "fdr"
description: "Keep track of Chatto's features as Feature Decision Records (FDRs) \u2014 one structured document per feature capturing behavior, design decisions, and rationale. Replaces the older agent-docs/features/ system."
---

# Feature Decision Records (FDRs)

Manage Chatto's features as structured markdown documents in `docs/fdr/`. Each FDR captures what a feature does from a user perspective **and** the design decisions that shaped it, with rationale.

## What an FDR Is

An FDR is a single source of truth for one feature. It answers:

- **What** does the feature do, behaviorally? (no implementation details)
- **Why** is it built this way? (the design decisions, with rationale)
- **Where** does it fit? (related ADRs, related FDRs, permissions that gate it)

FDRs sit alongside [ADRs](../adr/INDEX.md). The split:

- **ADRs** are about **architectural decisions** — cross-cutting choices like "GraphQL as primary API" or "per-user encryption keys". Often immutable once decided.
- **FDRs** are about **features** — what they do and the design decisions specific to that feature. Updated as the feature evolves.

A single feature may cite several ADRs; a single ADR may underpin several FDRs. That's fine and expected.

## What an FDR Is NOT

- **NOT** a code walkthrough. No function signatures, no proto field tags, no GraphQL schema dumps, no KV key patterns, no NATS subject formats.
- **NOT** a file index. No "Key Files" tables.
- **NOT** an implementation guide. Agents can `grep` for those things.
- **NOT** a changelog. Old design decisions that have been superseded should be rewritten, not appended. The FDR describes the feature *today*.

If you're writing GraphQL schema, proto definitions, or Go/TypeScript code in an FDR, you're going too deep. Stop and pull back to behavior + rationale.

## Directory Structure

```
docs/fdr/
├── INDEX.md                                # Index with TOC (read this first)
├── FDR-001-roles-and-permissions.md       # Individual FDR
├── FDR-002-replies-and-threads.md
└── ...
```

## File Naming

```
FDR-{NNN}-{kebab-case-slug}.md
```

- `NNN`: Zero-padded three-digit number, sequential
- Slug: Short kebab-case summary (not the full title)

## FDR Template

```markdown
# FDR-{NNN}: {Feature Name}

**Status:** Active
**Last reviewed:** {YYYY-MM-DD}

## Overview

One paragraph: what the feature is, who uses it, and why it exists.

## Behavior

Bullet points describing user-visible behavior. No implementation details.

## Design Decisions

Numbered list. Each entry calls out a non-obvious choice and *why*.

### 1. {Short decision title}

**Decision:** What we chose.
**Why:** The reasoning. Cite an ADR if there is one.
**Tradeoff:** What this costs us.

## Permissions

Permission strings that gate this feature, with one-line descriptions.
Omit this section for features that aren't permission-gated.

## Related

- **ADRs:** ADR-XYZ, ADR-ABC
- **FDRs:** FDR-NNN

## Open Questions

Optional. Known design gaps, future considerations, or things we
deliberately haven't decided yet. Delete this section if there's
nothing to say.
```

### Status values

- **Active** — feature is in the codebase and supported
- **Experimental** — feature is in the codebase but unstable
- **Retired** — feature was removed but the FDR is kept to document the prior design (rare; usually we delete instead)

## Workflow

### Before doing anything

1. Read `docs/fdr/INDEX.md` to see the current list of FDRs
2. Only read individual FDR files if relevant to the current task

### Creating a new FDR

1. Read `docs/fdr/INDEX.md` to determine the next available number
2. Use the template above; fill in every required section
3. Set **Last reviewed** to today's date
4. Add the new entry to `docs/fdr/INDEX.md`
5. Cross-reference: if the FDR cites an ADR, the ADR doesn't need to be updated — citations flow one direction (FDR → ADR)
6. **Sibling check**: for each ADR you cite, skim other FDRs that already cite the same ADR. They're likely related to yours and worth listing in your `Related → FDRs` line.

### Updating an FDR

1. Read the FDR
2. Make targeted edits. Don't append "Update: ..." notes — rewrite the affected section so the doc describes the feature *today*.
3. Bump **Last reviewed** to today's date
4. If the title changed, update `docs/fdr/INDEX.md`

### Retiring an FDR

When a feature is removed:

- **Default:** Delete the FDR. Remove the entry from `INDEX.md`. The git history preserves it if anyone needs to look back.
- **Exception:** Set status to `Retired` and keep it only if the prior design is notable and likely to inform future work (e.g., a system that was deliberately rolled back and might be reconsidered). Add a top-level note explaining why it was retired.

## Writing Style

- **Behavior section: bullet points, short paragraphs.** No code blocks. Describe what users see and experience.
- **Design Decisions: numbered, with explicit Why + Tradeoff.** Each entry should be defensible to a future maintainer who didn't live through the original conversation.
- **Mention permission strings** (e.g., `message.react`) — they're part of the feature design. But don't describe *how* permission checks are implemented.
- **Cite ADRs by number** when a design decision is downstream of an architectural choice. Don't restate the ADR; just point to it.
- **Omit sections that don't add value.** A simple feature might just need Overview + Behavior + Permissions. Don't pad.

## How To Run

### Mode 1: Audit All FDRs (default)

When invoked without arguments, audit every FDR in `docs/fdr/` against the codebase.

### Mode 2: Audit One FDR

When invoked with a slug (e.g., `/fdr reactions`), audit only that FDR.

### Mode 3: Create a New FDR

When invoked with `new <feature-slug>` (e.g., `/fdr new voice-calls`), research the feature and draft a new FDR. Always ask the user to confirm the slug, number, and scope before writing.

## Audit Process

For each FDR being audited, launch a dedicated Explore subagent that:

1. **Reads the FDR** in full
2. **Verifies each claim** against the codebase:
   - Permission names — do the constants exist in `cli/internal/core/permissions.go`?
   - Behavioral claims — does the code actually work this way?
   - Design Decisions — are the stated rationales still accurate? Has the implementation drifted?
   - Related ADRs/FDRs — do the cited records still exist and still apply?
3. **Returns a structured report** with:
   - **Verified**: claims confirmed by code
   - **Discrepancies**: claims that contradict the code
   - **Stale claims**: behaviors or decisions that no longer apply
   - **Missing**: significant user-facing behavior the FDR doesn't mention

Run audits in parallel via the Agent tool when auditing multiple FDRs.

## After Auditing

1. Present a summary: which FDRs are clean, which need updates
2. For each discrepancy, propose a concrete edit
3. Only apply updates with user approval — don't silently rewrite
4. If new docs were added or titles changed, update `docs/fdr/INDEX.md`

## Verification Checklist

When auditing or creating an FDR, verify:

- [ ] All permission strings exist in `cli/internal/core/permissions.go`
- [ ] Permission strings use hyphens, not underscores
- [ ] User-facing behaviors match the code
- [ ] Design Decisions still reflect the current implementation
- [ ] Cited ADRs / FDRs exist and are relevant
- [ ] `INDEX.md` lists this FDR with the correct title

## TOC Format (in INDEX.md)

```markdown
| # | Feature | Status | Last reviewed |
|---|---------|--------|---------------|
| [FDR-001](FDR-001-slug.md) | Title of the feature | Active | 2026-05-19 |
```
