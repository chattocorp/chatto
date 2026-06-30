# FDR-028: Operator API & CLI

**Status:** Active
**Last reviewed:** 2026-06-29

## Overview

The Operator API gives server operators a local, root-equivalent user administration surface outside the in-app RBAC model. It exists for bootstrap, recovery, and scripted operations where no suitable user session exists yet or where the action should be attributed to Chatto's system actor rather than a human account.

## Behavior

- Operators opt in with the top-level `operator_api` configuration section. The Operator API is disabled by default.
- When enabled, Chatto serves `chatto.operator.v1` on a Unix socket. The default path is `/tmp/chatto/operator.sock`; the socket mode is fixed at `0600`.
- The operator socket serves only the Operator API. It does not serve `chatto.api.v1` or `chatto.admin.v1`.
- The public web listener does not serve `chatto.operator.v1`.
- There are no operator bearer tokens, CIDR allow-lists, sessions, cookies, or CORS policy in this local-socket model. Socket filesystem permissions are the access boundary.
- The server refuses to start if the operator socket parent directory is not private to the Chatto process user or if an existing operator socket has a mode other than `0600`. A stale existing socket with mode `0600` may be removed and replaced.
- Operator actions are attributed to the system actor. They are not tied to a Chatto user account, cookie session, bearer session, or RBAC role.
- The user-administration surface lives in `chatto.operator.v1.OperatorUserService` and can list and look up users, create users, update login/display name, set passwords, delete users, add verified email addresses, assign roles, and revoke roles.
- The CLI groups these commands under `chatto operator user ...`, for example `chatto operator user create`, `chatto operator user set-password`, and `chatto operator user role add`.
- CLI clients read the socket path from `--operator-socket`, `CHATTO_OPERATOR_API_SOCKET_PATH`, or `operator_api.socket_path` in `chatto.toml`.
- Password-setting commands prompt on interactive terminals when a password flag is not supplied. Non-interactive use must pass the password explicitly with `--password-stdin`, `--password-file`, or `--password`.
- User deletion is irreversible and requires `--yes` in non-interactive use.

## Design Decisions

### 1. Top-level `operator_api` configuration

**Decision:** Operator API configuration lives under `operator_api`, with environment variables prefixed `CHATTO_OPERATOR_API_`.
**Why:** This names the capability being exposed: local root-equivalent operator control. It avoids overloading public Admin API terminology, which is user-authenticated and RBAC-gated.
**Tradeoff:** Operators see one more top-level config section. The separation is worth the clarity because this surface has a different threat model than user authentication or public admin UI settings.

### 2. Unix socket instead of TCP tokens

**Decision:** The initial Operator API transport is a Unix socket, not a TCP listener with bearer tokens.
**Why:** Operator commands must mutate state inside the already-running Chatto process, especially when embedded NATS is in use. A Unix socket avoids a second store writer, avoids accidental public network exposure, and avoids plaintext root tokens in config or environment variables.
**Tradeoff:** Remote automation must run on the host/container or deliberately share the socket with a trusted sidecar/container. A future TCP mode would need a separate design review.

### 3. Dedicated protobuf package

**Decision:** Root-equivalent operator RPCs live in `chatto.operator.v1`, separate from public `chatto.admin.v1`.
**Why:** Public Admin API calls use user authentication and RBAC. Operator calls bypass RBAC by design and should not share a service surface with public clients.
**Tradeoff:** Some protobuf shapes overlap with admin-member UI data, but the service boundary is explicit and transport mounting can enforce it mechanically.

### 4. Socket mode is strict

**Decision:** Chatto verifies the socket mode at startup and refuses to boot when an existing socket has a different mode than configured.
**Why:** Silent chmod of a pre-existing socket can hide packaging or deployment mistakes around a root-equivalent control surface.
**Tradeoff:** Operators must fix stale or incorrectly provisioned socket files instead of relying on Chatto to repair them automatically.

### 5. Docker-first default path

**Decision:** The default socket path is `/tmp/chatto/operator.sock`, not `/run` or `/var/run`.
**Why:** Many self-hosters run Chatto in Docker, where `/tmp` is writable without extra packaging setup and `docker exec chatto chatto operator ...` works without mounting host runtime directories.
**Tradeoff:** System packages should override the path to `/run/chatto/operator.sock` when that is more idiomatic for their service manager.

## Permissions

Operator API access is not gated by Chatto RBAC permissions. It is gated by local operating-system access to the configured Unix socket. Treat socket access as root-equivalent Chatto authority.

For Docker, the preferred workflow is to run operator commands inside the running container:

```sh
docker exec -it -u chatto chatto /chatto operator user list
docker exec -it -u chatto chatto /chatto operator user list --search alice
docker exec -it -u chatto chatto /chatto operator user list --search alice@example.com
docker exec -it -u chatto chatto /chatto operator user set-password USER_ID
```

Sharing the socket with another container or host path is advanced usage and should only be done for trusted operator tooling.

## Related

- **ADRs:** ADR-042 (protobuf-first public API), ADR-044 (ConnectRPC service conventions)
- **FDRs:** FDR-018 (Account Lifecycle), FDR-021 (Admin Dashboard & System Monitoring), FDR-023 (Authentication & Sessions)
