---
name: "adr"
description: "Keep track of architectural decisions in a structured format using Architecture Decision Records (ADRs)."
---

# Architecture Decision Records (ADRs)

Manage architectural decisions as structured markdown documents in `docs/adr/`.

## What an ADR Is

An ADR records one architectural decision and why it was made. It answers:

- **What** decision was made? (e.g. "use NATS JetStream as the message bus")
- **Why** was it made? (the context, constraints, and alternatives considered)
- **What does it cost?** (the consequences — what becomes easier or harder)

ADRs sit alongside [FDRs](../fdr/INDEX.md). The split:

- **ADRs** are about **architectural decisions** — cross-cutting choices that underpin multiple features, often immutable once decided.
- **FDRs** are about **features** — what they do and the design decisions specific to that feature.

A single ADR may underpin several FDRs; a single FDR may cite several ADRs. ADRs don't list back-references — citations flow FDR → ADR.

## What an ADR Is NOT

- **NOT** a description of a *fact*. "We use NATS" is not an ADR; "we chose NATS over Kafka because we wanted embeddable single-binary deployment" is.
- **NOT** a feature design. If the decision is shaped by one feature's user-facing requirements and would go away if the feature did, it's an FDR.
- **NOT** an implementation guide. ADRs explain *why*, not *how*.
- **NOT** a living document. Once the decision is made, the ADR records the moment. Later changes either supersede the ADR (with a new ADR) or amend the *Consequences* section as reality clarifies. Don't rewrite the *Decision* to reflect a different choice — write a superseding ADR instead.

## Triage: is this an ADR or an FDR?

When in doubt about whether a decision belongs in an ADR or an FDR, check the heuristics in [`../fdr/SKILL.md`](../fdr/SKILL.md#triage-is-this-an-adr-or-an-fdr). Quick summary, any one tipping toward ADR:

- Would the decision still apply if any single feature were removed?
- Is it a *principle* ("we never X", "we always Y") rather than a *behavior* ("when user does X, system does Y")?
- Would a future feature with a similar shape inherit it?
- Is it already relevant to more than one feature?

**Smell test while drafting**: if a sentence in your ADR is describing what *one specific feature* does to the user, you've drifted into FDR territory. Pull back to the architectural choice and let the FDR describe the feature.

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
