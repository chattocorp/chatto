# ADR-074: Keep the Frontend Server Catalogue Device-Local

**Date:** 2026-08-14

## Status

Accepted

## Context

The multi-server frontend separates public server metadata from device-local
Chatto sessions. It later synchronized that metadata through Authling's
experimental account-data service, which required a second OAuth client,
frontend bootstrap configuration, an application-data scope, and a TinyBase
runtime alongside normal Chatto login.

Authling is being narrowed to identity-provider state. A known-server list is
application preference data, not identity state, and does not need to be
available on every device for the multi-server client to work.

## Decision

The frontend server catalogue and all per-server sessions are device-local.
They remain separate state owners so a known server can be signed out, but
neither is synchronized through Authling or another global identity provider.

Authling is treated like any other OIDC provider configured by a Chatto server
operator. The frontend has no global Authling session, issuer selection, or
provider auto-selection path. The `/client-config.json` bootstrap,
`frontend.authling_issuer` configuration, account-data callback, and associated
frontend CIMD redirect are removed.

During upgrade, persisted registrations with the former `local` or `synced`
provenance are accepted and rewritten as ordinary device-local catalogue
entries. Existing bearer tokens and cached local user summaries remain local.
The retired Authling authorization, device ID, and TinyBase local-storage keys
are cleared. No request is made to delete data from Authling.

Signing out of all servers revokes reachable Chatto sessions best-effort,
clears device-local sessions and remote catalogue entries, and retains a
configured origin server as signed out. It has no Authling-specific cleanup.

## Consequences

- Multi-server routing and signed-out known servers continue to work on one
  device.
- A new browser or device starts with an empty catalogue except for an
  auto-detected origin server; users add or connect servers again.
- The frontend no longer holds an Authling application-data token or TinyBase
  state, reducing its authentication and persistence surface.
- Old synced registrations are preserved on the upgrading device, but changes
  no longer propagate between devices.
- ADR-064 is superseded. ADR-025 remains current for multi-server routing and
  device-local authentication, with its synchronization sections replaced by
  this decision.

## Compatibility

This removes experimental public configuration and HTTP behavior before
Chatto 1.0. An older frontend served by a newer server receives `404` for
`/client-config.json` and cannot start account-data synchronization, but its
ordinary Chatto connections still work. A newer frontend ignores the removed
configuration when served by an older server. Authling rejects the retired
scope, while normal `openid` server login remains compatible.

## Related

- [ADR-025: Multi-Server Client Architecture](ADR-025-multi-instance-client-architecture.md)
- [ADR-064: Separate the Frontend Server Catalogue from Device Sessions](ADR-064-separate-server-catalog-and-sessions.md)
- [ADR-071: Identify Open OAuth Clients through CIMD](ADR-071-cimd-identified-open-oauth-clients.md)
- [Authling ADR-007: Limit Authling to Identity-Provider State](../../authling/docs/adr/ADR-007-limit-authling-to-identity-provider.md)
