# ADR-022: NanoID with Entity-Type Prefixes

**Date:** 2026-03-01

## Context

Every entity in Chatto (users, spaces, rooms, events, assets, notifications) needs a unique identifier. The options include UUIDs (128-bit, standard but verbose), auto-incrementing integers (simple but leak ordering and count), and NanoID (compact, customizable alphabet, configurable entropy).

Additionally, when debugging production issues or reading logs, it's helpful to immediately know what *type* of entity an ID refers to without additional context.

## Decision

Use 14-character NanoIDs with an alphanumeric alphabet (~83.4 bits of entropy) prefixed by a single letter indicating the entity type:

| Prefix | Entity |
|--------|--------|
| `U` | User |
| `S` | Space |
| `R` | Room |
| `A` | Asset |
| `E` | Event |
| `N` | Notification |

Opaque tokens use two-letter prefixes: `PR` (password reset), `RG` (registration completion), and `AD` (account deletion). Email verification codes are six-digit numeric OTPs and do not use NanoID prefixes.

DM room IDs are a special case: deterministic SHA-256 hex truncated to 14 characters, with no prefix (since they're computed from participant IDs, not randomly generated).

## Consequences

- **Self-describing IDs**: Seeing `U8kX2mP4qR7nYw` in a log immediately tells you it's a user. No need to cross-reference which table or bucket it came from.
- **Compact**: 15 characters total (1 prefix + 14 NanoID) vs. 36 for a UUID. Matters for NATS subject paths where IDs are embedded in subjects.
- **URL-safe**: The alphanumeric alphabet avoids characters that need URL encoding. IDs can appear in URLs, NATS subjects, and KV keys without escaping.
- **Visual consistency with DM IDs**: The 14-character NanoID length matches the 14-character SHA-256 hex truncation used for DM room IDs, so all IDs have roughly the same visual footprint.
- **Collision probability is acceptable**: At ~83.4 bits of entropy, the probability of collision is negligible for the expected scale (millions of entities, not billions).
