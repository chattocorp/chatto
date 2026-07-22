# ADR-055: Pluggable Message Search over NATS

**Date:** 2026-07-21

## Context

Message search needs a full-text index derived from encrypted message history.
That index is privileged plaintext-derived state, may need substantial CPU and
disk, and is not part of Chatto's durable domain model. Operators should be
able to run a bundled implementation in one `chatto run` process, move it to a
standalone process, or replace it without changing clients.

Putting Bleve directly behind the public API would couple Chatto Core to one
engine and make embedded and standalone deployments behave differently.
Allowing a provider to decide end-user visibility would instead duplicate
Chatto's room-membership and current-message authorization boundary.

## Decision

Expose message search to authenticated clients through the public
`chatto.api.v1.MessageSearchService`, implemented by the main Chatto process.
The main process owns feature enablement, canonical query parsing, current room
scope resolution, result authorization, body-revision checks, hydration, and
public cursor sealing.

Delegate provider-neutral matching to a trusted NATS service at
`svc.chatto_ext.search.v1.>`, following ADR-053. Requests and responses use the
`chatto.search.v1` protobuf contract. The provider receives normalized required
terms and phrases, filters, ordering, pagination, and the caller's complete
authorized room scope. Provider hits are candidates rather than authorization
decisions.

Queryable provider replicas share the versioned `.query` and `.status` queue
subjects. A provider that is rebuilding can report progress on
`.status.startup` without joining either ready queue. Consumers ask the ready
status subject first and use startup status only when no queryable provider is
available, so rolling replacement does not make a healthy search service look
unready. Provider cursors are portable between compatible replicas but do not
pin an index snapshot; pagination is live and results may shift as replicas
advance.

Ship a Bleve implementation as a runtime unit under ADR-041. The same unit and
NATS contract run embedded when `search_provider.enabled` is true or standalone
through `chatto search-provider`. `search.enabled` independently controls the
consumer-facing API and UI.

The bundled provider is a projection of `EVT`. It keeps a disposable local
index and may resume through the optional local-checkpoint capability from
ADR-054. The index is excluded from Chatto backups, can be reconstructed from
retained event history, and never becomes a source of domain truth. Chatto does
not recursively delete an unreadable or incompatible configured disk index;
the provider fails startup until the operator moves or removes it.

Search is server-local. Cross-server or federated search would need a separate
authorization, ranking, and pagination decision.

## Consequences

Bundled and third-party providers exercise the same integration boundary.
Operators can change process topology or matching engines without changing the
public client contract.

The main process remains the sole end-user authorization boundary. Providers
do not need to reproduce current membership rules, and stale or malformed hits
are discarded before reaching clients.

Search providers are trusted infrastructure because they receive enough data
to build or query a plaintext-derived content index. Operators must restrict
their NATS permissions, protect their storage, and exclude that storage from
ordinary backups.

Provider availability degrades only Search. The discovery capability reports
protocol support, while configuration and provider readiness remain separate
runtime states.
