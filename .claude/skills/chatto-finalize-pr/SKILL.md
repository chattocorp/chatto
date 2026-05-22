---
name: chatto-finalize-pr
description: Pre-merge PR checklist that verifies FDRs and ADRs are up to date. Runs the fdr and adr audits against the current branch's changes to catch missing documentation updates before merging.
---

# Finalize PR

Run pre-merge documentation checks to ensure FDRs and ADRs are current with the changes on this branch.

## Process

### Step 1: Understand What Changed

Run `git diff main...HEAD --stat` and `git log main..HEAD --oneline` to understand the scope of changes on this branch.

### Step 2: Run Documentation Checks in Order

Invoke the two skills sequentially via the Skill tool — **`/adr` first, then `/fdr`**. The order matters: architectural decisions inform feature documents, and running ADR triage first lets any new ADRs get extracted before the FDR pass starts citing them. Running in parallel produces FDR drafts containing principles that should have been ADRs (a real failure mode — see the triage section in [`fdr/SKILL.md`](../fdr/SKILL.md#triage-is-this-an-adr-or-an-fdr)).

1. **`/adr`** *(first)* — Review `docs/adr/INDEX.md` and check whether any architectural decisions were made on this branch that should be recorded. Look for: new patterns introduced, technology choices, significant design trade-offs, principles ("we never X", "we always Y") that apply beyond a single feature, or changes that supersede existing ADRs.

2. **`/fdr`** *(after `/adr`)* — Audit Feature Decision Records against the codebase. Focus on features touched by the branch's changes. Flag discrepancies, stale design decisions, and user-facing behavior that should be documented as a new FDR. Cite any ADRs extracted in step 1 rather than re-stating their content.

### Step 3: Report

Present a summary to the user:

- **FDRs**: Which FDRs are up to date, which need updates, and whether any new FDRs should be created
- **ADRs**: Whether any new ADRs should be written or existing ones updated
- **Recommended actions**: Concrete next steps (create FDR X, update ADR Y, etc.)

Only make changes with user approval — this skill is for auditing, not auto-fixing.
