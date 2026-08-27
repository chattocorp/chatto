# ADR-060: Extract Application-Neutral Data Cryptography

**Date:** 2026-07-31

## Context

Chatto already protects message bodies and user data with random 256-bit keys,
XChaCha20-Poly1305, authenticated key wrapping, and product-specific associated
data. Authling ADR-002 chooses the same primitives for its planned hierarchical
user and data keys. Keeping two implementations would duplicate
security-sensitive code, while moving Chatto's complete encryption subsystem
would wrongly couple a shared package to Chatto's persisted formats, key
references, KMS boundary, storage, caching, and cryptographic-erasure policy.

The extraction must preserve every existing Chatto ciphertext, nonce, and
associated-data byte. Authling does not yet persist protected user data, so it
needs a concrete consumer contract without representing the planned hierarchy
as implemented runtime behavior.

## Decision

Create the independently versioned `hmans.de/chatto/pkg/datacrypto` module. It
owns only:

- cryptographically random 256-bit key generation;
- XChaCha20-Poly1305 seal and open operations with caller-supplied associated
  data; and
- authenticated wrapping and unwrapping of 256-bit keys.

The module does not construct associated data or serialize envelopes.
Applications own domain separation, identifiers, key hierarchies and
references, epochs and purposes, persistence, KMS integration, caching,
rotation, deletion, and erasure workflows. Its API returns ciphertext and
nonce as separate byte slices so consumers retain their existing storage
formats.

Chatto keeps `cli/internal/encryption` as its product facade. The facade
delegates XChaCha encryption, key wrapping, and key generation to the shared
module while retaining Chatto's exact associated-data prefixes and envelope
fields. Its legacy 96-bit-nonce ChaCha20-Poly1305 path remains internal for
persisted-data compatibility. Existing error sentinels alias the shared
sentinels so `errors.Is` compatibility is preserved.

Authling establishes the second consumer through a test-only external-package
contract. The contract exercises the hierarchy and substitution resistance
from Authling ADR-002 with Authling-owned associated data; it does not add a
production encryption subsystem or claim protected data is implemented.

License the complete module under Apache-2.0 in accordance with ADR-059. It is
a pre-1.0 incubation surface without an API stability promise.

## Consequences

Chatto's facade and Authling's planned-hierarchy consumer contract exercise one
small, independently testable implementation of the cryptographic primitives
while keeping product policy and durable formats out of the shared boundary.
Raw-cipher compatibility tests protect Chatto's existing data from accidental
format drift, and Authling's contract tests protect the package from becoming
shaped only around Chatto.

The module deliberately does not yet extract KMS interfaces, wrapped-key
records, caches, key stores, or erasure orchestration. Those boundaries need a
second concrete implementation before extraction. Callers must construct
unambiguous, versioned associated data and persist nonces alongside
ciphertexts; the shared package cannot enforce product-specific correctness.
