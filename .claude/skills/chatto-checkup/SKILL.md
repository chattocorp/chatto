---
name: chatto-checkup
description: Run a maintenance checkup of the Chatto codebase. Fans out to /fdr, /adr, /chatto-architecture, and /update-project-dependencies, then compiles a single consolidated report. Always propose-only — no changes applied without explicit user approval. Security review is intentionally NOT included; run /chatto-security-review separately when needed.
---

# Chatto Checkup

The standard rounds. The checkup is **propose-only**: it audits and reports, never auto-applies fixes (especially not dependency upgrades).

Security review is intentionally **not** included — it's heavier, slower, and adversarial, and belongs in its own deliberate invocation (`/chatto-security-review`).

## What It Runs

Run these in parallel via the Skill tool:

- **`/fdr`** (no args) — audit all FDRs against the codebase. Surface discrepancies, stale design decisions, and any user-facing features that should now have an FDR.
- **`/adr`** — audit ADRs for staleness. Surface cited file paths or APIs that no longer exist; flag superseded ADRs still referenced as authoritative.
- **`/chatto-architecture`** — refresh `docs/ARCHITECTURE.md` against the current state of streams, KV buckets, and GraphQL operations. Report drift; propose updates.
- **`/update-project-dependencies`** — run only the discovery step (read `cli/go.mod` and `frontend/package.json` and list outdated direct dependencies). **Do not apply.** Report the list; let the user decide whether to upgrade.

## How To Run

1. Invoke all four audits in parallel.
2. **Compile a single consolidated report** at the end. Don't dump four separate report bodies on the user.
3. Present the consolidated report with four sections:
   - **FDR drift** — FDRs that no longer match the code; candidate features for new FDRs.
   - **ADR drift** — stale references in older ADRs.
   - **Architecture drift** — anything in `docs/ARCHITECTURE.md` that's stale.
   - **Dependency updates** — table of outdated packages with current/latest versions.
4. **Wait for user direction** before applying anything. The checkup never auto-applies.

## Report Format

Output a single Markdown report. Brief findings, no walls of text. For every finding, include:

- **What** — one sentence
- **Where** — file path, stream name, or FDR/ADR number
- **Suggested action** — one line

End with a single **Recommended next step** — usually one action with the biggest payoff (e.g., "land the FDR-016 update first; the rest can wait").

## Anti-Patterns

- **Don't apply changes without asking.** The checkup is a status report, not a refactor session.
- **Don't repeat raw audit output verbatim.** Compile and summarize. The point of the meta-skill is to compress four reports into one.
- **Don't include `/chatto-security-review`.** It's a separate, deliberate invocation — not part of routine maintenance.
- **Don't open follow-up PRs from inside the checkup.** Hand the findings back; let the user decide what to act on.
