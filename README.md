# chatto

A really good chat application for teams and communities, free and easy to self-host, with [cloud hosting available](https://chatto.run/cloud).

- [Website](https://www.chatto.run)
- [Documentation](https://docs.chatto.run)

## Warning: Alpha Software 🚧

While Chatto is moving forward at a rapid pace, we can't yet give any guarantees about stability, security, or performance; we also at this point can't support data migrations.

We are providing the source code here for transparency and to allow early adopters to experiment and provide feedback. If you choose to actually run it, **be prepared to lose some or all of your data at any time**.

A lot of projects say this and people often ignore it, so let me spell things out a bit more:

- You **will** lose runtime and permission configuration and will be required to manually fix things.
- You **will** lose data for experimental features that we decide to remove or significantly change.
- You **will** experience breaking changes in the GraphQL API.
- You **will** lose user and message data to bugs, or if we need to make breaking changes to the data model.

It should be no surprise that we are working hard to move towards a release that can give better guarantees, but we're not there yet.

## Development with Conductor

[Conductor](https://conductor.build) workspaces run the dev stack natively via `mise dev` — no Docker. The `run` script in `conductor.json` wires Conductor's assigned `$CONDUCTOR_PORT` (and `+1` / `+2`) into the env vars `mise dev` reads:

| Port              | Process                              |
| ----------------- | ------------------------------------ |
| `$CONDUCTOR_PORT` | Vite dev server (user-facing URL)    |
| `+1`              | Go backend (`CHATTO_WEBSERVER_PORT`) |
| `+2`              | Embedded NATS                        |

Outside Conductor, plain `mise dev` uses the defaults from `cli/chatto.toml` (Vite 5173, backend 4000, NATS 4555).

Each instance is bootstrapped with the same dev credentials (configured in `cli/chatto.toml` under `[[bootstrap.users]]`):

- **Login:** `alice`
- **Email:** `alice@example.com`
- **Password:** `foobar123`

## Instructions for Coding Agents

This codebase keeps agent-relevant context in five places. Read the one that fits your task:

- **`.claude/rules/**`** — always-on coding, testing, and review conventions, mostly path-scoped (`frontend.md` and `frontend-conventions.md` for SvelteKit work, `backend.md` for Go, `testing-frontend.md` / `testing-backend.md` for tests, `authorization.md` for permission changes, etc.). Start here for "how do we do things in this repo?"
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

Use this project's GitHub Issues for planning work.

## License

Chatto is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](LICENSE). This means:

- You are free to use, modify, and distribute Chatto.
- If you run a modified version as a network service, you must make the source code of your modifications available to its users.
- Any derivative work must also be licensed under the AGPL-3.0.

For full details, see the [LICENSE](LICENSE) file or run `chatto license`.

## Contributing

This project is **not accepting outside contributions** at this time. If you have feedback, bug reports, or ideas, please [get in touch](mailto:hendrik@mans.de) — we'd love to hear from you.
