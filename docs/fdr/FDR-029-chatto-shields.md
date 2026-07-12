# FDR-029: Chatto Shields

**Status:** Active
**Last reviewed:** 2026-07-05

## Overview

Chatto Shields are opt-in public PNG badges that self-hosted communities can embed in READMEs, project pages, and websites. They expose a small aggregate view of community size and current activity without requiring a Chatto session.

## Behavior

- Operators enable shields explicitly in configuration. Disabled servers return Not Found for shield URLs.
- `online.png` shows the number of users with any current live presence record. Online, Away, and Do Not Disturb all count because the badge answers "how many members are currently present," not which availability state each member selected.
- `registered.png` shows the number of verified accounts. Unverified accounts are excluded.
- Shields are fixed-style PNG images with short public caching. v1 does not support query-customized labels, colors, or styles.
- Shield responses expose only aggregate counts. They do not expose user identities, per-status presence breakdowns, or per-user activity.

## Design Decisions

### 1. Opt-in public counts

**Decision:** Community shields are disabled by default and enabled only when the operator opts in.
**Why:** Public READMEs and websites are unauthenticated, cacheable surfaces. Community size and live activity can be sensitive for private or small servers.
**Tradeoff:** Operators must configure one extra setting before they can embed badges.

### 2. PNG assets instead of ConnectRPC

**Decision:** Shields are served as plain PNG HTTP assets, not as a protobuf API.
**Why:** The primary consumers are Markdown renderers, static websites, and social/project pages that expect image URLs.
**Tradeoff:** The surface is intentionally narrow. Integrations that need structured metrics should use the authenticated API or Prometheus/exporter surfaces instead.

### 3. Aggregate-only privacy boundary

**Decision:** v1 exposes only the online and registered aggregate counts.
**Why:** Aggregate badges satisfy the README use case while avoiding per-user identity, per-status presence, and richer operational telemetry on a public endpoint. Presence remains live runtime state; offline is still represented by absence.
**Tradeoff:** The public badge cannot explain why a count changed or distinguish Online from Away or DND.

## Related

- **ADRs:** ADR-001 (NATS JetStream as Primary Data Store), ADR-036 (Persist Runtime State in RUNTIME_STATE)
- **FDRs:** FDR-011 (User Presence), FDR-021 (Admin Dashboard & System Monitoring), FDR-023 (Authentication & Sessions)
