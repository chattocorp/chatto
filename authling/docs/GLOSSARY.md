# Authling Glossary

This glossary defines canonical Authling terminology. Do not copy Chatto terms
here unless Authling uses the same word with an explicitly defined meaning.

## Product

**Authling** — An independent, self-hostable identity provider and light
user-controlled metadata service. Authling may be trusted by Chatto servers but
is not itself a Chatto server or a user's home server.

**Account** — Authling's opaque aggregate for one user identity. A local
account may have an encrypted, verified email/password credential. Accounts do
not yet have browser sessions, profile data, or OIDC grants.

**Local credential** — An Authling login method based on a verified normalized
email address and an Argon2id password verifier. Both values are retained only
inside an encrypted credential payload.

**Signup flow** — Short-lived, encrypted runtime state that carries an email
verification challenge to account creation. It is not an account and expires
after 15 minutes.

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
