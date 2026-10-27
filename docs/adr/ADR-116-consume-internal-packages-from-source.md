# ADR-116: Consume Internal TypeScript Packages from Source

**Date:** 2026-10-18

## Context

The workspace libraries `@chatto/api-types`, `@chatto/translatto`, and
`@chatto/client` exported compiled `dist/` output. Only `@chatto/client` also
exported its source, under the `@chatto/source` condition.
[ADR-102](ADR-102-turborepo-workspace-tasks.md) removed package builds from
`mise setup`. Turbo builds them before root checks and tests, but a direct
command, such as `vitest` in `apps/frontend`, did not build them. In a new
worktree, these commands failed until a different task built the packages.
When package source changed, the same commands used stale output.

At the time of this decision, we did not publish these packages. All consumers
were in this repository: the bundled frontend, ChattoBot, the Runling bot example, and the
package tests. Each consumer already runs through a tool that compiles
TypeScript: Vite and Vitest, svelte-check, `tsc`, jiti through Runling, or tsx.

## Decision

In the workspace, the three packages export their TypeScript source. Their
`exports` entries point to `src/` and have no `dist/` alternative. Their
`check` scripts type-check the source without emitting files. Consumers read
the source directly. No export condition or compiled output is necessary.

Every consumer must load TypeScript through a compiling tool. Plain Node cannot
load this source: generated protobuf code contains TypeScript enums, and the
source imports modules with `.js` specifiers. Node's type stripping supports
neither. Node processes that load a package, such as the Runling bot example
tests and the end-to-end bot test, use `node --import tsx`.

Consumers type-check the package source with their own compiler and settings.
Package source must therefore pass the strictest consumer settings. Today,
ChattoBot adds `noUncheckedIndexedAccess`, `noImplicitOverride`, and
`noFallthroughCasesInSwitch`. The client check applies these settings to its
production source with `tsconfig.strict.json`, so these flags cannot fail
ChattoBot's check later. Specs and test helpers keep the base settings. Other
differences in consumer settings, such as Node types, can still fail a
consumer check; `mise check-workspace` finds them. Source consumers use the
same TypeScript major version as the packages.

Runling was originally outside this decision. Its own
[ADR-013](../../packages/runling/docs/adr/ADR-013-source-workspace-exports.md)
now uses source exports in the workspace and retains compiled npm packages.

Translatto uses
`build` to emit JavaScript and type declarations. pnpm `pack` applies
`publishConfig` to replace the source exports in the published manifest. A
package test installs the tarball outside the workspace and checks Node ESM
imports and TypeScript declarations. Workspace consumers still use source.
The client also has a publication build. It includes the private generated
API types, rewrites their output imports to relative paths, and keeps their
Apache-2.0 license. pnpm packs a separate publication directory with compiled
exports and no dependency on the private package. Workspace links still use
source. Its package test checks a clean installation, Node imports with and
without Svelte, and TypeScript declarations.

The current Turbo `^build` dependencies also run these builds before dependent
package tasks. Removing these unnecessary builds is a follow-up task.

## Consequences

A new worktree can run any frontend, client, or bot command after
`mise setup`. Changes to package source take effect immediately, also in
direct Vitest and svelte-check runs. The API types have no build task.
Translatto and the client have builds for npm publication. `mise codegen-proto` only
generates source.

Consumer type checks include the source of the packages that they import. This
makes those checks slightly slower. A change to package code can fail a
consumer check that the package check accepts, when the consumer uses stricter
settings.

The Translatto and client package tests cover their compiled npm output.
The end-to-end bot test runs the workspace client in a Node host through tsx.
