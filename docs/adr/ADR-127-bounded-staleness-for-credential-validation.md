# ADR-127: Bounded Staleness for Runtime Credential Validation

**Status:** Accepted
**Date:** 2026-11-22

## Summary

Today a replica accepts a runtime credential only after it has confirmed, with
the `EVT` stream leader, that its User Auth projection is current for that
user. This is the strict auth generation check. It runs on every request.

We propose that a replica remembers a strict check that accepted a user for
60 seconds. During that time, request validation for that user reads the local
projection and sends nothing to the stream leader.

**The changed guarantee:** a replica can accept a revoked credential for a
maximum of 60 seconds after the revocation, and only while its User Auth
projection has not applied the revoking event. Today that time is zero. In
usual operation the projection applies the event in milliseconds.

The window is the guarantee. Two gates end it early when the replica detects
that it has no NATS connection or that its projector failed. They are a small
improvement for those clear cases. They do not make the worst case shorter.

## Context

Chatto runs as multiple replicas. Each replica keeps its projections in
memory and feeds them from the `EVT` stream
([ADR-033](ADR-033-event-sourced-state-with-projections.md)). A projection on
one replica can lag a write that a different replica made.

Each validation of a runtime credential (cookie session, access token,
renewable session) compares the auth generation that the credential stores
with the current auth generation of the user. `RequireAuthenticationAllowed`
and `CurrentAuthGeneration` in `cli/internal/core/auth_generation.go` do this
check. The generation and the active state come from
`UserAuthProjection.AuthGeneration`.

