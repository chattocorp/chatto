# Authling Runtime Architecture Inventory

This directory records Authling's current runtime components and operational
contracts. Keep planned architecture in ADRs until it is implemented.

## Process

The [`authling` command](../../cmd/authling/main.go) exposes `help`, `version`,
and `run`. `run` loads the standalone configuration, opens Authling's NATS
storage, starts every required projection and the browser-session inventory,
waits for startup replay, starts the HTTP listener, and then runs until its
process context is cancelled. A required task failure cancels startup readiness
waits and is returned to the CLI with its original error chain. A missing
JetStream tier error also includes an account-quota hint.

The HTTP surface contains server-rendered signup, login, password-reset,
signed-in password-change, verified email-change, account deletion, consent,
account, and logout pages plus embedded browser assets. It also exposes OpenID Connect discovery,
authorization, token, UserInfo, and JWKS endpoints. Authling exposes no public
account-management, application-data, document, or synchronization API.

## Configuration

The runtime reads `authling.toml` by default. `AUTHLING_*` environment variables
override TOML values. Unknown TOML fields fail decoding.

`http.bind_address` selects the public HTTP listener and defaults to
`127.0.0.1:8080`. `AUTHLING_HTTP_BIND_ADDRESS` overrides it.

`http.public_url` declares Authling's externally visible origin and controls
browser cookie transport policy. An `http://` origin is valid only when both
the origin and listener are loopback; every other deployment must configure an
`https://` origin. `AUTHLING_HTTP_PUBLIC_URL` provides the equivalent override.
Requests at another host, port, or scheme receive a temporary redirect (307)
to the configured public origin, with their path and query preserved. Redirects
run before application handlers and are not cached. Unsafe browser requests must
carry a matching `Origin`; Fetch Metadata is an additional cross-site signal.
The listener itself is plain HTTP, so production deployments terminate HTTPS
at a reverse proxy. An explicit configuration switch lets canonical-origin
checks consume proxy-overwritten `X-Forwarded-Host` and `X-Forwarded-Proto`;
the direct listener must remain trusted-only in that mode. HTTPS deployments
use a host-bound `__Host-` session cookie; the unprefixed cookie name exists
only for loopback development.

`authentication.password_minimum_length` sets the local signup password
minimum and defaults to ten Unicode characters. Values from eight through 128
are accepted. `AUTHLING_AUTHENTICATION_PASSWORD_MINIMUM_LENGTH` provides the
equivalent environment override; the 1,024-byte maximum remains fixed.

The `smtp` section configures transactional email. When enabled, `host`,
`port`, and `from` are required. TLS defaults to mandatory STARTTLS (or
implicit TLS on port 465); `opportunistic` is an explicit local-development
fallback. Fields have corresponding `AUTHLING_SMTP_*` environment overrides.

Each `[[oidc.clients]]` table declares a conventional OIDC client with `id`,
`name`, and one or more exact `redirect_uris`. An omitted `secret` creates a
public client; a secret of at least 32 characters enables
`client_secret_basic`. URL client IDs are reserved for CIMD and need no local
configuration. HTTPS redirects are mandatory outside loopback development.
`oidc.cimd_trusted_private_hosts` and `oidc.cimd_trusted_loopback_hosts` are
separate, exact-host development exceptions. They permit named CIMD hosts to
resolve only to private or loopback addresses respectively; neither permits
other special-use destinations.

Operators must select exactly one NATS mode:

- `nats.embedded.enabled = true` starts a private in-process NATS server with
  JetStream and no TCP listener. Its file-backed state lives in
  `nats.embedded.data_dir`, which defaults to `.authling/nats`.
- `nats.client` connects to an external URL using a NATS credentials file.
  Credentials are mandatory so Authling uses its own NATS account.

JetStream resources use one replica by default. Explicit replica counts may be
one, three, or five.

The application-neutral embedded server lifecycle comes from
`hmans.de/chatto/pkg/natsruntime`; Authling retains its private-listener,
storage-path, logging, and deployment policy.

