---
name: chatto-finalize-pr
description: Pre-merge PR checklist that verifies FDRs and ADRs are up to date. Runs the fdr and adr audits against the current branch's changes to catch missing documentation updates before merging.
---

# Finalize PR

Run pre-merge documentation checks to ensure FDRs and ADRs are current with the changes on this branch.

## Process

### Step 1: Understand What Changed

Run `git diff main...HEAD --stat` and `git log main..HEAD --oneline` to understand the scope of changes on this branch.

### Step 2: Run Documentation Checks in Parallel

Invoke both skills using the Skill tool:

1. **`/fdr`** — Audit Feature Decision Records against the codebase. Focus on features touched by the branch's changes. Flag any discrepancies, stale design decisions, or new user-facing behavior that should be documented as a new FDR.

2. **`/adr`** — Review `docs/adr/INDEX.md` and check whether any architectural decisions were made on this branch that should be recorded. Look for: new patterns introduced, technology choices, significant design trade-offs, or changes that supersede existing ADRs.

### Step 3: Report

Present a summary to the user:

- **FDRs**: Which FDRs are up to date, which need updates, and whether any new FDRs should be created
- **ADRs**: Whether any new ADRs should be written or existing ones updated
- **Recommended actions**: Concrete next steps (create FDR X, update ADR Y, etc.)

Only make changes with user approval — this skill is for auditing, not auto-fixing.
