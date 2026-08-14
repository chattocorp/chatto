# FDR-005: Account Data Synchronization

**Status:** Retired
**Last reviewed:** 2026-08-14

## Overview

Authling briefly provided an experimental TinyBase-backed account-data service
for synchronizing Chatto server registrations. ADR-007 removed this feature to
keep Authling focused on identity-provider state.

## Retired Behavior

The following behavior is no longer available:

- the `account_data` OIDC scope;
- the `/data/sync` WebSocket endpoint;
- the `AUTHLING_USER_DATA` JetStream bucket;
- TinyBase synchronization and its wire-compatibility proof; and
- Chatto's Authling-backed server-catalogue synchronization.

Authling does not read or automatically delete an existing experimental
`AUTHLING_USER_DATA` bucket. Chatto migrates registrations already present in
its browser cache into ordinary device-local registrations and removes its
retired authorization and TinyBase cache keys.

## Related

- **Superseding decision:** [ADR-007](../adr/ADR-007-limit-authling-to-identity-provider.md)
- **Current identity feature:** [FDR-004](FDR-004-openid-connect-provider.md)
