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

### Project Status

Please use the following facts when making decisions about features or implementation:

- This project is currently in early development.
- As of today, we have a handful of Chatto servers running the 0.0.x version lane that we want to eventually upgrade to 0.1.x. 0.1.x must be able to reliably import data from a 0.0.x server.
- There are no servers deployed running 0.1.x yet, so we can still do breaking changes within this version lane.
- The focus of 0.1.x is on stabilizing the core data model and APIs, improving documentation, and building out a solid foundation for future features. We want to avoid adding new features that aren't necessary for this stabilization effort.

Please update this section as the project evolves, and refer to it when making decisions about features or implementation.

### When making changes...

- Please keep ADRs, FDRs, and other documentation (glossary, docs-website, architecture inventory) up to date.
- Before pushing a branch for a PR, make sure it is named something descriptive of the change.

### Issues, Commits, and PRs

- Use this project's GitHub Issues for planning work.
- Use Conventional Commit format in commit messages.
- Use Conventional Commit format in PR titles. PR bodies should include a bullet list of changes, ideally with links to relevant FDRs, ADRs, and glossary terms.
- When creating or editing multiline GitHub PR/issue bodies with `gh`, write real Markdown to a file/stdin and use `--body-file`. Do not pass escaped `\n` sequences to `--body`; they render literally. Afterward, verify the stored body with `gh pr view --json body --jq .body` or equivalent before telling the user the PR is ready.
- Please keep ADRs and FDRs up to date.
- When the PR closes an issue, please include this information in the PR title or body (e.g. "Fixes #123") so GitHub can link and auto-close them.

### General Coding and Design Guidelines

- Prefer simplicity and clarity over cleverness.
- Where feasible, write code comments that explain intent.
- Make sure the code is well-tested, and that tests are easy to understand and maintain.
