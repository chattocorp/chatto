# FDR-028: Operator Admin API & CLI

**Status:** Active
**Last reviewed:** 2026-06-27

## Overview

The Operator Admin API gives server operators a token-authenticated automation surface for user administration outside the in-app RBAC model. It exists for bootstrap, recovery, and scripted operations where no suitable user session exists yet or where the action should be attributed to Chatto's system actor rather than a human account.

## Behavior

- Operators opt in with the top-level `admin_api` configuration section. The Admin API is disabled by default.
- The Admin API accepts configured bearer tokens only from each token's configured CIDR ranges. When no ranges are configured for a token, that token accepts loopback addresses only.
- Admin API actions are attributed to the system actor. They are not tied to a Chatto user account, cookie session, bearer session, or RBAC role.
- The user-administration surface can list and look up users, create users, update login/display name, set passwords, delete users, add verified email addresses, assign server roles, and revoke server roles.
- The CLI groups these commands under `chatto admin user ...`, for example `chatto admin user create`, `chatto admin user set-password`, and `chatto admin user role add`.
- CLI clients read the server URL and admin token from flags, environment variables, or `chatto.toml`, and call the same Admin API used by other trusted operator automation.
- The CLI refuses to send admin tokens read from `chatto.toml` or counted `CHATTO_ADMIN_API_TOKENS_<index>_*` environment variables to a URL override unless the override resolves to the configured `webserver.url` / `CHATTO_WEBSERVER_URL`. Operators can still target another endpoint by passing `--admin-token` or `CHATTO_ADMIN_API_TOKEN` explicitly.
- The CLI requires HTTPS for non-loopback Admin API URLs. Plain HTTP is accepted only for loopback hosts.
- Password-setting commands prompt on interactive terminals when a password flag is not supplied. Non-interactive use must pass the password explicitly.
- User deletion is irreversible and requires `--yes` in non-interactive use.

## Design Decisions

### 1. Top-level `admin_api` configuration

**Decision:** Admin API configuration lives under `admin_api`, with environment variables prefixed `CHATTO_ADMIN_API_`.
**Why:** This names the capability being exposed: an opt-in administrative HTTP/ConnectRPC surface. It avoids overloading `auth`, which is about user authentication, and avoids the ambiguity of a generic `admin` section that could refer to the in-app admin UI or RBAC-admin behavior.
**Tradeoff:** Operators see one more top-level config section. The separation is worth the clarity because this token has a different threat model than user auth settings.

### 2. Named token entries with per-token CIDR allow-lists

**Decision:** Requests need one configured named admin bearer token and must originate from a direct peer IP allowed by that token's CIDR list. The per-token default allow-list is loopback-only.
**Why:** Admin tokens are root operator credentials. Named entries make rotation practical, and per-token CIDRs let operators keep a local CLI token loopback-only while giving a sidecar or automation token a narrow private subnet. CIDR gating reduces the impact of accidentally exposing the endpoint through a public route.
**Tradeoff:** Deployments behind reverse proxies must ensure the TCP peer Chatto sees is in the allowed range for the token being used. Forwarded headers are deliberately not trusted for this gate.

### 3. System actor attribution

**Decision:** Admin API writes use the system actor rather than impersonating a user or requiring an owner/admin login.
**Why:** Bootstrap and recovery often happen before a suitable user session exists. System attribution is explicit in audit/history and avoids creating hidden coupling to one operator's account lifecycle.
**Tradeoff:** The API bypasses RBAC by design. Operators must protect the admin token and network path accordingly.

### 4. ConnectRPC instead of GraphQL

**Decision:** The new operator API is ConnectRPC/protobuf-first.
**Why:** New public API surface should move toward protobuf-first contracts, and operator automation benefits from generated clients and stable request/response schemas.
**Tradeoff:** The in-app admin UI still uses GraphQL today, so there are two admin-adjacent API shapes. The Admin API is intentionally narrower and focused on external operator automation.

### 5. CLI grouped under `chatto admin user`

**Decision:** User administration commands live under `chatto admin user ...`.
**Why:** The extra `admin` segment makes it clear that these commands use the Admin API and carry operator-level authority. It also leaves room for future admin subcommands that are not user-specific.
**Tradeoff:** Commands are more verbose than `chatto user ...`, but the grouping makes the privilege boundary visible at the call site.

### 6. CLI token forwarding safeguards

**Decision:** Config-file and counted environment admin tokens are bound to the configured `webserver.url` / `CHATTO_WEBSERVER_URL` by default. URL overrides require a one-off token source (`--admin-token` or `CHATTO_ADMIN_API_TOKEN`), and non-loopback URLs must use HTTPS.
**Why:** `chatto.toml` often contains the local instance's root operator credential. A mistyped or malicious `--url` should not silently forward that credential to a different server, and tokens should not cross non-loopback networks in cleartext.
**Tradeoff:** Some scripted remote-administration flows need one extra token flag or environment variable. That explicitness is acceptable for root-level credentials.

## Permissions

The Admin API is not gated by Chatto RBAC permissions. It is gated by `[admin_api]` enablement, named bearer-token authentication, and per-token CIDR allow-listing.

## Related

- **ADRs:** ADR-042 (protobuf-first public API), ADR-044 (ConnectRPC service conventions)
- **FDRs:** FDR-018 (Account Lifecycle), FDR-021 (Admin Dashboard & System Monitoring), FDR-023 (Authentication & Sessions)
