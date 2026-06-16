# Project Overview

Chatto is a real-time chat application with a GraphQL gateway backed by NATS/JetStream. Single Go executable with embedded NATS server + web server; SvelteKit frontend connects via GraphQL queries/mutations/subscriptions.

## Key Files

- `docs/ARCHITECTURE.md` - Source of truth for architecture (streams, KV buckets, data flow)
- `docs/adr/` - Architecture Decision Records explaining _why_ key decisions were made

## Monorepo Structure

- `cli/` - Go backend (Gin, gqlgen, embedded NATS)
- `frontend/` - SvelteKit SPA (Svelte 5 runes, urql, TailwindCSS v4)
- `proto/` - Protocol Buffer definitions
- `examples/` - Various example configurations for self-hosting Chatto

## Basic Architecutre

- NATS for pubsub and persistent storage (JetStream)
- "ChattoCore", a Go package with low-level domain logic that interacts with NATS. Receives information on the actor, but does not perform authorization.
- GraphQL gateway (gqlgen) that calls ChattoCore directly, performing both authentication and authorization before calling Core methods.

## Running Tools

**IMPORTANT:** Tools like `go`, `pnpm`, `node`, etc. are managed by mise and are NOT directly available in PATH. You must use `mise x --` to run them:

```bash
# Correct - use mise x to run tools
mise x -- pnpm install
mise x -- go test ./...
mise x -- node script.js

# Incorrect - tools are not in PATH
pnpm install        # Will fail
go test ./...       # Will fail
```

Prefer using mise tasks (below) when available, as they handle this automatically.

## Commands

All commands via mise task runner. Run from project root.

| Command                      | Description                                   |
| ---------------------------- | --------------------------------------------- |
| `mise x -- tilt up --stream` | Run full dev environment (backend + frontend) |
| `mise test`                  | Run all tests                                 |
| `mise test-cli`              | Run Go tests only                             |
| `mise test-frontend`         | Run frontend tests only                       |
| `mise build`                 | Build for all architectures                   |
| `mise codegen`               | Run all code generation                       |
| `mise codegen-cli`           | Generate Go code (GraphQL + proto)            |
| `mise codegen-frontend`      | Generate frontend GraphQL types               |
| `mise clean`                 | Remove build artifacts                        |
| `mise docs`                  | Start pkgsite documentation server            |

Releases are driven by [release-please](https://github.com/googleapis/release-please). Conventional-commit PRs on `main` accumulate in a rolling release PR using `.release-please-config-main.json`; merging it cuts the next mainline `vX.Y.Z` tag. Maintenance branches (`release-[0-9]*`) use `.release-please-config.json` for stable patch releases.

## Reference Links

- [NATS Go SDK](https://pkg.go.dev/github.com/nats-io/nats.go)
- [JetStream Go SDK](https://pkg.go.dev/github.com/nats-io/nats.go/jetstream)
- [gqlgen](https://gqlgen.com/)
