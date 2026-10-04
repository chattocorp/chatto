# FDR-049: Primary Host Redirect

**Status:** Active
**Last reviewed:** 2026-10-04

## Overview

A server operator can direct browser navigation from server aliases to the
primary URL. This gives users one browser address and avoids separate sessions
on aliases.

## Behavior

- Redirects are disabled by default.
- When enabled, HTML browser page requests on exact configured aliases go to
  the primary URL. The path and query stay intact.
- Requests already on the primary host do not redirect. Unknown hosts and
  wildcard origin entries do not select an alias.
- OAuth callbacks, APIs, MCP, assets, and WebSocket connections keep their
  requested host. This includes the remote-server popup callback.
- Redirects are temporary and cannot be cached. Disabling the setting stops
  them after the new configuration takes effect.
- Cookies remain specific to each host. A user can need to sign in at the
  primary URL even when they already have a session on an alias.

## Design Decisions

### 1. Redirect browser pages in Chatto

**Decision:** Redirect HTML page navigation instead of all traffic at a proxy.
**Why:** Protocol clients and OAuth flows depend on their requested origin.
**Tradeoff:** The deployment needs a Chatto release with redirect support.

### 2. Use an opt-in, temporary redirect

**Decision:** Keep redirects disabled by default and do not cache them.
**Why:** Operators can test the behavior and recover without persistent browser
redirects. A configured primary URL can point to another deployment.
**Tradeoff:** Aliases can continue to have independent browser sessions until
the operator enables the setting.

## Gates

- The server operator enables `webserver.redirect_to_primary_host` and
  configures the primary URL and exact server aliases.
- The primary URL must point to the intended server before redirects are enabled.

## Related

- [FDR-023: Authentication and Sessions](FDR-023-authentication-and-sessions.md)
- [FDR-027: PWA and Service Worker](FDR-027-pwa-and-service-worker.md)
