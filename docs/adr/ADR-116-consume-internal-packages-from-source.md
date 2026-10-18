# ADR-116: Consume Internal TypeScript Packages from Source

**Date:** 2026-10-18

## Context

The workspace libraries `@chatto/api-types`, `@chatto/lingua`, and
`@chatto/client` exported compiled `dist/` output. Only `@chatto/client` also
exported its source, under the `@chatto/source` condition.
[ADR-102](ADR-102-turborepo-workspace-tasks.md) removed package builds from
`mise setup`. Turbo builds them before root checks and tests, but a direct
command, such as `vitest` in `apps/frontend`, did not build them. In a new
worktree, these commands failed until a different task built the packages.
When package source changed, the same commands used stale output.

We do not publish these packages. All of their consumers are in this
repository: the bundled frontend, ChattoBot, the Runling bot example, and the
package tests. Each consumer already runs through a tool that compiles
TypeScript: Vite and Vitest, svelte-check, `tsc`, jiti through Runling, or tsx.

## Decision

The three packages export their TypeScript source. Their `exports` entries
point to `src/` and have no `dist/` alternative. The packages have no build
script. Their `check` scripts type-check the source without emitting files.
Consumers read the source directly. No export condition, build task, or setup
step is necessary.

Every consumer must load TypeScript through a compiling tool. Plain Node cannot
load this source: generated protobuf code contains TypeScript enums, and the
source imports modules with `.js` specifiers. Node's type stripping supports
neither. Node processes that load a package, such as the Runling bot example
tests and the end-to-end bot test, use `node --import tsx`.

Consumers type-check the package source with their own compiler and settings.
Package source must therefore pass the strictest consumer settings. Today,
ChattoBot adds `noUncheckedIndexedAccess`, `noImplicitOverride`, and
`noFallthroughCasesInSwitch`. Source consumers use the same TypeScript major
version as the packages. `mise check-workspace` runs every consumer check, so
it finds code that does not pass these settings.

Runling is not in scope. It is published to npm and keeps its build.

When we publish a package, add a build at that time. Use `publishConfig` to
replace `exports` with the compiled output in the published manifest, and add
a test that installs the packed package in a host outside the workspace.

## Consequences

A new worktree can run any frontend, client, or bot command after
`mise setup`. Changes to package source take effect immediately, also in
direct Vitest and svelte-check runs. The API types, Lingua, and client build
tasks are removed from Turbo, mise, and the root scripts. `mise codegen-proto`
only generates source.

Consumer type checks include the source of the packages that they import. This
makes those checks slightly slower. A change to package code can fail a
consumer check that the package check accepts, when the consumer uses stricter
settings.

No test covers compiled package output, because no consumer uses it. The
end-to-end bot test runs the client in a Node host through tsx. It does not
show that a compiled package works for third-party bots. Add that test with
the first published build.
