# Instructions for Agents

### Documentation

Please refer to this repository's README.md for general information.

This codebase keeps agent-relevant context in six places. Read the one that fits your task:

- **`.claude/rules/**`** — always-on coding, testing, and review conventions, mostly path-scoped (`frontend.md` and `frontend-conventions.md` for SvelteKit work, `backend.md` for Go, `testing-frontend.md` / `testing-backend.md` for tests, `authorization.md` for permission changes, etc.). Start here for "how do we do things in this repo?"
- **`.agents/skills/**`** — opt-in agent skills for repeated workflows. Use the matching skill when a task names one or clearly fits its workflow: `chatto-architecture` for `docs/ARCHITECTURE.md`, `glossary` for `docs/GLOSSARY.md`, and `svelte-core-bestpractices` together with the Svelte MCP tools when writing, editing, or reviewing Svelte components and modules.
- **`docs/fdr/INDEX.md`** — **Feature** Decision Records, one per feature. They describe what a feature does *and* why it's designed that way. Read the relevant FDR before changing user-facing behavior.
- **`docs/adr/INDEX.md`** — **Architecture** Decision Records. Cross-cutting choices like "NATS as primary data store" or "per-user encryption keys with crypto-shredding". Read when touching architectural seams.
- **`docs/ARCHITECTURE.md`** — current-version inventory of what exists: core services, projections, EVT events and subject patterns, streams, KV buckets, object stores, key shapes, and GraphQL operations. Use when you need to know *what's where*, not *why*, and keep it current rather than reintroducing historical storage inventories.
- **`docs/GLOSSARY.md`** — canonical one-line definitions of Chatto-specific terms (Server, Space, Event, Subject, Projection, OCC, etc.). Skim when you encounter a word you don't recognize, and update it when introducing or renaming shared concepts.

### Project Status

Please use the following facts when making decisions about features or implementation:

- This project is currently in early development.
- We've made the software public and users can now self-host. We're advising self-hosters to use the `:latest` image tag to keep up to date, but there may be stragglers.
- The next big new version will be 0.2.0, but for now we're concentrating on smaller, non-breaking changes to improve the application.

Please update this section as the project evolves, and refer to it when making decisions about features or implementation.

### General Coding and Design Guidelines

- Prefer simplicity and clarity over cleverness.
- Where feasible, write code comments that explain intent.
- Make sure the code is well-tested, and that tests are easy to understand and maintain.

### Specific Rules for Frontend Code

- Review your changes and additions in the browser, using the Chrome Devtools MCP.
- We use Tailwind 4 for styling. Please use it, and don't write custom CSS directives.
- Establish Tailwind 4 utility classes and/or Svelte components where feasible.
- Before styling overlays, popovers, banners, toasts, menus, or similar floating UI, reuse the closest established utility/component pattern first (for example `menu`, `menu-section`, `btn`, and existing chat overlays). Add a shared Tailwind utility or component when a pattern is missing instead of inventing one-off visual styles.
- Checkboxes and similar in the Server Admin UI should save their change immediately on click, confirmed through a toast notification.
- Implement pagination as automatic "load more" (ie. when the edge of the container is reached), not manual/page-based pagination.
- Use "Save" buttons only for forms with multiple fields that need to be submitted together, and make sure they are disabled until a change is made.
- Never silence linter warnings.
- Avoid Svelte's $effect like the plague. In almost all cases, there are better Svelte tools to do the same thing (eg. $derived, attachments, ...)

### Specific Rules for Backend Code

- Keep in mind that multiple replicas of the same server may be running, so anything you build must be ready to work in such a setup. Never assume that there is only ever a single replica.
- State changes go into the EVT stream. Do not litter RUNTIME_STATE or other KV buckets with durable state unless it's something that we deliberately don't want to put into the main EVT event stream (eg. encryption keys, ephemeral state like typing notifications, last-read markers, etc.)
- All state interactions should go through a Service responsible for a specific domain; that Service should create and maintain whatever projections it needs to do its work, and expose methods for the rest of the codebase to interact with it. Avoid direct interactions with JetStream, KV, or projections from outside of Services.
- GraphQL list pagination should follow the existing offset style unless there is a specific reason to use a cursor. Use `limit` and `offset` arguments, normalize them through the backend `paginationArgs` helper with a domain-appropriate default/max, and return a connection-like object with the page items plus `totalCount` and `hasMore`. Compute `hasMore` from the requested offset and actual returned page length, and keep filtering/search semantics server-side so clients can page through filtered results without loading the whole list.
- When adding GraphQL fields that frontend clients will use against registered remote servers, keep mixed-version compatibility in mind. Where feasible, isolate new/optional fields into focused follow-up queries instead of adding them to critical bootstrap queries, and have the frontend handle `Cannot query field "..."` validation errors through the shared GraphQL compatibility helper so older servers do not break the multi-server client.
- Never log PII. Logs must not include raw login names, display names, email addresses, submitted auth identifiers, OAuth/OIDC provider subject identifiers, tokens, passwords, auth codes, reset links, raw IP addresses, or full query strings. Prefer opaque Chatto IDs, counts, booleans, event names, and already-safe hashes from audit-specific code.

### Breaking Changes

- While we're in 0.x.y, it is fine to make breaking changes to the GraphQL API, but please only make them when absolutely necessary, and alert the user accordingly.
- Protocol Buffer messages that we are using in our persisted JetStream streams (EVT, RUNTIME_STATE, maybe others) are more stable, and breaking changes to their structure should be avoided. Protocol Buffer messages that are only used for transient communication (live events, etc.) are less stable, and can be changed more freely. (But please consider that changes to these might also lead to changes in the GraphQL API!)
- Treat `GET /api/server` as a higher-stability compatibility surface than the GraphQL API. It is the unauthenticated, cross-origin discovery endpoint used by multi-server clients before they can establish GraphQL access, so breaking its URL, CORS behavior, required JSON fields, or OAuth discovery fields can strand older clients. Prefer additive changes and double-check compatibility before changing this endpoint.

### When making changes...

- Please keep ADRs, FDRs, and other documentation (glossary, docs-website, architecture inventory) up to date.
- When changing core services, projections, EVT events or subjects, NATS resources, or GraphQL operations, use the `chatto-architecture` skill and update `docs/ARCHITECTURE.md`.
- When introducing, renaming, or clarifying canonical vocabulary, use the `glossary` skill and update `docs/GLOSSARY.md`.
- Before pushing a branch for a PR, make sure it is named something descriptive of the change.

### Issues, Commits, and PRs

- Use this project's GitHub Issues for planning work.
- Use Conventional Commit format in commit messages.
- Use Conventional Commit format in PR titles. PR bodies should include a bullet list of changes, ideally with links to relevant FDRs, ADRs, and glossary terms.
- When creating or editing multiline GitHub PR/issue bodies with `gh`, write real Markdown to a file/stdin and use `--body-file`. Do not pass escaped `\n` sequences to `--body`; they render literally. Afterward, verify the stored body with `gh pr view --json body --jq .body` or equivalent before telling the user the PR is ready.
- Please keep ADRs and FDRs up to date.
- When the PR closes an issue, please include this information in the PR title or body (e.g. "Fixes #123") so GitHub can link and auto-close them.
