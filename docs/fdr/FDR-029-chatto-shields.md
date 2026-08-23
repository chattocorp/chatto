# FDR-029: Chatto Shields

**Status:** Active
**Last reviewed:** 2026-08-23

## Overview

Chatto Shields are optional public badges for self-hosted communities.
Operators can put these badges in READMEs, project pages, and websites. A badge
shows an aggregate account count or presence count without a Chatto session.

## Behavior

- An operator enables shields with `webserver.shields.enabled` or
  `CHATTO_WEBSERVER_SHIELDS_ENABLED`. Chatto returns Not Found for shield URLs
  when shields are disabled.
- `/.well-known/chatto/shields/online.json` returns Shields.io endpoint JSON.
  It counts each user who has a current live presence record. Online, Away, and
  Do Not Disturb all count. The badge measures presence, not the selected
  availability state.
- `/.well-known/chatto/shields/registered.json` returns Shields.io endpoint
  JSON. It counts verified accounts and does not count unverified accounts.
- The related `.png` URLs redirect to the Shields.io endpoint renderer. Each
  redirect uses the related Chatto JSON endpoint as its data source.
- Chatto supplies the metric labels and colors. Shields.io supplies the default
  image style.
- Shield responses show only aggregate counts. They do not show user
  identities, individual presence states, or individual activity.

## Design Decisions

### 1. Keep public counts optional

**Decision:** Keep community shields disabled by default. An operator must
enable them with an explicit setting.

**Why:** Public READMEs and websites are unauthenticated and can be cached.
Community size and live presence can be sensitive on private or small servers.

**Tradeoff:** Badge use requires one more setting.

### 2. Use the well-known public namespace

**Decision:** Put shields under `/.well-known/chatto/shields/`. Do not use a
top-level `/shields/` route.

**Why:** Badge metadata is public discovery information. The well-known
namespace prevents unrelated integration routes at the Chatto root URL.

**Tradeoff:** The URLs are longer. The documentation supplies Markdown
examples, and the `.png` redirects hide the longer Shields.io URL.

### 3. Let Shields.io render the image

**Decision:** Return Shields.io endpoint JSON. Redirect the short `.png` URLs
to the hosted Shields.io renderer.

**Why:** Markdown renderers, static websites, and project pages use image
badges. Shields.io owns the badge typography, rasterization, and style
compatibility.

**Tradeoff:** The viewer must have access to Shields.io. Shields.io must also
have access to the public Chatto JSON endpoint. An operator can use only the
JSON endpoint or keep shields disabled.

### 4. Use plain HTTP

**Decision:** Use small public HTTP endpoints. Do not add a ConnectRPC service
for shields.

**Why:** Shields.io uses one JSON document with its endpoint schema. A
ConnectRPC GET endpoint requires protobuf definitions and generated code. It
also puts an encoded request in the Shields.io URL parameter.

**Tradeoff:** This design adds narrow public HTTP endpoints that do not use
ConnectRPC. The endpoints supply only badge counts and are not a general
metrics API.

### 5. Expose only aggregate data

**Decision:** Expose only the online and registered aggregate counts in v1.

**Why:** Aggregate badges satisfy the README use case. They do not expose user
identity, individual presence state, or more operational data. Presence is live
runtime state. When a user is offline, there is no presence record.

**Tradeoff:** A badge cannot show the cause of a count change. It cannot
distinguish Online from Away or Do Not Disturb.

## Related

- **ADRs:** ADR-001 (NATS JetStream as Primary Data Store), ADR-036 (Persist
  Runtime State in RUNTIME_STATE)
- **FDRs:** FDR-011 (User Presence), FDR-021 (Admin Dashboard & System
  Monitoring), FDR-023 (Authentication & Sessions)
