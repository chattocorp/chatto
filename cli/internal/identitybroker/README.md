# Cross-server identity brokerage proof of concept

This package tests whether independent Chatto servers can broker a portable
identity group without asking users to create, transfer, or recover an identity
key. It is an isolated protocol experiment, not a production Chatto feature.

The intended reader is a Chatto maintainer evaluating the protocol and its
security boundary before choosing public APIs, persisted protobufs, or user
experience.

## Result

The PoC demonstrates a linear identity group whose membership can be verified
by a clean client from public certificates:

- Two accounts on distinct servers create the group.
- Every later member is approved by its target server and two current members
  on distinct servers.
- Each participating server signs the exact group, account, role, challenge,
  issuance, and expiry fields.
- A disposable client ceremony key binds intercepted artifacts to one ceremony.
  The key is discarded after completion and is not part of durable client
  state.
- A verifier reconstructs membership from credential references rather than
  trusting certificate order or mutable profile fields.
- Twenty accounts require nineteen unique certificates: one genesis plus
  eighteen membership certificates.

The HTTP integration test places every broker behind a distinct loopback
origin. A new trust store then discovers the public server keys, fetches the
completed certificates, and reconstructs all twenty members without receiving
any ceremony private key.

## Protocol artifacts

### Challenge

A server issues a short-lived challenge only after authenticating its local
account. The challenge binds an opaque account ID, certificate kind, approval
role, nonce, and expiry.

### Statement

The client constructs one canonical statement containing all participants and
their challenges. The statement uses stable opaque user IDs and exact server
origins; usernames, display names, and email addresses are not signed identity
inputs.

The PoC uses an explicitly length-prefixed binary encoding for signatures. It
does not rely on JSON field order or protobuf serialization behavior.

### Ceremony request

The client signs the statement with a newly generated disposable Ed25519 key.
Every server verifies this signature before issuing its approval. An attacker
who intercepts an unsigned or partially signed request cannot alter it or
complete it through another client.

### Server approval

Each server signs its exact account and role over the statement ID. A complete
certificate contains one approval for every named participant and no duplicate
approvals.

### Membership credential

The genesis certificate grants one credential to each founder. A later
membership certificate grants one credential to its target and identifies the
two existing credentials that sponsored it. Sponsor credentials must:

- Belong to the named sponsor accounts.
- Come from distinct server origins.
- Be active when the membership was issued.
- Belong to the same identity group.

Credential references form a directed acyclic dependency graph. Unresolved or
cyclic sponsorship is rejected.

### Revocation

A member can issue a signed revocation for its own credential. Revocation is
durable and does not expire. The verifier excludes the credential after the
revocation time and rejects memberships sponsored after a sponsor was revoked.

The PoC does not yet define quorum removal, group recovery, or group epochs.

## Security boundary

The PoC establishes the following limited claim:

> A complete certificate records one client-mediated ceremony in which all
> named servers approved their named local accounts and roles.

It protects against:

- Modification of a server origin, account ID, role, group ID, nonce, issuance,
  or expiry after signing.
- Reusing one challenge for a different statement.
- Completing an intercepted ceremony without its disposable client key.
- Extending a group through one existing malicious server alone, because two
  current member credentials on distinct origins must sponsor a new member.
- Treating certificate input order as authority.

It does not protect against:

- Two colluding current member servers.
- A compromised client during an active ceremony.
- Two compromised sponsor accounts on otherwise honest servers.
- A server lying about how it authenticated its own local account.
- Correlation after a certificate has been disclosed.
- Stale status when servers are unavailable.
- Server signing-key loss, rotation, or origin migration.

The certificate proves account-control continuity as attested by Chatto
servers. It does not prove a legal identity, a unique human, employment, or
control of an email address.

## Production mapping

The PoC keeps all state in memory so protocol changes remain cheap. A production
implementation must follow Chatto's existing storage boundaries.

| PoC state | Production boundary |
| --- | --- |
| Unconsumed challenge | `RUNTIME_STATE` record with a short per-key TTL |
| Idempotent issued approval | `RUNTIME_STATE` until the ceremony completes or expires |
| Completed genesis or membership | Durable fact on the local user aggregate in `EVT` |
| Revocation | Durable fact on the local user aggregate in `EVT` |
| Current identity-group view | Replay-safe projection derived from `EVT` |
| Server signing private key | Dedicated protected key lifecycle, deliberately undecided |
| Server signing public keys | Origin-bound discovery metadata with historical rotation support |

Production writes would require JetStream optimistic concurrency over the local
user identity-link event family, projection catch-up before returning, and
idempotent finalization. Peer notification would be a retryable post-commit side
effect, not the durable source of truth.

No production implementation should copy the PoC's process-local challenge,
approval, or certificate maps.

## Open questions

The PoC deliberately leaves these decisions open:

- How server signing keys are generated, protected, backed up, rotated, and
  retained for historical verification.
- Whether membership certificates expire, renew automatically, or use explicit
  group epochs.
- How a user recovers when fewer than two existing member accounts remain
  accessible.
- Whether a certificate is private, visible to DM peers, server-member-visible,
  or public.
- How clients obtain the supporting certificate chain without publishing the
  complete identity group.
- Whether approval should use a disposable signature key, PKCE verifier, or a
  standardized sender-constrained authorization mechanism.
- Which ConnectRPC package and stability tier should expose future operations.
- How mixed-version servers negotiate protocol and canonical-encoding versions.

These questions should be resolved through an ADR/FDR before the experiment is
integrated into Chatto's public API or durable core protobufs.

## Verification

Run the focused suite from `cli/`:

```sh
mise x -- go test ./internal/identitybroker -count=1 -timeout 60s
```
