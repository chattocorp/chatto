# FDR-031: Personal Settings and Home Server

**Status:** Active
**Last reviewed:** 2026-07-15

## Overview

Chatto has a server-independent personal settings screen. It works even when no
server is registered and separates preferences that belong to a person or
device from account and notification policy that belongs to one server.

## Behavior

- A gear in the app header opens personal settings with or without registered servers.
- Theme, language, timezone, time format, notification sound, and sound shaping
  live on this screen. Per-server notification levels and Web Push registration
  remain in that server's settings.
- An authenticated server is eligible to become home only when discovery says
  its operator enabled client sync. The sole eligible server is selected
  automatically; when several are eligible, the user chooses explicitly. If
  none are eligible, settings remain on the current device.
- The server gutter marks the home server. Ordinary leave/remove actions cannot
  remove it until another home is selected.
- Language, timezone, time format, the known-server directory, and the home
  marker sync through the home server. Theme and notification-sound controls are
  cached only on the current device.
- Restored servers contain no credentials and ask the user to sign in again.
- Synced servers match existing registrations by canonical URL origin. Synced
  metadata can update presentation details but cannot replace a local origin or
  transfer credentials to another URL.
- A client's first sync and a deliberate home-server move merge both known
  directories. After a successful sync, removing a previously known server is
  propagated as a deletion.
- Moving home updates the former home's marker so clients which still contact
  it can discover and continue from the new home. A failed update is retained
  locally and retried while the client still has authenticated access to the
  former home.
- Personal caches and reconciliation baselines are scoped to the authenticated
  account on the home origin. Signing into that home as a different account
  does not reuse them; signing out of all servers explicitly clears the local
  account binding.
- When the home server is offline, the settings screen remains usable from its
  local cache and reports that sync is unavailable.
- The former per-server display route redirects to personal settings. The
  legacy server display API remains available for older clients.

## Design Decisions

### 1. Home is a normal server

**Decision:** Client sync is stored by one visibly designated server the user already joined.
**Why:** It avoids asking ordinary users for a sync-service URL and keeps Chatto
self-hostable without introducing a mandatory vendor account.
**Tradeoff:** Sync availability follows the selected server, and moving home is deliberate.

### 2. Credentials remain device-local

**Decision:** The synced server directory includes discovery metadata but no
passwords, cookies, bearer tokens, or sessions.
**Why:** Restoring navigation is useful without turning the home server into a
credential vault for unrelated servers.
**Tradeoff:** A new device requires one sign-in per restored server.

### 3. Server identity is its origin

**Decision:** Directory reconciliation matches canonical URL origins and keeps
sidebar IDs local to each device.
**Why:** A remotely supplied ID must never be able to redirect credentials held
by an existing local registration.
**Tradeoff:** Two paths on the same origin cannot represent distinct servers.

### 4. Preference scope is explicit

**Decision:** Language and time rendering sync; appearance and sound playback
remain device-local; account profile, notification levels, and push state remain server-scoped.
**Why:** Portable semantic choices should follow the person, while physical
display/audio behavior and server policy have different owners.

## Permissions

No dedicated permission. The client-sync API always scopes reads and writes
to the authenticated user and does not accept a target user ID. Operators opt
in with `[client_sync].enabled` or `CHATTO_CLIENT_SYNC_ENABLED`; the default is
off, and public discovery advertises the effective capability.

## Related

- **ADRs:** ADR-025, ADR-036, ADR-043, ADR-044, ADR-051
- **FDRs:** FDR-012, FDR-013, FDR-022, FDR-027
