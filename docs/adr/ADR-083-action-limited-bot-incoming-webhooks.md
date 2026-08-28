# ADR-083: Use Action-Limited Credentials for Bot Incoming Webhooks

**Status:** Accepted
**Date:** 2026-08-27

## Context

External systems need a small HTTP interface that can post messages as a bot.
The bot API key can already call the complete public API. Putting that key in a
webhook URL would give a simple caller more authority than it needs. A room-bound
credential for each integration would reduce authority, but it would also make
credential management complex and would prevent one automation from selecting
a destination at request time. One shared credential for all integrations
would make secret replacement disruptive. It would also make it difficult to
identify credentials that are not in use.

## Decision

Each bot can have at most 20 active incoming webhook credentials. Each
credential has a manager-defined name and authorizes only
`POST /webhooks/incoming/{credential}`. It cannot authenticate a ConnectRPC or
realtime request. Names do not have to be unique. This lets a manager create a
replacement before the manager revokes an old credential with the same name.

Chatto shows the complete webhook URL only when a human manager creates the
credential. Each URL contains the bot ID, a stable webhook ID, and a random
secret. The webhook ID selects one verifier without a scan. Chatto stores the
name, webhook ID, HMAC verifier, and creation time in the bot's user aggregate.
It projects these values, but it never stores the raw secret. A manager creates
a new named credential before the manager moves a caller and revokes the old
credential. Revocation invalidates only the selected credential. Bot deletion
and owner deletion invalidate all credentials for the bot.

The bundled frontend stops in-app navigation while it requests a show-once
credential and while it shows the returned value. It requests the browser's
native confirmation before a page unload. The manager must acknowledge the
credential before in-app navigation can continue. This rule also applies to
bot API-key creation and rotation.

The request uses a Slack-compatible plain-text JSON subset. `text` and `channel`
are the Slack field names. `body` and `room_id` are Chatto aliases. The optional
`room_id` query parameter allows a caller to put the destination in the URL. All
specified body and room values must agree. Room selectors contain stable room
IDs, not names. The Chatto-only `create_thread` field requests a new thread.

The HTTP handler authenticates the credential and calls the normal message
operation as the bot. Normal room membership, bot and owner permissions,
threading policy, archived-room state, slow mode, message validation, and
commit-time authorization apply. The webhook can post to an existing DM only
when a human already started that DM with the bot. It cannot start or find a DM.
It rejects thread creation in a DM.

The first version has no idempotency key. If a caller retries after it loses a
response, Chatto can create a duplicate message. A successful request returns
HTTP 200 with the plain-text body `ok`.

Chatto attempts to record the time at which each credential was last used.
Successful credential authentication creates an observation, even if request
validation or message posting subsequently fails. This rule helps a manager
find active callers without making optional telemetry part of authorization.

Last-use telemetry is mutable operational state in `RUNTIME_STATE`, not a
durable domain fact in `EVT`. One record for each bot contains the latest
observed use time for its webhook IDs. Each process records a new observation
in memory without blocking the request and writes updates with KV optimistic
concurrency control. Chatto writes the first observation promptly and
coalesces subsequent writes for the same credential to at most one each
minute. A failed telemetry write does not make authentication or message
posting fail. A missing record or credential entry means that Chatto has no
recorded use. This state does not prove that the credential was not used. The
management API reports telemetry as unavailable if it cannot read or decode
the record.

Credential revocation and process-local recording use one ordering lock on the
replica that handles the revocation. The recorder checks the current projected
credential before it accepts a new observation. Each replica also checks this
projected state before and after a storage write. A periodic sweep removes
local and stored telemetry for credentials that another replica revoked. If a
storage write completes after revocation, the writer attempts a second delete.
The recorder does not retain a revocation tombstone.

Bot construction does not read this optional state. Management responses read
it only for resources selected after filtering and pagination. These reads use
bounded concurrency. A response that returns a show-once credential does not
read last-use telemetry after the credential lifecycle fact commits. In that
response, telemetry for other credentials stays unspecified. It is not
reported as unavailable. The bundled frontend keeps its hydrated bot value and
refetches the bot instead of replacing that value with the partial response.

The EVT subject tokens from the first unreleased implementation remain stable.
The create-event wrapper tag changed before release because tag 722 now
identifies `UserBioChangedEvent`. No released server wrote the old tag. A
credential from the first implementation has no webhook ID in its stored event
or URL. Updated servers project it as a synthetic legacy credential and
continue to accept its two-part URL until it is revoked. New credentials use
the three-part URL format. Updated servers also replay verifier-replacement
events that the unreleased implementation wrote, but current servers do not
write these events. Operators must complete a rolling upgrade before they
create or revoke credentials. A rollback must use a binary that understands
the new lifecycle fields after these writes occur.

## Consequences

- A simple external caller does not receive general bot API authority.
- Each credential can post to all rooms where the bot has normal posting
  access.
- Separate credentials let managers replace or revoke one integration without
  an outage for other integrations.
- Credential lifecycle facts remain durable without persisting the raw secret.
- A route change cannot silently discard a newly issued credential in the
  bundled frontend.
- Last-use telemetry can be temporarily unavailable, delayed, or missing after
  a process or storage failure. Its failure cannot stop webhook requests.
- Revoked credentials do not create permanent process-local tombstones. A
  cleanup failure can leave stale optional telemetry in storage, but it cannot
  restore the revoked credential.
- The internal credential-usage recorder can also support multiple bot API
  keys later. This decision does not change bot API keys.
- URL credentials can appear in reverse-proxy logs. Operators must redact the
  incoming webhook path. Chatto redacts this path in its request logger.
- Callers that require exactly-once posting must wait for a later idempotency
  contract.
- Slack blocks and attachments, form payloads, and replies to an existing
  thread are not part of this decision.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-046](ADR-046-typed-runtime-credentials.md)
- [FDR-038](../fdr/FDR-038-bot-accounts.md)
