---
name: "chatto-checkup"
description: "Run a documentation-focused maintenance checkup of the Chatto codebase. Fans out to /fdr, /adr, and /chatto-architecture, then compiles a single consolidated report. Always propose-only \u2014 no changes applied without explicit user approval. At the end, points the human at other maintenance skills (/update-project-dependencies, /chatto-security-review) they may want to run themselves."
---

# Chatto Checkup

The standard documentation rounds. The checkup is **propose-only**: it audits and reports, never auto-applies fixes.

Heavier or more opinionated work — dependency upgrades, security review — is deliberately **not** invoked here. The end-of-report section points the human at those skills so they can decide whether to run them.

## What It Runs

Run these in parallel via the Skill tool:

- **`/fdr`** (no args) — audit all FDRs against the codebase. Surface discrepancies, stale design decisions, and any user-facing features that should now have an FDR.
- **`/adr`** — audit ADRs for staleness. Surface cited file paths or APIs that no longer exist; flag superseded ADRs still referenced as authoritative.
- **`/chatto-architecture`** — refresh `docs/ARCHITECTURE.md` against the current state of core services, projections, EVT event tokens, EVT/live subject patterns, current NATS resources, ConnectRPC endpoints, and realtime websocket behavior. Report drift; propose updates.

## How To Run

1. Invoke all three audits in parallel.
2. **Compile a single consolidated report** at the end. Don't dump three separate report bodies on the user.
3. Present the consolidated report with three sections:
   - **FDR drift** — FDRs that no longer match the code; candidate features for new FDRs.
   - **ADR drift** — stale references in older ADRs.
   - **Architecture drift** — anything in `docs/ARCHITECTURE.md` that's stale, including services, projections, EVT events, subject patterns, or current resource inventories.
4. End with a **Recommended next step** — usually one action with the biggest payoff (e.g., "land the FDR-016 update first; the rest can wait").
5. Then a final **Other skills you may want to run** section that lists, as plain pointers (no invocation):
   - `/update-project-dependencies` — bump deps within semver and run tests
   - `/chatto-security-review` — multi-agent security audit
6. **Wait for user direction** before applying anything. The checkup never auto-applies.

## Report Format

Output a single Markdown report. Brief findings, no walls of text. For every finding, include:

- **What** — one sentence
- **Where** — file path, stream name, or FDR/ADR number
- **Suggested action** — one line

## Anti-Patterns

- **Don't apply changes without asking.** The checkup is a status report, not a refactor session.
- **Don't repeat raw audit output verbatim.** Compile and summarize. The point of the meta-skill is to compress three reports into one.
- **Don't invoke `/update-project-dependencies` or `/chatto-security-review`.** They're surfaced as pointers at the end of the report; the human invokes them.
- **Don't open follow-up PRs from inside the checkup.** Hand the findings back; let the user decide what to act on.
