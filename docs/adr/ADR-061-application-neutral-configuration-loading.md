# ADR-061: Extract Application-Neutral Configuration Loading

**Date:** 2026-07-31

## Context

Chatto and Authling both load an application-owned Go configuration struct
from a conventional TOML file and then apply environment-variable overrides.
The products duplicated the file selection, TOML decoding, and environment
parsing mechanics, but intentionally differ in compatibility policy: Chatto
allows missing explicitly selected files and ignores unknown TOML fields,
while Authling requires explicitly selected files and rejects unknown fields.

The products also have different schemas, environment names, defaults,
normalization, validation, compatibility aliases, and configuration generators.
Moving those policies into a shared package would create coupling and could
silently break existing deployments.

## Decision

Create the independently versioned `hmans.de/chatto/pkg/appconfig` module. It
owns only:

- selecting an explicit configuration path or an application-supplied default;
- optionally requiring an explicitly selected file to exist;
- decoding TOML, with a caller-selected unknown-field policy; and
- applying environment overrides described by the target struct's `env` tags.

The generic loader returns the fully populated target value only on success.
On file, TOML, or environment failure it returns the target type's zero value,
preventing callers from accidentally continuing with partially decoded
configuration.

Chatto keeps permissive missing-file and unknown-field behavior, along with its
provider-environment compatibility hook, defaults, normalization, validation,
schema, and generated configuration. Authling keeps its stricter explicit-file
and unknown-field behavior, defaults, validation, schema, and environment
namespace.

The module does not own command-line flags, watch/reload behavior, secret
resolution, logging, or application lifecycle. License the complete module
under Apache-2.0 in accordance with ADR-059. It remains pre-1.0 without an API
stability promise.

## Consequences

The shared precedence and error-handling mechanics have one independently
tested implementation, while each product retains its existing operator-facing
configuration contract. New consumers can opt into strict behavior without
forcing it on compatibility-sensitive applications.

The option surface reflects concrete differences between Chatto and Authling
rather than attempting to model every configuration system. Future features
such as secret providers, live reload, generated examples, or remote sources
remain product-owned until multiple consumers establish another useful
boundary.
