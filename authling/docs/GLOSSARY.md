# Authling Glossary

This glossary defines canonical Authling terminology. Do not copy Chatto terms
here unless Authling uses the same word with an explicitly defined meaning.

## Product

**Authling** — An independent, self-hostable identity provider. Authling may be
trusted by Chatto servers but is not itself a Chatto server, application-data
store, or a user's home server.

**Account** — Authling's opaque aggregate for one user identity. Its account ID
is the stable OpenID Connect subject (`sub`) exposed to authorized clients. A local
account may have an encrypted, verified email/password credential and one or
more independent browser sessions. Accounts do not yet have profile data or
OIDC grants.

**Local credential** — An Authling login method based on a verified normalized
email address and an Argon2id password verifier. Both values are retained only
inside an encrypted credential payload.

**Signup flow** — Short-lived, encrypted runtime state that carries an email
verification challenge to account creation. It is not an account and expires
after 15 minutes.

**Browser session** — Short-lived, server-side runtime state that binds an
opaque browser cookie to one account after signup or login. A session is not a
durable account fact and can be revoked independently from other sessions.

**Issuer** — The immutable public URL identifying one Authling deployment as
one OpenID Provider. Tokens and discovery use this exact value; changing it is
an identity migration, not an ordinary listener reconfiguration.

**Relying party** — An application that asks Authling to authenticate an
account through OpenID Connect. Its individual protocol identity is an OIDC
client.

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
