# ADR-059: License Shared Framework Modules under Apache-2.0

**Date:** 2026-07-31

## Context

ADR-056, ADR-058, and ADR-060 establish `pkg/events`, `pkg/natsruntime`, and
`pkg/datacrypto` as independently versioned, application-neutral modules
intended for use outside Chatto and Authling. The first two modules began under
the repository's default AGPL-3.0-or-later license while their boundaries were
being identified; `pkg/datacrypto` begins under Apache-2.0.

The modules need additional consumers to mature. Requiring an application's
combined work to follow the repository's strong copyleft terms would
discourage otherwise useful evaluation and reuse. API maturity is a separate
concern: a permissive license need not imply a compatibility guarantee.

ChattoCorp GmbH owns the current module contributions, so the license boundary
can be established before outside contributions make a later change harder.

## Decision

License the complete `pkg/events`, `pkg/natsruntime`, and `pkg/datacrypto`
modules under the Apache License 2.0. This includes their source, tests,
documentation, module metadata, and standalone license files.

The modules remain independently versioned, pre-1.0 incubation surfaces with
no API stability promise. Their README files must state both the permissive
license and the unstable API status.

Chatto, Authling, and the rest of their product-owned server code remain under
their existing licenses. Consuming an Apache-2.0 shared module does not change
the license of either application.

Earlier AGPL-3.0-or-later grants for these files are not withdrawn. Current and
future module versions are offered under Apache-2.0 through the module-specific
license and repository SPDX metadata.

## Consequences

External applications can evaluate, embed, modify, and redistribute the shared
framework modules under a widely used permissive license with an explicit
patent grant. This should broaden the feedback available while their APIs
mature.

Downstream users are not required to publish modifications. Improvements must
be attracted through useful APIs, documentation, and project stewardship
rather than copyleft requirements.

The repository has a deliberate mixed-license boundary. REUSE metadata,
module-local license files, public README files, and agent instructions must
keep the shared modules Apache-2.0 while preserving AGPL-3.0-or-later as
the default for product-owned server code.
