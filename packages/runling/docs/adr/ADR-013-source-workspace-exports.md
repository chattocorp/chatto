# ADR-013: Source exports in the workspace

**Status:** Accepted
**Date:** 2026-10-27

## Context

Compiled workspace exports required a build before direct consumer startup and
could run stale code after edits. The source condition did not fix normal
consumer startup, and the console still required compiled assets.

## Decision

Workspace exports point directly to TypeScript. The CLI installs tsx and selects
the source runtime when source files are present. Source hosting uses Vite for
the console; published hosting uses the compiled handler. Both paths share
configuration loading, HTTP ownership, source startup, and shutdown.

pnpm packing applies `publishConfig` to export compiled JavaScript and declarations.
Tarballs contain compiled console assets and no runtime source. Package tests
install and exercise the exact tarball outside the workspace.

## Consequences

Workspace consumers need a TypeScript loader but no build or custom condition.
The CLI supplies the loader. npm consumers retain compiled exports and do not
need development dependencies. Vite initialization temporarily needs the Runling
package directory; the host restores the caller's directory before starting
sources or handling requests. Build and publication workflows still compile
the runtime and console. This extends repository ADR-116 to Runling.
