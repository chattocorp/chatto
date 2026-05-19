---
name: chatto-checkup
description: Run a periodic maintenance checkup of the Chatto codebase. Fans out to /fdr, /adr, /chatto-architecture, /chatto-security-review, /update-project-dependencies based on the requested cadence (weekly, monthly, or quarterly). Always propose-only — no changes applied without explicit user approval.
---

# Chatto Checkup

A periodic maintenance pass over the codebase. The checkup is **propose-only**: it audits and reports, never auto-applies fixes (especially not dependency upgrades).

## Arguments: $ARGUMENTS

The cadence selects which audits to run:

- `weekly` — surface available dependency upgrades only.
- `monthly` (default) — FDR audit, architecture refresh, and dependency upgrades.
- `quarterly` — everything in `monthly` plus ADR audit and security review.

If no argument is provided, treat it as `monthly`.

## What Each Cadence Covers

### `weekly`

- **`/update-project-dependencies`** — run only the discovery step (read `cli/go.mod` and `frontend/package.json` and list outdated direct dependencies). **Do not apply**. Report the list; let the user decide whether to upgrade.

### `monthly`

Everything in `weekly`, plus:

- **`/fdr`** (no args) — audit all FDRs against the codebase. Surface discrepancies, stale design decisions, and any user-facing features that should now have an FDR.
- **`/chatto-architecture`** — refresh `docs/ARCHITECTURE.md` against the current state of streams, KV buckets, and GraphQL operations. Report drift; propose updates.

### `quarterly`

Everything in `monthly`, plus:

- **`/adr`** — audit ADRs for staleness. Surface cited file paths or APIs that no longer exist; flag superseded ADRs still referenced as authoritative.
- **`/chatto-security-review`** — full multi-agent security audit. Output saved under `.context/security-review-*.md`.

## How To Run

1. Resolve the cadence from `$ARGUMENTS` (default `monthly`).
2. Run the audits **in parallel** via the Skill tool. They don't depend on each other.
3. **Compile a single consolidated report** at the end. Don't dump five separate report bodies on the user.
4. Present the consolidated report with sections matching the cadence:
   - **Dependency updates** — table of outdated packages with current/latest versions.
   - **FDR drift** — any FDRs that no longer match the code; candidate features for new FDRs.
   - **Architecture drift** — anything in `docs/ARCHITECTURE.md` that's stale.
   - **ADR drift** *(quarterly only)* — stale references in older ADRs.
   - **Security findings** *(quarterly only)* — link to `.context/security-review-final.md` plus a top-line summary.
5. **Wait for user direction** before applying anything. The checkup never auto-applies.

## Report Format

Output a single Markdown report. Brief findings, no walls of text. For every finding, include:

- **What** — one sentence
- **Where** — file path, stream name, or FDR/ADR number
- **Suggested action** — one line

End with a single **Recommended next step** — usually one action with the biggest payoff (e.g., "land the FDR-016 update first; the rest can wait").

## Anti-Patterns

- **Don't apply changes without asking.** The checkup is a status report, not a refactor session.
- **Don't run the full suite when the user asked for `weekly`.** Stick to the requested cadence — that's the whole point of having tiers.
- **Don't repeat raw audit output verbatim.** Compile and summarize. The point of the meta-skill is to compress five reports into one.
- **Don't open follow-up PRs from inside the checkup.** Hand the findings back; let the user decide what to act on.
