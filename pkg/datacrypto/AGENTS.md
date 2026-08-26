# Instructions for Agents Working in `pkg/datacrypto/`

Read the root [`AGENTS.md`](../../AGENTS.md),
[`cli/AGENTS.md`](../../cli/AGENTS.md), and
[`authling/AGENTS.md`](../../authling/AGENTS.md) before changing this shared
module. Also follow
[ADR-060](../../docs/adr/ADR-060-application-neutral-data-cryptography.md).

## Boundary

- Keep production code independent of applications.
- This module owns only random 256-bit key generation, XChaCha20-Poly1305
  authenticated encryption, and authenticated wrapping of 256-bit keys.
- Applications own associated-data construction and domain separation, key
  identities and hierarchies, envelope serialization, storage, KMS
  integration, caching, rotation, and erasure policy.
- Do not import Chatto or Authling packages or encode either product's
  identifiers, formats, or policies.
- This module has an independent pre-1.0 version. It has no API stability
  promise.
- The complete module is licensed under Apache-2.0. Keep its source, tests,
  documentation, and standalone license metadata inside that permissive
  boundary.

## Verification

Run:

```sh
mise test-datacrypto
(cd authling && mise test)
mise test-cli
mise license-check
```
