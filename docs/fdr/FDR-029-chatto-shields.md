# FDR-029: Chatto Shields

**Status:** Active
**Last reviewed:** 2026-07-12

## Overview

Chatto Shields are opt-in public badges that self-hosted communities can embed in READMEs, project pages, and websites. They expose a small aggregate view of community size and current activity without requiring a Chatto session.

## Behavior

- Operators enable shields explicitly with `[webserver.shields].enabled` or `CHATTO_WEBSERVER_SHIELDS_ENABLED`. Disabled servers return Not Found for shield URLs.
- `/.well-known/chatto/shields/online.json` returns Shields.io endpoint-badge JSON for the number of users with any current live presence record. Online, Away, and Do Not Disturb all count because the badge answers "how many members are currently present," not which availability state each member selected.
- `/.well-known/chatto/shields/registered.json` returns Shields.io endpoint-badge JSON for the number of verified accounts. Unverified accounts are excluded.
- The matching `.png` URLs are convenience redirects to Shields.io's endpoint badge renderer, using the matching Chatto JSON endpoint as the data source.
- Chatto controls the metric labels and colors returned by the JSON endpoints. The default `.png` redirect uses Shields.io's default rendering style.
- Shield responses expose only aggregate counts. They do not expose user identities, per-status presence breakdowns, or per-user activity.

## Design Decisions

### 1. Opt-in public counts

**Decision:** Community shields are disabled by default and enabled only when the operator opts in.
**Why:** Public READMEs and websites are unauthenticated, cacheable surfaces. Community size and live activity can be sensitive for private or small servers.
**Tradeoff:** Operators must configure one extra setting before they can embed badges.

### 2. Well-known public namespace

**Decision:** Shields live under `/.well-known/chatto/shields/` rather than a top-level `/shields/` route.
**Why:** Badge metadata is public, discovery-like server metadata. The well-known namespace keeps Chatto's root URL space from accumulating one-off integration paths.
**Tradeoff:** The URLs are longer, so the docs provide copy-paste Markdown examples and the `.png` redirects hide the longer Shields.io URL.

### 3. Shields.io endpoint JSON instead of local image rendering

**Decision:** Chatto exposes Shields.io-compatible JSON and redirects the short `.png` URLs to Shields.io's hosted renderer.
**Why:** The primary consumers are Markdown renderers, static websites, and social/project pages that expect polished image badges. Delegating visual rendering keeps Chatto from owning badge typography, rasterization, and style compatibility.
**Tradeoff:** Rendering the `.png` badge depends on Shields.io being reachable by the viewer, and Shields.io must be able to fetch the public Chatto JSON endpoint. Operators who do not want this dependency can link to the JSON endpoint only or keep shields disabled.

### 4. Plain HTTP instead of ConnectRPC

**Decision:** Shields use small public HTTP endpoints rather than a new ConnectRPC service, even though side-effect-free ConnectRPC methods can be served through GET.
**Why:** Shields.io expects a root JSON document with its endpoint-badge schema. A ConnectRPC GET URL would require protobuf/codegen surface area and an awkward encoded `connect=v1&encoding=json&message=...` URL inside the Shields.io `url` parameter.
**Tradeoff:** This remains a narrow non-ConnectRPC public surface. It is intentionally scoped to aggregate badge data and does not become a general metrics API.

### 5. Aggregate-only privacy boundary

**Decision:** v1 exposes only the online and registered aggregate counts.
**Why:** Aggregate badges satisfy the README use case while avoiding per-user identity, per-status presence, and richer operational telemetry on a public endpoint. Presence remains live runtime state; offline is still represented by absence.
**Tradeoff:** The public badge cannot explain why a count changed or distinguish Online from Away or DND.

## Related

- **ADRs:** ADR-001 (NATS JetStream as Primary Data Store), ADR-036 (Persist Runtime State in RUNTIME_STATE)
- **FDRs:** FDR-011 (User Presence), FDR-021 (Admin Dashboard & System Monitoring), FDR-023 (Authentication & Sessions)
