# ADR-083: Use Action-Limited Credentials for Bot Incoming Webhooks

**Status:** Accepted
**Date:** 2026-08-27

## Context

External systems need a small HTTP interface that can post messages as a bot.
The bot API key can already call the complete public API. Putting that key in a
webhook URL would give a simple caller more authority than it needs. A room-bound
credential for each integration would reduce authority, but it would also make
credential management complex and would prevent one automation from selecting
a destination at request time.

## Decision

Each bot can have zero or one active incoming webhook credential. The credential
authorizes only `POST /webhooks/incoming/{credential}`. It cannot authenticate a
ConnectRPC or realtime request.

Chatto shows the complete webhook URL only when a human manager enables or
rotates the credential. Chatto stores an HMAC verifier in the bot's user
aggregate and projects only the verifier and lifecycle timestamps. Disablement,
rotation, bot deletion, and owner deletion invalidate the credential.

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

## Consequences

- A simple external caller does not receive general bot API authority.
- One credential can post to all rooms where the bot has normal posting access.
- Credential lifecycle facts remain durable without persisting the raw secret.
- The management API and UI can use the existing bot credential patterns.
- URL credentials can appear in reverse-proxy logs. Operators must redact the
  incoming webhook path. Chatto redacts this path in its request logger.
- Callers that require exactly-once posting must wait for a later idempotency
  contract.
- Multiple credentials, Slack blocks and attachments, form payloads, and
  replies to an existing thread are not part of this decision.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-046](ADR-046-typed-runtime-credentials.md)
- [FDR-038](../fdr/FDR-038-bot-accounts.md)
