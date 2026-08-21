# Authling Glossary

This glossary defines canonical Authling terminology. Do not copy Chatto terms
here unless Authling uses the same word with an explicitly defined meaning.

## Product

**Authling** — An independent, self-hostable identity provider. Authling may be
trusted by Chatto servers but is not itself a Chatto server, application-data
store, or a user's home server.

**Account** — Authling's opaque aggregate for one user identity. Its account ID
is the stable OpenID Connect subject (`sub`) exposed to authorized clients. A local
account may have an encrypted, verified email/password credential, one or more
independent browser sessions, and durable OIDC authorization grants. Accounts
do not yet have profile data.

**Local credential** — An Authling login method based on a verified normalized
email address and an Argon2id password verifier. Both values are retained only
inside an encrypted credential payload.

**Signup flow** — Short-lived, encrypted runtime state that carries an email
verification challenge to account creation. It is not an account and expires
after 15 minutes.

**Password reset flow** — Short-lived, encrypted runtime state that carries an
email challenge and binds successful recovery to the password credential that
was current when the flow began. It expires after 15 minutes and is not a
durable account fact.

**Signed-in password change** — A local-credential rotation authorized by an
active browser session and the current password. It preserves the account ID,
verified email, and OIDC `sub` while invalidating older Authling browser
sessions.

**Email change flow** — Short-lived, encrypted runtime state that binds a
signed-in account and reauthenticated credential to its old and requested new
addresses and an email challenge. It expires after 15 minutes and is not a
durable account fact. The first credential mutation makes other flows bound to
the older credential stale.

**Browser session** — Short-lived, server-side runtime state that binds an
opaque browser cookie to one account after signup or login. A session is not a
durable account fact and can be revoked independently from other sessions.
Password reset, signed-in password change, and verified email change invalidate
every session issued under the account's older authentication version. The
account page enumerates active sessions through a disposable in-memory
inventory and uses a separate opaque, non-bearer session ID for remote
revocation; Authling does not retain browser, IP, or location metadata for the
inventory.

**Issuer** — The immutable public URL identifying one Authling deployment as
one OpenID Provider. Tokens and discovery use this exact value; changing it is
an identity migration, not an ordinary listener reconfiguration.

**OIDC signing key** — An asymmetric RS256 key whose private part signs
Authling ID tokens and whose public part appears in JWKS under its fingerprint
`kid`. Authling automatically moves keys through prepared, active, retiring,
and retired lifecycle states without changing the issuer or account subjects.

**Relying party** — An application that asks Authling to authenticate an
account through OpenID Connect. Its individual protocol identity is an OIDC
client.

**OIDC client** — One exact protocol identity identified by `client_id` and
validated from Authling configuration or a CIMD document. Authling initially
treats every client ID as a separate account-facing application; a future
relying-party identity may deliberately group multiple clients.

**Authorization grant** — A durable, account-owned authorization for one exact
OIDC client and scope set. It lets a covered future request skip repeated
consent and can be revoked independently of Authling browser sessions. Grant
revocation does not enumerate or terminate already issued short-lived tokens
or the relying party's own sessions.

**Client ID Metadata Document (CIMD)** — An HTTPS JSON document whose URL is
also a public OIDC client's identifier. Authling uses CIMD for automatic,
read-only client onboarding without a dynamic registration write API.

## Backend

**Loom Architecture** — Repository-wide event-sourced architecture adopted by
Authling, built around one authoritative event log, disposable
materializations, and durable outcomes. See
[root ADR-073](../../docs/adr/ADR-073-define-the-loom-architecture.md) and
[Authling ADR-001](adr/ADR-001-event-sourced-nats-architecture.md).

**Materialization** — Loom term for disposable state derived from the event
log; Authling's in-memory projections are materializations. See
[root ADR-073](../../docs/adr/ADR-073-define-the-loom-architecture.md).

**Outcome** — Loom term for reliable asynchronous work caused by a committed
event and performed by a durable worker. See
[root ADR-073](../../docs/adr/ADR-073-define-the-loom-architecture.md).

## Data protection

**Cryptographic erasure** — Making encrypted Authling data permanently
unreadable by irreversibly destroying the key material required to decrypt it.
Authling uses this to erase data from immutable event history without rewriting
that history.

**Data key** — A random encryption key used to encrypt sensitive credential
data. Data keys are wrapped by a user key and referenced opaquely from durable
events.

**User key** — An account-scoped, KMS-managed wrapping key. Destroying a user
key makes all data keys wrapped by it unusable and provides Authling's
account-wide cryptographic-erasure boundary.