## NATS and JetStream

| Resource | Kind | Storage | Subjects | Purpose |
|----------|------|---------|----------|---------|
| `AUTHLING_EVT` | Stream | File, S2-compressed | `authling.evt.>` | Authoritative Authling event history |
| `AUTHLING_RUNTIME_STATE` | KV bucket | File, history 1 | Opaque HMAC-derived keys | Encrypted signup, password-reset, email-change, session, OIDC request, code, and access-token state, plus bounded delivery and login-attempt counters |
| `AUTHLING_KEYS` | KV bucket | File, history 1 | Opaque key references | Workflow, OIDC signing, user, and wrapped credential data keys |

`AUTHLING_EVT` enables JetStream atomic publication for multi-event
commands. The key bucket is a separate, exceptionally sensitive backup and
restore boundary.

Credential provisioning writes an opaque operation record before creating its
user and data keys, then removes the marker after the referencing event
is acknowledged. Failures before publication and definite OCC rejections permit
immediate cleanup. An unknown publication outcome retains both keys and the
operation marker, even if the request reports failure. Crash orphans remain
discoverable by their durable marker; Authling does not use time alone as
authority to delete keys that an in-flight replica could still reference.

## Persisted events and subjects

Persisted records use the `authling.core.v1.Event` protobuf envelope:

New envelope IDs are opaque random `evt_...` values. Encoding and replay
accept historical single-token identifiers but reject whitespace, subject
separators, and email-address punctuation; correlation fields use the same
token-safe vocabulary.

| Event | Subject | Aggregate | Contents |
|-------|---------|-----------|----------|
| `AccountCreatedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account ID, envelope creation time, and encrypted local credential fields including the preferred username |
| `PasswordResetRequestedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account and credential-event IDs; the envelope ID identifies the audit request |
| `PasswordChangedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account, credential-key, prior-credential, ceremony, and optional reset-request references plus the replacement encrypted password verifier |
| `EmailChangeRequestedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account and reauthenticated credential-event IDs |
| `EmailChangedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account, credential-key, request, and prior-credential references plus the replacement encrypted email |
| `ProfileUpdatedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account and credential-key references plus replacement encrypted preferred-username and full-name fields |
| `EmailClaimedEvent` | `authling.evt.account-registry` | Account registry | Opaque account and optional staged credential-event IDs |
| `OIDCGrantAuthorizedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account, grant, and prior-authorization IDs; keyed exact-client digest; encrypted client display snapshot and account key references; granted scopes; consent disclosure version |
| `OIDCGrantRevokedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account, grant, and active authorization-event IDs |
| `AccountErasureRequestedEvent` | `authling.evt.account.{accountId}` | Account | Current credential and key references; permanent access denial |
| `EmailReleasedEvent` | `authling.evt.account-registry` | Account registry | Opaque account and erasure-request correlation; atomic email release |
| `AccountErasedEvent` | `authling.evt.account.{accountId}` | Account | Opaque account and request correlation; live key purge completion |
| `IssuerEstablishedEvent` | `authling.evt.issuer` | Issuer singleton | Immutable issuer URL and opaque signing-key reference and ID |
| `OIDCSigningKeyRotationRequestedEvent` | `authling.evt.issuer` | Issuer singleton | Opaque future signing-key reference |
| `OIDCSigningKeyPreparedEvent` | `authling.evt.issuer` | Issuer singleton | Opaque signing-key reference, public fingerprint ID, and activation time |
| `OIDCSigningKeyActivatedEvent` | `authling.evt.issuer` | Issuer singleton | New and preceding key identities plus predecessor retirement time |
| `OIDCSigningKeyRetirementRequestedEvent` | `authling.evt.issuer` | Issuer singleton | Opaque identity of the predecessor removed from JWKS before destruction |
| `OIDCSigningKeyRetiredEvent` | `authling.evt.issuer` | Issuer singleton | Opaque identity whose private material was destroyed |

