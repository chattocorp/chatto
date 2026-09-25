---
name: "fdr"
description: "Keep track of product or project features as Feature Decision Records (FDRs): one structured document per feature capturing behavior, design decisions, and rationale."
---

# Feature Decision Records (FDRs)

Manage product or project features as structured markdown documents. By default, FDRs live in `docs/fdr/`, but follow the repository's existing documentation layout if it already uses a different path.

Each FDR captures what a feature does from a user perspective and the design decisions that shaped it, with rationale.

## What an FDR Is

An FDR is a single source of truth for one feature. It answers:

- **What** does the feature do, behaviorally? No implementation details.
- **Why** is it built this way? The design decisions, with rationale.
- **Where** does it fit? Related ADRs, related FDRs, and gates such as permissions, roles, plans, feature flags, or rollout rules.

FDRs sit alongside ADRs. The split:

- **ADRs** are about architectural decisions: cross-cutting choices like "event-driven integrations" or "tenant-scoped authorization". Often immutable once decided.
- **FDRs** are about features: what they do and the design decisions specific to that feature. Updated as the feature evolves.

A single feature may cite several ADRs; a single ADR may underpin several FDRs. That's fine and expected.

## What an FDR Is NOT

- **NOT** a code walkthrough. No function signatures, no wire-format dumps, no database schema dumps, no queue/topic names, no low-level storage keys.
- **NOT** a file index. No "Key Files" tables.
- **NOT** an implementation guide. Agents can search the codebase for implementation details.
- **NOT** a changelog. Old design decisions that have been superseded should be rewritten, not appended. The FDR describes the feature today.

If you're writing API handler details, framework internals, type definitions, or code snippets in an FDR, you're going too deep. Stop and pull back to behavior and rationale.

## Directory Structure

Default layout:

```text
docs/fdr/
|-- INDEX.md                                # Index with TOC (read this first)
|-- FDR-001-roles-and-permissions.md       # Individual FDR
|-- FDR-002-export-history.md
`-- ...
```

If a repository already has a feature-decision directory, use that instead. If no FDR directory exists, prefer `docs/fdr/` unless the user asks for a different location.

## File Naming

```text
FDR-{NNN}-{kebab-case-slug}.md
```

- `NNN`: Zero-padded three-digit number, sequential.
- Slug: Short kebab-case summary, not the full title.

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

Numbered list. Each entry calls out a non-obvious choice and why.

### 1. {Short decision title}

**Decision:** What we chose.
**Why:** The reasoning. Cite an ADR if there is one.
**Tradeoff:** What this costs us.

## Gates

Capabilities, roles, permission strings, feature flags, plan tiers, rollout rules, or other conditions that gate this feature, with one-line descriptions.
Omit this section for features that aren't gated.

## Related

- **ADRs:** ADR-XYZ, ADR-ABC
- **FDRs:** FDR-NNN

## Open Questions

Optional. Known design gaps, future considerations, or things we
deliberately haven't decided yet. Delete this section if there's
nothing to say.
```

### Status values

- **Active**: feature is in the codebase and supported.
- **Experimental**: feature is in the codebase but unstable.
- **Planned**: feature has an accepted design but is not fully shipped yet.
- **Retired**: feature was removed but the FDR is kept to document the prior design. This should be rare; usually delete instead.

## Workflow

### Before doing anything

1. Read relevant project instructions first. Check for repository guidance such as `AGENTS.md`, `CLAUDE.md`, `.agents/`, `.codex/`, or documentation notes that mention FDRs, ADRs, permissions, roles, feature flags, plans, or product documentation.
2. Locate the FDR directory. Check for `docs/fdr/INDEX.md` first, then search for `FDR-*.md` if needed.
3. Read the FDR index to see the current list of records.
4. Only read individual FDR files if relevant to the current task.
5. If ADRs are referenced, locate the repository's ADR index before resolving citations.
6. Identify project-specific gate sources before validating gates. Search for permission, role, plan, entitlement, feature-flag, rollout, or capability registries in the current repository instead of assuming a fixed file path.

### Creating a new FDR

1. Read the FDR index to determine the next available number.
2. Use the template above; fill in every required section.
3. Set **Last reviewed** to today's date.
4. Add the new entry to the FDR index.
5. Cross-reference: if the FDR cites an ADR, the ADR doesn't need to be updated. Citations flow one direction: FDR to ADR.
6. **Sibling check**: for each ADR you cite, skim other FDRs that already cite the same ADR. They're likely related to yours and worth listing in your `Related -> FDRs` line.
7. If no FDR structure exists yet, confirm the directory, numbering scheme, and initial scope with the user before creating files.