So that a replica never decides from a projection that lags, the check first
reads from the stream leader the newest position of each subject that can
change the result. Then it waits until the projection has applied that
position. [#3778](https://github.com/chattocorp/chatto/pull/3778) decreased
this cost from approximately 25 sequential leader reads to seven concurrent
position reads and one or two reads that validate the wait. It also added
`userAuthGenerationSubjects`, a list of subjects that maintainers must keep
complete by hand.

The cost of the strict check increases with the number of requests, not with
the number of revocations. Measurements from November 2026:

- On the largest production deployment (approximately 25 replicas on one
  shared NATS cluster of three nodes), each NATS server used 60% to 67% of
  one CPU. The stream leader served approximately 141 JetStream API requests
  each second. All streams together received approximately 1.4 writes each
  second.
- In process, after #3778, one check costs 8 leader reads and 143 µs. Before,
  it cost 24 reads and 690 µs.

Almost all of these reads confirm that nothing changed.

Two existing rules already accept a bounded delay for revocation:

- Request validation reads `RUNTIME_STATE` from any replica first. That read
  can accept a credential but cannot reject one
  ([ADR-079](ADR-079-renewable-bearer-sessions.md)).
- A realtime connection validates its credential again on a timer
  (`realtimeCredentialCheckInterval`, one minute). A process-local watcher
  also closes it when a deactivation fact reaches the replica
  (`WatchAccountAccessInvalidated`).

## Decision

Accept bounded staleness when a replica validates an existing runtime
credential. The window is 60 seconds.

### 1. A replica remembers strict checks that accept

When a strict check accepts a credential of a user, the replica records the
time for that user. For the next 60 seconds, request validation for that user
on that replica does not read from the stream leader. It reads the local User
Auth projection directly. This is the fast check.

There is no background probe. A replica without requests sends nothing.

**Why 60 seconds:**

- The window does not make revocation fast or slow. The fast check reads the
  live projection, which usually applies an event in milliseconds. The window
  only limits how long a replica that is behind, and does not know it, can
  stay wrong.
- 60 seconds is the interval that realtime connections already use for their
  strict validation. With this value, request validation and realtime
  connections have the same worst case.
- 5 seconds helps only during a burst of requests. A client that sends one
  request each 15 or 30 seconds gets no benefit. 60 seconds includes those
  clients.

### 2. Only a "valid" answer can be stale

The fast check can only accept. When the local projection gives a different
answer (unknown user, deleted or deactivated account, or a generation that is
not the stored generation), the replica runs the strict check and uses that
result.

A stale negative answer rejects a valid user, for example an account that a
different replica created or reactivated. Some paths also delete the stored
credential on that answer. Thus no path acts on a negative answer that did not
come from a strict check.

### 3. Issuance and credential changes stay strict

The fast check applies only to the read-only pass of request validation. That
is the pass that already reads `RUNTIME_STATE` from any replica. Each pass
that reads through the stream leader keeps the strict check on every call.

| Path today                                                           | Type                | Check after this decision |
| -------------------------------------------------------------------- | ------------------- | ------------------------- |
| `ValidatePresentedRuntimeCredential`, first pass (cookie and bearer) | Validation          | Fast check permitted      |
| `ValidatePresentedRuntimeCredential`, second pass after a rejection  | Validation, deletes | Strict                    |
| `revalidateRealtimeCredential` (realtime timer, see section 5)       | Validation          | Strict                    |
| `RenewCookieSession`                                                 | Change, deletes     | Strict                    |
| `MarkCookieSessionFresh`, `MarkBearerTokenFresh`                     | Change, deletes     | Strict                    |
| `requireFreshCredential`                                             | Validation          | Strict                    |
| `SetCookiePrivilegedMode`, `setRenewableSessionPrivilegedMode`       | Change, deletes     | Strict                    |
| `RevokePresentedRuntimeCredentialWithReason` (logout)                | Revocation, deletes | Strict                    |
| `RevokeRefreshTokenWithReasonResult` (logout)                        | Revocation, deletes | Strict                    |
| `refreshBearerSessionAt` (refresh rotation)                          | Issuance            | Strict                    |
| `verifyUserPassword`                                                 | Issuance            | Strict                    |
| `CreateCookieSession`, `NewCookieSessionData` and their variants     | Issuance            | Strict                    |
| `CreateBearerSessionWithSource`, `createBearerSessionForGrant`       | Issuance            | Strict                    |
| `CreateAuthCode` and its variants                                    | Issuance            | Strict                    |
| `ExchangeAuthCodeForClientResourceSession`                           | Issuance            | Strict                    |

Requests are the large majority of all checks, so the strict rows cost
little.

### 4. Two gates end the window early

A replica uses the fast check only while it has reason to believe that its
User Auth projection is fed. Each of these conditions ends the window
immediately for all users on that replica. The next validation of each user
is then strict.

1. **The NATS connection is not connected, or the recovery after a reconnect
   is not complete.** The strict check then fails and the replica rejects the
   request, as it does today.
2. **The User Auth projector reports that it failed or is stalled.**

A replica can be behind in three ways:

- **It has no connection to NATS.** It also cannot read from the stream
  leader. Gate 1 applies, and the behavior is the same as today. Without this
  gate, a partitioned replica accepts recently checked users for the full
  window, which is worse than today.
- **It has a connection, but its projection is behind.** Examples are a
  blocked or slow apply, an event consumer that stopped or that the SDK
  creates again, and a slow NATS server that feeds the consumer while the
  stream leader answers normally. The strict check finds a revocation in this
  condition. The fast check does not. Gate 2 applies to the part that the
  replica can detect. The part that it cannot detect is the residual risk,
  and the full window applies to it.
- **Usual delivery delay of some milliseconds.** The strict check closes this
  interval today. The fast check does not. This delay does not depend on the
  length of the window.

**What the current code can detect, and how fast:**

| Signal                                     | Source today                                                                                           | Detection time                                                                                          |
| ------------------------------------------ | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| Connection closed or reset                 | `nats.Conn` status events and `IsConnected`, which `runNATSRecovery` and `chatto_nats_connected` use   | Immediate. `runNATSRecovery` also polls each 250 ms                                                     |
| Recovery after a reconnect is not complete | `natsRecoveryState`, which `ChattoCore.Ready` reads                                                    | Immediate. It stays set until all projections are current again                                         |
| Connection lost without a reset            | The NATS client ping. Chatto uses the defaults: 2 minutes, 2 pings                                     | 4 to 6 minutes. Longer than the window                                                                  |
| Projector failed                           | `Projector.Err()`, `Status().Err`                                                                      | Immediate                                                                                               |
| Projector stalled                          | `Status().Stalled`                                                                                     | After 5 minutes of pending work without a commit (`DefaultProjectorStallAfter`). Longer than the window |
| Consumer receives no heartbeat             | The SDK reports it to `handleConsumeErr` after approximately 10 seconds and creates the consumer again | Not visible to core. The projector only writes a log line                                               |

The gates are thus weaker than the window in two cases, and the full window
of 60 seconds applies to both:

- A network partition that does not reset the connection. The replica
  accepts recently checked users from its projection until each window ends.
  Then each strict check fails. Today the replica rejects those requests
  immediately.
- A projection that stopped less than 5 minutes ago. The stalled state does
  not end a window that is in progress. It only prevents new windows on a
  replica that has been stalled for 5 minutes or longer.

Faster signals are an open point. This record does not design them.

### 5. The realtime timer always uses the strict check

The realtime credential timer validates through
`ValidatePresentedRuntimeCredential`, for cookie and for bearer credentials.
With the fast check, a connection on a replica that is behind can stay open
for approximately two minutes: one timer interval and one window. Thus the
timer validation always uses the strict check. The cost is one strict check
each minute for each connection, as today.

### 6. Validation fails closed after the window

When the window has passed, the next validation runs the strict check. If NATS
is not reachable, that check fails as it does today. A replica that a network
partition isolates cannot accept credentials for longer than the window.

### 7. The record is process-local acceleration state

The time of the last strict check is process-local. A replica never persists
it and never shares it. A replica that restarts has no records and starts with
strict checks. The implementation must limit the size of this state. A record
that is older than the window has no value and can be removed.

## Consequences

- **The security guarantee changes.** Today every replica sees a revocation
  before it accepts the credential. Revocations are password changes, account
  deletion, deactivation, key shredding, and identity unlinks that do not
  preserve credentials. After this decision, a replica can accept a revoked
  credential for a maximum of 60 seconds, and only if its User Auth projection
  has not applied the event.
- **The usual exposure is the projection lag, not the window.** The fast check
  reads the projection, and the projection usually applies an event in
  milliseconds. The window is the worst case, for a replica that is behind
  and whose gates have not detected it.
- **Cost.** For sequential requests, a replica runs one strict check for each
  active user in each 60-second window, and one each minute for each realtime
  connection. Concurrent requests that find no valid window each run a strict
  check, and each strict path of section 3 runs one on each call. Today it runs
  one for each request.
- **Availability does not change.** Gate 1 ends all windows when the replica
  detects that it has no NATS connection. It then rejects requests as it does
  today. Before the detection, it accepts recently checked users until their
  windows end.
- **The gates depend on existing signals.** The stall signal and the default
  ping are slower than the window. The residual risk in section 4 stays until
  faster signals exist.
- **New process-local state.** It needs a size limit and tests for the window
  boundaries.
- **`userAuthGenerationSubjects` stays.** The strict check continues to use
  the list, and maintainers must continue to keep it complete.
- **Realtime connections keep their guarantee.** They keep their watchers,
  and their timer stays strict.

### Statements that the implementation must update

This record does not change other records. The change that implements it must
update these statements:

- [FDR-023](../fdr/FDR-023-authentication-and-sessions.md), "Password and
  account lifecycle revocation": validation "waits the user projection to the
  current auth-generation events". Also the bearer decision: "opaque tokens
  give instant revocation", and "deleting the stable authority rejects every
  access generation on its next API request".
- [ADR-024](ADR-024-opaque-bearer-tokens-for-cross-origin-auth.md): "Instant
  revocation", and the statement that the revocation guarantee comes from the
  comparison with the current auth generation. Each must say "within one
  minute in the worst case".
- [ADR-079](ADR-079-renewable-bearer-sessions.md): auth-generation changes
  reject "all overlap at its next validation". The paragraph on fast reads
  must also name the window.
- [`docs/architecture/projections.md`](../architecture/projections.md): "Each
  validation of a runtime credential checks the auth generation. Before the
  check reads the projection, it waits…".
- [`docs/architecture/runtime-state.md`](../architecture/runtime-state.md):
  "Authentication checks the account’s durable auth generation before cleanup
  finishes." The inventory also needs an entry for the new process-local
  state.
- [FDR-018](../fdr/FDR-018-account-lifecycle.md): deactivation "invalidates
  cookie and bearer sessions". The statement stays true but must name the
  window.
- `proto/chatto/admin/v1/members.proto` and the generated API reference:
  "Deactivation immediately invalidates existing sessions" becomes "within
  one minute in the worst case".
- The glossary entry "Auth generation check window", which names this
  proposal.

[ADR-033](ADR-033-event-sourced-state-with-projections.md),
[ADR-036](ADR-036-runtime-state-kv-boundary.md),
[ADR-046](ADR-046-typed-runtime-credentials.md), and
[ADR-114](ADR-114-jetstream-storage-conventions.md) contain no statement that
this decision contradicts. No persisted contract changes.

## Alternatives

- **Keep the strict check on every request.** This is the state after #3778.
  It is correct, but the cost increases with the number of requests.
- **One shared currency check for each replica.** Give the User Auth
  projection one broad subject filter. Currency is then one position read, and
  concurrent requests can share that read. This keeps the strict guarantee and
  removes the hand-maintained list. It needs changes to the shared projector
  in `pkg/events`. It is not rejected. It can complement this decision later:
  it makes the strict checks that remain cheaper.
- **A background probe that confirms the projection is at the end of the
  stream.** Replicas without requests also pay for the probe. At the current
  traffic, the probe costs more reads than it saves.
- **Different subjects, or an auth record for each user in key-value
  storage.** The first changes persisted contracts
  ([ADR-114](ADR-114-jetstream-storage-conventions.md)). The second needs two
  stores to agree.

## Open Points

- **Constant or configuration.** We recommend a constant of 60 seconds first.
  The window is a security property. A setting lets one deployment weaken it
  silently, and replicas with different values make the guarantee hard to
  state. Add a setting only when an operator shows a need.
- **Bot credentials.** Bot API key authentication and bot incoming webhook
  authentication also read from the stream leader on each request. A separate
  change makes those reads narrower. Whether and how this policy applies to
  them is not decided here.
- **The benefit is not measured.** There is no measurement of the time
  between requests of one user in production. Thus the share of strict checks
  that the window removes is not known.
- **Faster gate signals.** Three candidates:
  - `Status().PendingFor` exists today and shows pending work before the
    stall threshold. When the consumer is silent, it changes only after the
    next source check (`projectionSourceCheckInterval`, 30 seconds). Gate 2
    could use it with a short threshold and no framework change.
  - A signal for consumer heartbeat loss needs a change in `pkg/events`.
  - A shorter NATS ping interval makes gate 1 detect a silent partition
    faster. It is a connection option, and it changes the reconnect behavior
    of all NATS clients in the process.
- **Measurements after rollout.** Confirm the effect with the JetStream API
  request rate on the stream leader, the number of strict checks each second,
  and the ratio of fast checks to strict checks.