The account ID is restricted to one NATS-safe token. Structural account
creation uses per-account OCC. Verified local account creation atomically
publishes `AccountCreatedEvent` to the per-account subject and
`EmailClaimedEvent` to the PII-free registry subject. OCC guards both the new
account aggregate and current registry tail, serializing email claims across
replicas without a durable email-derived index.

## Models

The account model consumes `authling.evt.account.*` and
`authling.evt.account-registry`. It maps opaque account IDs to creation times.
During replay it resolves and decrypts active local credentials and rebuilds a
keyed digest index of normalized emails. It retains encrypted verifier fields and
opaque key references, but neither plaintext email nor plaintext password
verifiers. It retains encrypted profile fields and decrypts them only at the
account-service read boundary. The model retains bounded password-reset request correlations so
replay can validate recovery-produced password changes. Password changes
validate their declared recovery or signed-in ceremony, replace the current
encrypted verifier, and advance a durable account authentication version. The
model retains a bounded set of email-change requests per account
so replay can require the exact reauthentication audit chain without retaining
abandoned request IDs without bound. An email-change account event stages its
encrypted replacement; the adjacent correlated registry event swaps the active
digest and credential and advances the authentication version. Before a
credential-generation-bound password or request-audit command evaluates its
precondition, the account service captures and waits for both the account and
registry tails and rejects a staged replacement that is not active yet. Local
authentication, signed-in password change, and email-change reauthentication
share distributed attempt limits and bounded Argon2 capacity. They resolve and
decrypt a verifier only for one bounded Argon2id comparison; absent login
accounts resolve a persistent synthetic key hierarchy and encrypted dummy
verifier through the same storage path.
After a successful password check, login waits for both account and registry
projection boundaries and reads the generation only if the exact verified
credential remains active. Audit events do not change that credential proof.

The runtime does not become ready until the projections have replayed their
captured startup history. A decode or apply failure fails the projection and
runtime.
After account creation commits, the account service waits for the committed
stream position before returning the projected account.

The account projection is currently cold-replay-only. It has no snapshot or
local-checkpoint persistence.

The authorization-grant projection consumes `authling.evt.account.*` and
materializes active grants by account, exact client ID, and opaque grant ID. It
validates account existence and the correlation chain for explicit renewal and
revocation, retains ended grant IDs to prevent generation reuse, and serves
only after startup replay. Grant commands synchronize to the account tail,
publish with account-subject OCC, retry from refreshed state after conflicts,
and wait for their committed position. The projection is cold-replay-only and
retains scopes and encrypted client display metadata. The service decrypts
metadata for display and authenticates it before consent reuse. Metadata keys
must match the account creation event. Only encrypted grant records are
supported. See [FDR-010](../fdr/FDR-010-oidc-authorization-grants.md) for the
encryption and disclosure-version rules.

The browser-session inventory is a process-wide in-memory model over one
filtered `session.*` watcher on `AUTHLING_RUNTIME_STATE`. It decrypts the latest
session value for each key and maintains account-to-session and reverse-key
maps. The KV bucket remains authoritative, and the inventory has no snapshot,
checkpoint, or second persisted index. Startup replays all live session keys;
delete markers remove them. Malformed records are omitted because they cannot
authenticate. A watcher startup failure prevents readiness, and a later
watcher failure stops the runtime. Session writes wait for their observed KV
revision when this model is running.

The issuer projection consumes the singleton `authling.evt.issuer` subject.
On first initialization, its service creates or resolves the RS256 signing key
and establishes the issuer with subject-level OCC. It then materializes one
active key, at most one pre-published successor, and at most one unexpired
predecessor. The in-process reconciler automatically requests rotation when
the active key reaches its configured age, creates event-owned key material,
activates it after ten minutes of JWKS publication, and retires the predecessor
after a 15-minute overlap. Every transition uses issuer-subject OCC and waits
for its projected position. Restart resumes incomplete creation or destruction
outcomes. Every later startup requires the configured public URL and all
published signing-key identities to match their protected key records. Issuer
or key drift prevents readiness.

## HTTP interface

