# FDR-028: Operator API & CLI

**Status:** Active
**Last reviewed:** 2026-09-23

## Overview

The Operator API gives server operators local, root-equivalent user administration and channel room management outside the in-app RBAC model. It exists for bootstrap, recovery, and scripted operations where no suitable user session exists yet or where an action should be attributed to Chatto's system actor rather than a human account.

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
- `chatto operator room list` shows active and archived channel rooms without a user session or room membership. Operators can filter by the exact stored name. Each result includes the room ID, name, description, group ID, and archived state.
- Room pages use stable room ID order and include a total count and next-page status. Pages are live reads; room changes between requests can move results between offsets. A repeated read is safe.
- `chatto operator room create` creates a channel as the system actor with normal name and description checks. An omitted group selects the current default group.
- `chatto operator room member add` adds an existing user as an explicit member of a channel room. An existing membership succeeds without another join fact. Missing resources, DM rooms, archived rooms, universal rooms, and active bans prevent the add.
- CLI clients read the socket path from `--operator-socket`, `CHATTO_OPERATOR_API_SOCKET_PATH`, or `operator_api.socket_path` in `chatto.toml`.
- Password-setting commands prompt on interactive terminals when a password flag is not supplied. Non-interactive use must pass the password explicitly with `--password-stdin`, `--password-file`, or `--password`.
- User deletion is irreversible and requires `--yes` in non-interactive use.
- Development and test builds provide `chatto operator seed` to add reproducible
  synthetic users, channels, messages, and thread replies. Counts and a random
  seed select the dataset. Room membership varies. Joins and some leaves occur
  between messages; authors are current members. Each room keeps at least one
  member. The manifest lists final generated members. Activity is ordered
  without artificial waits or backdated timestamps.
  Accounts have no password or owner role. JSON output provides the created IDs
  and content for later operations and test assertions.
- A pinned data generator supplies natural names, room topics, and text. Visible
  resources contain no dataset labels or message markers. Tests use IDs and
  content from the manifest. Existing name collisions receive numeric suffixes.
- Seeding retains existing data and adds a dataset on each call. A failed
  run can leave partial data and must not be retried automatically. Release
  builds do not provide seeding. Tests can call a test-only HTTP wrapper and
  create cookie sessions for generated users without password setup.

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

### 6. Development data uses ordinary domain operations

**Decision:** Synthetic data uses the existing account, membership, and message
operations, with normal encryption and message permission checks. Content and
relationships are reproducible; IDs and timestamps are not fixed.
**Why:** Test and demonstration data must behave like real conversations,
including thread state and live updates.
**Tradeoff:** This costs more than direct event injection. The fixed performance
fixture remains separate so its workload stays comparable.

### 7. Operator channel lookup

**Decision:** Channel lookup includes archived rooms and retains distinct results when names match. It uses room ID order for pages.
**Why:** A script must recover stable IDs after an interrupted operation. Archived rooms and names that match more than one room must not be hidden or guessed away.
**Tradeoff:** Offset pages can shift when rooms change during a scan. Scripts can repeat the read and compare IDs.

### 8. Channel creation and import scripts

**Decision:** Channel creation accepts room fields and returns the created room ID. Import scripts keep their own mapping from source IDs to Chatto IDs and use channel lookup to recover from an uncertain response.
**Why:** Source identity and retry policy belong to each import script. The Operator API provides room creation and lookup without storing external source keys.
**Tradeoff:** A script must resolve name matches when recovering an ID after a lost response. A repeated create request with the same name returns a conflict.

### 9. Operator channel membership

**Decision:** The local Operator API adds explicit channel members through the existing membership operation as the system actor. A repeat for a current member returns the member without another join fact.
**Why:** Import scripts must make mapped users members before they can refer to them as historical room participants. Membership is already a set in Chatto, so no import source key is needed.
**Tradeoff:** The operation keeps normal room limits: it cannot add explicit members to archived, universal, or DM rooms, and it cannot bypass an active room ban.

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

- **ADRs:** ADR-042 (protobuf-first public API), ADR-044 (ConnectRPC service conventions), ADR-085 (user-scoped MCP integration)
- **FDRs:** FDR-018 (Account Lifecycle), FDR-021 (Admin Dashboard & System Monitoring), FDR-023 (Authentication & Sessions), FDR-043 (Model Context Protocol Integration)
