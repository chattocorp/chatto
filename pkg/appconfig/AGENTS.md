# Instructions for Agents Working in `pkg/appconfig/`

Read the repository-root [`AGENTS.md`](../../AGENTS.md),
[`cli/AGENTS.md`](../../cli/AGENTS.md), and
[`authling/AGENTS.md`](../../authling/AGENTS.md) before changing this shared
module. Also follow
[ADR-061](../../docs/adr/ADR-061-application-neutral-configuration-loading.md).

## Boundary

- Keep production code application-neutral.
- This module owns only TOML file selection and decoding followed by
  struct-tagged environment-variable overrides.
- Applications own their configuration types, field and environment names,
  defaults, normalization, validation, compatibility aliases, generated
  examples, and command-line flag policy.
- Preserve consumer compatibility through explicit loader options. Do not
  tighten or relax a product's unknown-field or missing-file behavior as a
  side effect of framework work.
- Do not import Chatto or Authling packages or encode either product's paths,
  prefixes, schemas, or policies.
- The module is independently versioned but pre-1.0 and has no API stability
  promise yet.
- The complete module is licensed under Apache-2.0.

## Verification

Run:

```sh
mise test-appconfig
mise test-authling
mise test-cli
mise license-check
```