The HTTP handler renders HTML with templ. Vite compiles Tailwind CSS, IBM Plex
Sans, and Iconify glyphs during the build; the resulting assets are embedded
in the Go executable and served below `/assets/`. The runtime has no Node.js or
third-party asset-host dependency.

The initial Content Security Policy prohibits scripts and third-party content.
All essential future authentication interactions must continue to work through
ordinary server-rendered links and forms.

`GET /signup` renders the email form. Three POST endpoints start a flow, verify
its code, and complete account creation with a password. Unsafe requests reject
cross-origin browser submissions. Signup from an OIDC login carries the
validated pending request ID through its forms and resumes consent after
session creation. A silent OIDC request returns
an authorization code or a protocol error without rendering login or consent;
its encrypted `silent` flag survives restart.
The browser carries a random opaque flow token in hidden fields; raw email addresses, OTPs, and passwords never enter
URLs.

`GET /login` renders local credential login. `POST /login` applies a shared,
keyed attempt limit before checking the encrypted credential and creates a
fresh browser session on success. `GET /account` requires that session, and
same-origin `POST /logout` revokes it. Successful signup also starts a session.
The host-only browser cookie carries only a random opaque bearer and is
`HttpOnly`, `SameSite=Lax`, scoped to `/`, non-persistent, and secure outside
the explicit loopback development mode.

The `internal/runtimejson` codec encodes version-1 encrypted JSON envelopes
for signup, password reset, email change, sessions, and OIDC runtime state.
It preserves the capitalized workflow fields and lowercase session/OIDC fields.
Callers supply the existing key and associated data, including the storage key.
They retain expiry, revision checks, storage access, and domain validation.
The codec clears temporary plaintext buffers before returning. Existing records
remain readable, and old readers can read new records without migration.

Session records are authenticated-encrypted in runtime state beneath
HMAC-derived keys. They have a 24-hour absolute lifetime and a one-hour
inactivity limit. Activity updates use OCC and never extend the absolute
deadline. Each session records the account authentication version current at
issuance. Password reset, signed-in password change, and verified email change
advance that durable version, invalidating every older session across replicas
and restarts. Login, signup, and recovery bind session creation to the exact
authentication generation that authorized the operation. A later mutation
cannot upgrade an earlier proof to the new generation. The event and session
storage formats are unchanged; all replicas must run the fix to close the old
session-creation path. Logout deletes the server record before clearing the
cookie.

`GET /account` also reads the current account's active sessions from the
process-wide inventory. It renders lifecycle timestamps and identifies the
current browser without collecting user agents, IP addresses, device names, or
locations. Same-origin `POST /account/sessions/revoke` signs out one other
browser, while `POST /account/sessions/revoke-others` signs out every other
browser. Forms carry a deployment-local opaque session ID derived separately
from the bearer and internal KV coordinate. Every deletion authoritatively
re-reads, decrypts, and re-authorizes the KV record, uses OCC, and waits for the
local watcher before redirecting.

`GET /password-reset` starts verified-email recovery. Three POST endpoints
create an expiring flow, verify its six-digit code, and commit a new password.
Claimed and unclaimed valid addresses follow the same email-delivery and
browser path. After non-refundable admission and delivery limits accept an
existing account's request, a PII-free `PasswordResetRequestedEvent` must commit
before flow creation or SMTP
delivery; absent accounts have no aggregate on which to record one. Encrypted
flow state is bound to that audit event and the credential event current at
start. Account-subject OCC prevents concurrent stale flows from overwriting a
newer password while tolerating intervening request-audit appends. Successful
completion links `PasswordChangedEvent` to the request event, preserves the
account ID and email claim, creates a new browser session, and can resume an
interrupted OIDC consent request.

