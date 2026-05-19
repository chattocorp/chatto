---
name: adr
description: Keep track of architectural decisions in a structured format using Architecture Decision Records (ADRs).
---

# Architecture Decision Records (ADRs)

Manage architectural decisions as structured markdown documents in `docs/adr/`.

## Directory Structure

```
docs/adr/
├── INDEX.md                                          # Index with TOC (read this first)
├── ADR-001-nats-jetstream-instead-of-kafka.md      # Individual ADR
├── ADR-002-embedded-nats-server.md
└── ...
```

## Workflow

### Before doing anything

1. Read `docs/adr/INDEX.md` to see the current index of all ADRs
2. Only read individual ADR files if their content is relevant to the current task

### Creating a new ADR

1. Read `docs/adr/INDEX.md` to determine the next available number
2. Create the ADR file using the template below
3. Update `docs/adr/INDEX.md` to add the new entry to the TOC
4. **FDR sweep**: after writing, scan `docs/fdr/INDEX.md` for features whose design now relates to this ADR. Update those FDRs to cite the new ADR in their `Related → ADRs` line. (ADRs themselves don't carry a `Related FDRs` section — citations flow FDR → ADR only.)

### Updating an ADR

1. Read `docs/adr/INDEX.md` to find the ADR
2. Read the specific ADR file
3. Make changes (typically amending the consequences or adding context)
4. Update the TOC in `docs/adr/INDEX.md` if the title changed

### Superseding an ADR

1. Create a new ADR that references the old one
2. Add a note to the old ADR's Context or Decision section pointing to the replacement
3. Update the TOC to include the new entry

## File Naming

```
ADR-{NNN}-{kebab-case-slug}.md
```

- `NNN`: Zero-padded three-digit number, sequential
- Slug: Short kebab-case summary of the decision (not the full title)

## ADR Template

```markdown
# ADR-{NNN}: {Title}

**Date:** {YYYY-MM-DD}

## Context

What is the issue that we're seeing that is motivating this decision or change?

## Decision

What is the change that we're proposing and/or doing?

## Consequences

What becomes easier or more difficult to do because of this change?
```

## TOC Format (in INDEX.md)

Each entry in the TOC should be a markdown table row:

```markdown
| # | Decision | Date |
|---|----------|------|
| [ADR-001](ADR-001-slug.md) | Title of the decision | 2026-03-01 |
```
