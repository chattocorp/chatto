# ADR-102: Order Workspace Tasks with Turborepo

**Date:** 2026-09-21

**Updated:** 2026-09-26

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

Cache the API types, Lingua, and Chatto client library builds. Their
TypeScript builds depend on package source, configuration, and the lockfile;
`dist/**` is restored on a cache hit. Changes to mise tool settings,
the pnpm configuration file, or `NODE_OPTIONS` also invalidate the cache.

Also cache the Chatto frontend and Runling builds. The frontend build reads
`CHATTO_BUILD_VERSION` and `CHATTO_FRONTEND_PRECOMPRESS`, so both are declared
task environment inputs. Test, story, end-to-end, and documentation files are
excluded from both task inputs. Turbo restores `build/**` and `dist/**`
respectively. Mise keeps frontend embedding and runs it after every frontend
build task. Mise does not keep separate source lists for these builds.

Do not set `cacheDir`. Turbo then shares the local cache of the main checkout
with every Git worktree, so a new worktree restores unchanged builds. The
cached outputs contain no absolute worktree paths. Disable remote caching and
disable telemetry in the root `pnpm turbo` script; no cache service receives
source, build output, or logs.

Verification tasks remain uncached. Use loose environment mode to retain
existing release, test, and development settings. Keep publishing, signing,
and installed-package tests outside Turbo.

## Consequences

New consumers declare workspace dependencies in `package.json` instead of
adding ordered build chains. Existing root script and mise task names remain
available. Library, frontend, and Runling artifacts can be restored after
their output folders are removed, also in a different worktree. A restored
build takes approximately one second. A warm `mise dev` restart takes a few
seconds longer than with mise source checks, because it always replays the
cached Turbo tasks and copies the embedded frontend. Root checks and tests keep one-task concurrency to bound memory use;
do not run separate SvelteKit tasks concurrently in one checkout.

Use `mise x -- pnpm turbo run check --filter=runling --dry=json` to inspect the
task graph. Use `--force` on a Turbo run to bypass cached results.
Direct package commands require built dependencies. Filtered container builds
that do not install root tooling use pnpm's dependency selector instead.