### Updating an FDR

1. Read the FDR.
2. Make targeted edits. Don't append "Update: ..." notes; rewrite the affected section so the doc describes the feature today.
3. Bump **Last reviewed** to today's date.
4. If the title changed, update the FDR index.
5. If related FDRs become stale because of the change, update their `Related` entries too.

### Retiring an FDR

When a feature is removed:

- **Default:** Delete the FDR. Remove the entry from the index. Git history preserves it if anyone needs to look back.
- **Exception:** Set status to `Retired` and keep it only if the prior design is notable and likely to inform future work, such as a system that was deliberately rolled back and might be reconsidered. Add a top-level note explaining why it was retired.

## Writing Style

- **Behavior section: bullet points, short paragraphs.** No code blocks. Describe what users see and experience.
- **Design Decisions: numbered, with explicit Why and Tradeoff.** Each entry should be defensible to a future maintainer who didn't live through the original conversation.
- **Mention gates that shape the feature.** Permissions, roles, plan tiers, feature flags, or rollout constraints are part of feature design. Don't describe how the checks are implemented.
- **Cite ADRs by number** when a design decision is downstream of an architectural choice. Don't restate the ADR; just point to it.
- **Omit sections that don't add value.** A simple feature might just need Overview and Behavior. Don't pad.
- **Prefer stable product language over implementation language.** Use user roles and visible outcomes, not class names, package names, or request/response shapes.
- **Rewrite stale content in place.** Avoid "Previously..." sections unless the prior behavior is still needed to explain compatibility or migration.

## How To Run

### Mode 1: Audit All FDRs (default)

When invoked without arguments, audit every FDR in the FDR directory against the codebase.

### Mode 2: Audit One FDR

When invoked with a slug, title fragment, or FDR number, audit only that FDR.

### Mode 3: Create a New FDR

When invoked with `new <feature-slug>` such as `new export-history`, research the feature and draft a new FDR. Always ask the user to confirm the slug, number, and scope before writing.

## Audit Process

For each FDR being audited:

1. **Read the FDR** in full.
2. **Verify each claim** against the codebase and adjacent documentation:
   - Gates: do the referenced permissions, roles, plan tiers, feature flags, or rollout conditions exist in the codebase, configuration, or product documentation?
   - Gate sources: did you first identify the repository's actual registries or conventions for these gates?
   - Behavioral claims: does the code actually work this way?
   - Design decisions: are the stated rationales still accurate? Has the implementation drifted?
   - Related ADRs/FDRs: do the cited records still exist and still apply?
3. **Return a structured report** with:
   - **Verified:** claims confirmed by code or documentation.
   - **Discrepancies:** claims that contradict the implementation.
   - **Stale claims:** behaviors or decisions that no longer apply.
   - **Missing:** significant user-facing behavior the FDR doesn't mention.

When auditing multiple FDRs and subagents are available, run audits in parallel. If subagents are not available, audit sequentially and keep notes per FDR.

## After Auditing

1. Present a summary: which FDRs are clean, which need updates.
2. For each discrepancy, propose a concrete edit.
3. Only apply updates with user approval unless the user explicitly asked you to update the FDRs.
4. If docs were added, removed, renumbered, or retitled, update the FDR index.

## Verification Checklist

When auditing or creating an FDR, verify:

- [ ] Relevant project instructions and documentation conventions were checked first.
- [ ] The FDR directory and numbering match the repository's existing convention.
- [ ] The repository's gate sources or naming conventions were identified before validating gates.
- [ ] All referenced gates exist in the codebase, configuration, or product documentation.
- [ ] Gate names use the repository's established naming style.
- [ ] User-facing behaviors match the code.
- [ ] Design decisions still reflect the current implementation.
- [ ] Cited ADRs and FDRs exist and are relevant.
- [ ] The FDR index lists this FDR with the correct title, status, and review date.
- [ ] The FDR contains no unnecessary implementation details.

## TOC Format

The FDR index should use a markdown table:

```markdown
| # | Feature | Status | Last reviewed |
|---|---------|--------|---------------|
| [FDR-001](FDR-001-slug.md) | Title of the feature | Active | 2026-05-19 |
```
