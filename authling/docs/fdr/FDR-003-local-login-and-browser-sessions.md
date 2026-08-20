# FDR-003: Local Login and Browser Sessions

**Status:** Experimental
**Last reviewed:** 2026-08-14

## Overview

Authling lets a person authenticate a local account with its verified email
address and password, continue through a short-lived first-party browser
session, and explicitly sign out. Successful account creation starts the same
kind of session so signup and later login lead to one consistent signed-in
experience.

## Behavior

- The login page accepts a verified email address and password. Email
  comparison uses the same trimming and lowercasing rules as signup.
- Missing accounts, wrong passwords, and throttled accounts receive the same
  browser error. Unknown accounts still perform password-verifier work so
  response timing does not provide a simple account-existence oracle.
- Ten failed attempts for one normalized address prevent further login for 15
  minutes after the tenth recorded failure. The limit is shared by Authling
  replicas. Password-verification concurrency is also bounded per process.
- Successful login creates a new browser session. Successful signup does the
  same and takes the person directly to the signed-in account page.
- A session expires after 24 hours even if active, or after one hour without
  activity. Activity extends only the inactivity limit, never the absolute
  lifetime.
- Signing out invalidates the current session on the server and removes its
  browser cookie. It does not sign out other browsers or relying-party
  sessions.
- Sessions remain valid across an Authling process restart when the browser
  still has its session cookie and runtime storage remains available.
- Sessions carry the account's durable authentication version. Password reset
  advances that version and invalidates every older Authling browser session,
  including across process restarts; the completing browser receives a new
  session.
- Protected pages reject absent, expired, malformed, forged, and revoked
  sessions. Cross-origin login and logout submissions are rejected.
- The configured public origin is canonical: requests for another host are
  rejected, and unsafe browser requests must carry that exact origin.
- Deployments may explicitly trust sanitized `X-Forwarded-Host` and
  `X-Forwarded-Proto` from their sole reverse proxy. The listener must not be
  directly reachable by untrusted clients when that option is enabled.

## Design Decisions

### 1. Keep session authority on the server

**Decision:** The browser carries only an opaque random bearer; the account and
lifetime state remain in expiring Authling runtime storage.

**Why:** Server-side state makes logout, expiry, and password-reset invalidation
authoritative and reveals no account data in the cookie.

**Tradeoff:** Every authenticated browser request depends on runtime-storage
availability.

### 2. Use a non-persistent browser cookie

**Decision:** The session cookie is host-only, `HttpOnly`, `SameSite=Lax`, and
scoped to `/`. HTTPS origins use the `__Host-authling_session` name and
`Secure`; loopback HTTP development uses an unprefixed cookie. Duplicate
session-cookie values are rejected. The cookie has no persistent expiry
attribute.

**Why:** These attributes reduce script access, cross-site presentation, and
cleartext transport risk while letting ordinary OIDC top-level navigation work
in a later slice. A browser-session cookie avoids silently adding a "remember
me" feature.

**Tradeoff:** Browser session restoration behavior varies, and local HTTP
development uses a separate cookie name from production.

### 3. Treat sessions as runtime state

**Decision:** Login and logout do not add durable domain events. Encrypted
session records expire in runtime state and use non-reversible lookup keys.

**Why:** The account and credential are durable facts; one browser's temporary
authenticated continuity is not. Replaying expired bearer state would be both
incorrect and unsafe under ADR-001.

**Tradeoff:** Authling does not yet provide a durable authentication audit
history. Any future audit feature needs its own data-minimising event policy.

### 4. Avoid account enumeration during password login

**Decision:** Public failures are generic, unknown accounts resolve and decrypt
a persistent synthetic credential before executing equivalent Argon2id work,
and attempt-limit keys are derived from normalized addresses with a deployment
key. Only actual credential mismatches consume the failure budget;
infrastructure errors do not.

**Why:** Error text, HTTP behavior, durable key names, and obvious timing gaps
must not reveal whether an address is registered.

**Tradeoff:** Generic failures are less helpful to legitimate users, and an
attacker can temporarily deny login to a known address by exhausting its
attempt budget.

## Limitations

- There is no "remember me", session list, selective remote session revocation,
  signed-in password change, or user-visible authentication history yet.
- Password-only login is a single-factor authentication ceremony. Authling
  does not yet implement MFA or phishing-resistant authenticators.
- Authling's listener does not terminate TLS. Production operators must expose
  login only through an HTTPS reverse proxy and configure its canonical
  `https://` public URL. Plain HTTP is a loopback development mode only.

## Related

- **ADRs:** [ADR-001](../adr/ADR-001-event-sourced-nats-architecture.md),
  [ADR-002](../adr/ADR-002-hierarchical-keys-and-cryptographic-erasure.md),
  [ADR-003](../adr/ADR-003-server-rendered-templ-ui.md)
- **Features:** [FDR-001](FDR-001-standalone-account-runtime.md),
  [FDR-002](FDR-002-verified-email-signup.md),
  [FDR-006](FDR-006-password-reset.md)
- **Security baseline:** [NIST SP 800-63B](https://pages.nist.gov/800-63-4/sp800-63b.html),
  [OWASP Authentication](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html),
  and [OWASP Session Management](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)
