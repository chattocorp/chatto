# Instructions for Agents

### Documentation

Please refer to this repository's README.md for general information.

This codebase keeps agent-relevant context in six places. Read the one that fits your task:

- **`.claude/rules/**`** — always-on coding, testing, and review conventions, mostly path-scoped (`frontend.md` and `frontend-conventions.md` for SvelteKit work, `backend.md` for Go, `testing-frontend.md` / `testing-backend.md` for tests, `authorization.md` for permission changes, etc.). Start here for "how do we do things in this repo?"
- **`.agents/skills/**`** — opt-in agent skills for repeated workflows. Use `svelte-core-bestpractices` together with the Svelte MCP tools when writing, editing, or reviewing Svelte components and modules.
- **`docs/fdr/INDEX.md`** — **Feature** Decision Records, one per feature. They describe what a feature does *and* why it's designed that way. Read the relevant FDR before changing user-facing behavior.
- **`docs/adr/INDEX.md`** — **Architecture** Decision Records. Cross-cutting choices like "NATS as primary data store" or "per-user encryption keys with crypto-shredding". Read when touching architectural seams.
- **`docs/ARCHITECTURE.md`** — inventory of what currently exists (streams, KV buckets, subject patterns, GraphQL operations). Use when you need to know *what's where*, not *why*.
- **`docs/GLOSSARY.md`** — one-line definitions of Chatto-specific terms (Server, Space, Echo, OCC, etc.). Skim when you encounter a word you don't recognize.

### Maintenance slash commands

Periodic codebase upkeep — all propose-only unless noted:

| Command | Use when |
| --- | --- |
| `/chatto-checkup` | Standard documentation rounds; fans out to `/fdr`, `/adr`, `/chatto-architecture`. Points at other maintenance skills at the end. |
| `/chatto-finalize-pr` | Pre-merge check on the current branch — runs `/fdr` + `/adr`. |
| `/fdr [feature]` | Audit Feature Decision Records against the code, or create a new one. |
| `/adr` | Audit Architecture Decision Records, or create a new one. |
| `/glossary [term \| add <term>]` | Look up, add, or audit terms in `docs/GLOSSARY.md`. |
| `/chatto-architecture` | Refresh `docs/ARCHITECTURE.md` inventory. |
| `/chatto-security-review` | Multi-agent security audit. |
| `/update-project-dependencies` | Bump deps within semver, run tests. |
| `/chatto-debugging` | `nats` CLI recipes for production debugging. |
| `/chatto-dev-instance` | Deploy to dev (Argo Rollouts). |
| `/chatto-release-announcement` | Generate release notes from git tags. |

### Issues, Commits, and PRs

- Use this project's GitHub Issues for planning work.
- Use Conventional Commit format in commit messages.
- Use Conventional Commit format in PR titles. PR bodies should include a bullet list of changes, ideally with links to relevant FDRs, ADRs, and glossary terms.
- Please keep ADRs and FDRs up to date.