`GET /account/email` requires a valid browser session and renders signed-in
email change. Three POST endpoints reauthenticate the current password, verify
a six-digit code delivered to the requested address, and confirm completion.
`EmailChangeRequestedEvent` commits before flow creation or delivery and stores
no address. The encrypted flow binds both addresses and the requested change to
the reauthenticated credential. Multiple flows can coexist, but the first
credential mutation makes the others stale. Completion atomically appends an
encrypted `EmailChangedEvent` and PII-free correlated `EmailClaimedEvent` under
account and registry OCC. The old address remains authoritative until that
batch commits; afterward Authling advances the durable authentication version,
creates a fresh completing session, and attempts a best-effort security notice
to the old address. A retry after an ambiguous process failure recognizes the
committed request from the projected credential; notification recovery is
at-least-once and can duplicate the notice. A 45-second OCC-backed completion
lease encloses explicitly bounded lease acquisition, identity mutation,
notification, and cleanup phases so concurrent recovery cannot overlap active
completion. The replacement session is bound to the email change's exact
authentication generation, and a later credential generation invalidates both
recovery and session establishment. The completion POST redirects to the
account page with a refresh-safe success result and, when needed, the
old-address delivery warning.

`GET /account/password` requires a valid browser session and renders signed-in
password change. Its POST requires the current password, a distinct replacement
that satisfies the configured password policy, and matching confirmation. The
current-password check uses an OCC-backed distributed attempt limit and bounded
Argon2 capacity. Completion waits through captured account and email-registry
projection boundaries, then appends a `PasswordChangedEvent` bound to the exact
reauthenticated credential. It advances the authentication version, invalidates
older browser sessions, and creates a replacement session at that exact
generation. The account ID, verified email, and OIDC `sub` remain unchanged.
`GET` and `HEAD /.well-known/change-password` return a temporary, non-cacheable
redirect to this page. A signed-out request carries only this fixed internal
return target through login. Other submitted return targets are ignored.

OpenID Connect mounts discovery at `/.well-known/openid-configuration` and its
protocol endpoints below `/oauth/`. Authorization accepts only code flow,
requires exactly the `openid` scope and S256 PKCE.
Signed-out requests resume through an opaque server-side request ID after
login. Encrypted session state carries a separate authentication timestamp;
email-change session replacement preserves it. Encrypted OIDC request state
stores `max_age` and forced-login constraints. Both grant reuse and approval
check these constraints, return stale sessions to login, and copy the actual
authentication time into the ID token's `auth_time` claim.
`GET /oidc/consent` reuses a durable exact-client authorization grant
when it covers the requested scopes and current disclosure version, except
when `prompt=consent` requires an explicit decision. The page lists the account
ID, username, and optional full name and discloses later profile changes. Same-origin `POST /oidc/consent`
requires the current form disclosure version to record explicit approval
before authorizing the expiring request or returns a denial to the validated
client redirect.

`GET /account` lists active OIDC grants separately from Authling browser
sessions. Same-origin `POST /account/authorizations/revoke` authorizes the
opaque grant ID under the current account and commits revocation. Revocation
forces future authorization to ask again but does not terminate already issued
five-minute tokens or relying-party sessions.

Conventional clients resolve from configuration. Unconfigured HTTPS URL client
IDs resolve through the bounded CIMD fetcher, which disables redirects and
proxies, validates DNS destinations before fetch and dial, and caps fetch time,
body size, concurrency, and cache lifetime. Each process admits at most eight
cache-miss lookups without a waiting queue, before DNS work starts. One
five-second lookup deadline includes DNS and body reads. Its cache holds at
most 256 clients, removes expired entries on access, and evicts the entry with
the earliest expiry when full. Pending requests, code mappings,
and opaque access-token records are encrypted and expire in runtime state.
New authorization requests first consume a shared OCC admission counter in
`AUTHLING_RUNTIME_STATE`. It permits 1,000 admissions and expires ten minutes
after the last admission. Failed work does not refund it. Recovery uses separate
global and keyed per-address admission counters with a 15-minute quiet window,
so failed SMTP delivery cannot bypass the bound on permanent recovery events.
These counters contain no identifiers or secrets and share no event subjects.
Authorization-code claim uses KV OCC so concurrent exchange has at most one
winner. ID tokens use the active RS256 key; JWKS publishes its public key plus
any prepared successor and unexpired predecessor. JWKS responses have a
five-minute public cache lifetime. Opaque access-token encryption uses a
separate stable symmetric key, so signing-key rotation does not invalidate
UserInfo access. ID tokens and UserInfo return the account ID as `sub` and non-empty,
encrypted-at-rest profile hints as `preferred_username` and `name`.

