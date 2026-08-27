# ADR-022: NanoID with Entity-Type Prefixes

**Date:** 2026-03-01

**Updated:** 2026-08-23

## Context

Chatto gives each durable entity a unique identifier. Identifier formats have
different costs.

UUIDs are standard, but they contain 36 characters. Sequential integers are
short, but they show creation order and approximate entity counts. NanoID can
use a selected alphabet and length.

Logs and diagnostic output can show an identifier without more context. A
type prefix lets an operator identify the entity type immediately.

## Decision

Use a 14-character NanoID body with an alphanumeric alphabet. This format gives
approximately 83.4 bits of entropy.

Add one or more prefix characters that identify the entity type or token
purpose. Chatto uses these prefixes for primary entity identifiers:

| Prefix | Entity type |
|--------|-------------|
| `U` | User |
| `S` | Legacy space |
| `R` | Room |
| `C` | Call |
| `CP` | Call media publisher |
| `G` | Room group |
| `L` | Sidebar link |
| `A` | Asset |
| `I` | Invitation |
| `E` | Event |
| `N` | Legacy notification |

Multi-character prefixes also identify opaque tokens. Current examples include
password-reset (`PR`), registration-completion (`RG`), external-identity
(`EC`, `EL`, `ELS`), and account-deletion (`AD`) tokens.

Some credential formats add the `cht_` marker before the NanoID prefix. These
formats include access tokens (`cht_AT`), link-preview tokens (`cht_LP`), and
OAuth authorization codes (`cht_AC`).

Email verification codes use six numeric digits. They do not use a NanoID
prefix.

DM room identifiers are a special case. Chatto sorts the participant
identifiers and calculates a SHA-256 hash. It uses the first 14 hexadecimal
characters without a prefix.

Notifications 2.0 uses deterministic identifiers. An occurrence identifier
starts with `ntf_`, and a lifecycle event identifier starts with `nte_`.
Deterministic identity makes retries and replay idempotent. See ADR-076.

## Consequences

- **Visible entity type:** An operator can identify the entity type from most
  identifiers without a second lookup.
- **Compact format:** Most entity identifiers contain 15 characters. A UUID
  contains 36 characters. Short identifiers keep NATS subjects short.
- **URL-safe format:** URL encoding is not necessary for the alphanumeric
  alphabet. Identifiers can occur in URLs, NATS subjects, and KV keys.
- **Same length:** A DM room identifier has the same 14-character body
  length as a NanoID.
- **Small collision probability:** The selected entropy is sufficient at a
  scale of millions of entities.
- **Type disclosure:** The prefix shows the identifier type. An identifier is a
  reference. It is not a secret.
