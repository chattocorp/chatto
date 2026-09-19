# ADR-099: Package Chatto Mobile with Capacitor

**Date:** 2026-09-19

**Status:** Accepted

## Context

Chatto Desktop already bundles the shared frontend. Mobile needs the same
multi-server client with access to system authentication and, later, native
notification and call services. Calls must eventually continue when the phone
is locked. This first implementation focuses on the client shell and sign-in.

## Decision

Use Capacitor for an experimental iOS shell under `apps/mobile/`. Bundle the
shared static frontend at `capacitor://localhost`. Keep Android as a later
target. Do not embed a Chatto server or load remote application code.

Extend ADR-072's capability pattern to native system authentication. A focused
frontend adapter detects the `ChattoAuthorization` plugin. The iOS plugin owns
one five-minute `ASWebAuthenticationSession`; the frontend owns PKCE, callback
state validation, token exchange, and the existing per-server session records.
The verifier remains in memory. App termination requires a new sign-in attempt.

Register `eu.chattocorp.chatto.mobile` as a built-in public OAuth client with the exact
callback `eu.chattocorp.chatto.mobile:/oauth/callback`. Keep server consent, client policy,
and PKCE checks. This is an additive registration; older servers do not know it.

Use the persistent webview store for prototype credentials. A native credential
store requires a separate lifecycle design before production hardening. Do not
add background modes until native code implements their intended behaviour.
Native call ownership and push delivery remain separate follow-up decisions.

## Consequences

Browser, desktop, and mobile reuse the same application UI and server session
logic. iOS uses system WebKit, so webview limitations remain. Native media can
be added without replacing the shared UI. The mobile runtime has no backend,
NATS resources, or new persisted domain protocol.

Mobile sign-in requires an updated server and HTTPS. Browser and desktop sign-in
remain unchanged. Xcode and a Simulator runtime are needed for local verification;
physical-device installation also needs development signing. Manual TestFlight
uploads use signed Xcode archives. Public App Store distribution and release
automation remain deferred.
