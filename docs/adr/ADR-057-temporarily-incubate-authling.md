# ADR-057: Temporarily Incubate Authling in the Chatto Repository

**Date:** 2026-07-30

## Context

Chatto's multi-server client needs an optional shared identity provider and a
small amount of user-owned metadata storage. This product, named Authling,
should use the same NATS, JetStream, and event-sourcing mechanics as Chatto.
ADR-056 already identifies an extractable framework boundary, but that boundary
needs a second application to keep its API grounded.

Authling is operationally distinct from a Chatto server. An identity provider
has a different trust boundary, release lifecycle, and availability profile.
It must also use its own NATS account rather than sharing Chatto's application
account. Embedding Authling into Chatto is not an initial requirement, although
the architecture should not make future process-level composition impossible.

## Decision

Temporarily incubate Authling in this repository as a separate Go module under
`authling/`. The repository-level `go.work` file composes the Chatto, Authling,
shared `pkg/events/`, shared `pkg/natsruntime/`, shared `pkg/datacrypto/`, and
shared `pkg/appconfig/` modules for local development without merging their
module or package boundaries.

Authling is a separate product and executable. It owns its configuration,
application composition, HTTP surface, lifecycle, and NATS credentials. It
must not import Chatto domain packages or any package beneath Chatto's
`internal` directories. Reusable event-sourcing mechanics live behind the
independently versioned but unstable `hmans.de/chatto/pkg/events` module
boundary described by ADR-056. Reusable embedded NATS lifecycle mechanics live
in the separate `hmans.de/chatto/pkg/natsruntime` module described by ADR-058.
Reusable authenticated-encryption primitives live in the separate
`hmans.de/chatto/pkg/datacrypto` module described by ADR-060.
Reusable TOML and environment configuration loading lives in the separate
`hmans.de/chatto/pkg/appconfig` module described by ADR-061.
Authling should consume shared modules only when a concrete use case needs
them.

Authling always operates through a dedicated NATS account. A future embedding
adapter may let another process construct and mount Authling's runtime, but the
adapter must supply Authling's own NATS connection and preserve the same
application boundary as the standalone executable. Chatto does not embed
Authling initially.

Chatto, Authling, and the shared modules have independent Release Please
components, versions, changelogs, release pull requests, and tags:

- Chatto remains the root component and uses `v<version>` tags. Its release
  component excludes commits whose files are entirely under `authling/`,
  `pkg/events/`, `pkg/natsruntime/`, `pkg/datacrypto/`, `pkg/appconfig/`, or
  `.agents/skills/`.
- Authling uses the `authling/` component and `authling/v<version>` tags. The
  slash follows Go's nested-module tag convention and keeps module versions
  consumable through normal Go tooling.
- The events framework uses the `pkg/events/` component and
  `pkg/events/v<version>` tags, matching its nested-module repository path.
- The embedded NATS runtime uses the `pkg/natsruntime/` component and
  `pkg/natsruntime/v<version>` tags, matching its nested-module repository
  path.
- The data-cryptography module uses the `pkg/datacrypto/` component and
  `pkg/datacrypto/v<version>` tags, matching its nested-module repository path.
- The application-configuration module uses the `pkg/appconfig/` component and
  `pkg/appconfig/v<version>` tags, matching its nested-module repository path.

Release Please does not infer dependencies between Go workspace modules. A
change to a shared module that requires a new Chatto or Authling version must
also update that consumer's module dependency or another file under the
consumer's component, making its release explicit.

Authling uses its intended standalone module identity, `hmans.de/authling`,
while incubating here. Its module path must not inherit Chatto's namespace;
moving repositories must not require an import-path migration.

Co-location ends when all of the following are true:

- the framework needed by both products is available through a stable,
  application-neutral, versioned module boundary;
- routine Authling feature work no longer requires frequent atomic commits to
  Chatto or its framework incubator; and
- Authling's CI, releases, documentation, and agent instructions can move
  without depending on Chatto-specific repository paths or automation.

At that point, move `authling/` to a dedicated repository while preserving its
history. Until then, keep Authling-owned implementation and documentation
inside its subtree and limit root-level integration to workspace, CI, release,
instruction, and shared-framework needs.

## Consequences

Authling can drive framework extraction with fast local cross-module
development while remaining independently deployable and releasable. Its
security and operational lifecycle cannot become accidentally coupled to a
Chatto server or Chatto's NATS account.

The repository now contains six Go modules and six release lines. CI and
developer tasks must cover the relevant modules, and release automation must
preserve the separate tag namespaces.

Temporary co-location creates an ongoing review obligation: new dependencies,
documentation, automation, and package paths must be checked for whether they
make the planned repository split harder. Some repository-wide setup will need
to be recreated when Authling moves.

Keeping future embedding possible requires dependency-injected composition
boundaries, but no embedding abstraction is built before a real use case
requires it.
