# ADR-102: Order Workspace Tasks with Turborepo

**Date:** 2026-09-21

**Status:** Accepted

## Context

Root scripts and mise tasks repeat the order in which shared TypeScript
packages must build. Adding a consumer requires several script changes.
Repeated checks compile the same dependencies again.

## Decision

Use Turborepo for JavaScript workspace task ordering. Root scripts select a
task and package; `dependsOn: ["^build"]` builds its workspace dependencies
first. Package scripts remain local commands. pnpm owns dependency installation.
mise owns tool versions, Go tasks, native packaging, and supervised services.
Authling retains its independent workspace and tasks.

Make the local Runling bot an explicit workspace package. Keep its Runling,
Chatto client, and HTTP dependencies there, rather than at the root: Turbo
includes root workspace dependencies in every task's cache key.
The bot has a test script but no check or lint script. Its empty check and
lint task definitions prevent Turbo's virtual tasks from building the Runling
console during unrelated workspace checks.

Cache only the API types, Lingua, and Chatto client library builds initially.
Their TypeScript builds depend on package source, configuration, and the
lockfile; `dist/**` is restored on a cache hit. Changes to mise tool settings,
the pnpm configuration file, or `NODE_OPTIONS` also invalidate the cache.
Use a workspace-local `.turbo/cache`. Disable remote caching and disable
telemetry in the root `pnpm turbo` script; no cache service receives source,
build output, or logs.

App builds and verification tasks remain uncached. Use loose environment mode
to retain existing release, test, and development settings. Cache app builds
only after their environment variables, generated files, and external inputs
are fully declared. Keep frontend embedding in mise, including its existing
source/output skip check. Keep publishing, signing, and installed-package
tests outside Turbo.

## Consequences

New consumers declare workspace dependencies in `package.json` instead of
adding ordered build chains. Existing root script and mise task names remain
available. Library artifacts can be restored after their output folders are
removed. Root checks and tests keep one-task concurrency to bound memory use;
do not run separate SvelteKit tasks concurrently in one checkout.

Use `mise x -- pnpm turbo run check --filter=runling --dry=json` to inspect the
task graph. Use `--force` on a Turbo run to bypass cached library results.
Direct package commands require built dependencies. Filtered container builds
that do not install root tooling use pnpm's dependency selector instead.
