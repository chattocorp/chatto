# Authling Glossary

This glossary defines canonical Authling terminology. Do not copy Chatto terms
here unless Authling uses the same word with an explicitly defined meaning.

## Product

**Authling** — An independent, self-hostable identity provider and light
user-controlled metadata service. Authling may be trusted by Chatto servers but
is not itself a Chatto server or a user's home server.

**Account** — Authling's opaque aggregate for one future user identity. The
current account record contains only an opaque identifier and creation time; it
does not yet contain login methods, profile data, sessions, or OIDC grants.

## Data protection

**Cryptographic erasure** — Making encrypted Authling data permanently
unreadable by irreversibly destroying the key material required to decrypt it.
Authling uses this to erase data from immutable event history without rewriting
that history.

**Data key** — A random, purpose- and epoch-scoped encryption key used to
encrypt sensitive account, credential, or application data. Data keys are
wrapped by a user key and referenced opaquely from durable events.

**User key** — An account-scoped, KMS-managed wrapping key. Destroying a user
key makes all data keys wrapped by it unusable and provides Authling's
account-wide cryptographic-erasure boundary.