The HTTP server bounds header, body-read, response-write, and idle time. Signup,
password reset, signed-in password change, and email change also cap request
bodies. OTP flows globally limit delivery and bound concurrent SMTP and
completion work per process.

Signup, password reset, and email change use `storage.DeliveryBudget` for
refundable delivery counters. Each workflow keeps its existing global key,
HMAC-derived recipient keys, limits, and 15-minute quiet window. The budget
reserves global capacity before recipient capacity. Failed work refunds
confirmed reservations on a best-effort basis. Counter mutations retry only
confirmed revision conflicts. An uncertain write acknowledgement stops the
operation and can leave capacity consumed until expiry. Rollback attempts both
counters even if one fails. Non-refundable request admission remains separate.

## Account deletion and erasure

`GET /account/delete` renders the effects and limits from
[FDR-013](../fdr/FDR-013-account-deletion.md). Its same-origin, body-limited POST
requires an active session, explicit confirmation, and a fresh throttled password
proof at the current authentication version. Account and registry OCC commit
`AccountErasureRequestedEvent` and `EmailReleasedEvent` as one atomic batch.
Commands wait through both relevant subject boundaries and reject staged email
changes. The request removes the account, email index, credentials, profile,
recovery requests, and grants from active projections. Session checks cross the
durable account tail; storage errors deny access without deleting valid sessions.
Protected profile reads also cross that boundary, which denies code exchange and
UserInfo after deletion. In-flight operations must pass their account boundary;
OCC prevents an identity mutation from committing after the request.

A separate key-free erasure projection starts before the account projector. It
records account key ownership, request and release positions, and completion.
It retains ownership history to reject key reuse and substitution. Account replay
skips protected email decryption only with durable erasure evidence; it still
validates structural account history. A failed key read repeats the erasure
barrier to cover another replica's concurrent purge. Missing active keys remain
a fatal replay error. No active email index entry is built from erased history.

The named durable consumer `authling-account-erasure` filters
`authling.evt.account.*`, uses explicit acknowledgements, a one-minute ack wait,
and one pending message. The worker retries failures after one second. It waits
for account and grant projections to validate the decision, purges the user key
then its wrapped data key from `AUTHLING_KEYS`, and appends `AccountErasedEvent`
with account OCC. Repeated purges and repeated completion attempts are safe.
The request remains the retry authority if a reply is lost or the process stops.
Workers on multiple replicas may share the consumer; the consumer cursor is not
the correctness boundary. Worker errors do not include protected data.

The account key vault does not cache unwrapped keys. Historical ciphertext and
PII-free structure remain in the event stream. Workflow-key-encrypted email
change records can retain both addresses until their original 15-minute expiry;
other verification flows have the same independent maximum lifetime. Session
and OIDC records retain their existing expiry policies but cannot authorize the
deleted account. Key erasure does not retract SMTP already in progress or data
already returned to a relying party.

A completion event proves purges from the live key API, not secure removal of
filesystem remnants, snapshots, backups, external key exports, or other apps'
data. An operator must retire key backups under a separate retention policy.
Restoring a snapshot from before deletion can restore the identity and keys;
do not serve that restore as current state without reconciling later deletions.
New event variants require all replicas to use this release before deletion.
There is no migration or mixed-version fallback for this undeployed product.

## Deliberately absent

The runtime does not yet contain MFA recovery, browser-device
or location tracking, durable login history, OIDC refresh tokens, emergency
manual signing-key rotation, diagnostic endpoints, or backup tooling.
Application data, documents, and generic synchronization are deliberately
outside Authling's identity-provider boundary.
