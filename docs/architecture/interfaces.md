# Interface Inventory

Key files: [`cli/internal/connectapi/api.go`](../../cli/internal/connectapi/api.go),
[`cli/internal/http_server/connect.go`](../../cli/internal/http_server/connect.go),
[`cli/internal/http_server/auth.go`](../../cli/internal/http_server/auth.go),
[`cli/internal/http_server/realtime.go`](../../cli/internal/http_server/realtime.go),
[`proto/chatto/`](../../proto/chatto/)

This inventory records mounted transport and service boundaries. The generated
[ConnectRPC API reference](../../apps/docs-website/src/content/docs/reference/connectrpc-api/index.mdx)
is authoritative for individual RPCs, request and response messages, and public
method documentation.

Related decisions: [ADR-044](../adr/ADR-044-connectrpc-service-conventions.md),
[ADR-045](../adr/ADR-045-public-api-stability-tiers.md),
[ADR-053](../adr/ADR-053-versioned-nats-service-namespaces.md),
[ADR-079](../adr/ADR-079-renewable-bearer-sessions.md), and
[ADR-084](../adr/ADR-084-separate-internal-protobufs-by-storage-contract.md).

## Transport boundaries

| Surface | Mount | Contract | Access boundary |
| ------- | ----- | -------- | --------------- |
| Public ConnectRPC | `/api/connect/chatto.{auth,discovery,api,admin}.v1.*` | Unary Connect, gRPC, and gRPC-Web services | Explicit per-service public or authenticated-user policy; method-level authorization remains inside operation models |
| Browser authentication | `GET /auth/browser/csrf`, `POST /auth/browser/login`, `POST /auth/browser/register/complete`, `POST /auth/browser/logout`, `POST /auth/browser/session/migrate`, `POST /auth/browser/session/renew`, `POST /auth/browser/revoke-bearer-session` | Bound CSRF-proof repair, cookie-only password/registration authentication, one-time 0.4 typed-cookie migration, logout, stable-handle session renewal, and removal of stored origin bearer authority | Every mutation requires JSON and an exact same-origin request. A browser-auth mode header, if present, must select cookies. Browser routes treat an absent header as cookie mode. Renewal and logout also require signed double-submit CSRF proof while a valid cookie authority exists. Migration uses the independent browser-route proof because it runs before a current cookie session exists. Logout can clear invalid session cookies with the same proof. The safe CSRF route requires a valid cookie session. These routes do not return bearer credentials. |
| Programmatic authentication | `POST /auth/login`, `POST /auth/register/complete`, `POST /auth/logout`, `POST /oauth/token` | First-party bearer issuance, stable bearer-session revocation, and OAuth code/refresh exchange | JSON is required for direct login and registration. These routes do not create, read, or clear ambient browser authentication cookies. OAuth token exchange also accepts the documented form encoding. |
| Realtime WebSocket | `GET /api/realtime` | Binary `chatto.realtime.v1.Realtime*` frames | Bearer access token in the hello frame or same-origin cookie; exact human credentials are revalidated before subscription and once per minute; bearer expiry and cookie renewal thresholds request reconnects, while OAuth-client blocks terminate matching established sessions |
| Bot incoming webhook | `POST /webhooks/incoming/{credential}` with optional `room_id` query parameter | Slack-compatible plain-text JSON subset with Chatto aliases and optional thread creation | Action-limited bot webhook credential; the handler posts through the normal message operation and does not accept the bot API key |
| Server OIDC client metadata | `GET /oauth/client-metadata.json` | CIMD public-client identity and exact callbacks for Chatto server login | Public; mounted only when an OIDC provider uses this deployment's metadata URL as its client ID |
| Frontend OAuth client metadata | `GET /oauth/frontend-client-metadata.json` | CIMD public-client identity and exact popup callback for connecting the bundled frontend to Chatto servers | Public; always mounted, but publishes metadata only when the request host matches `webserver.url` or an exact non-wildcard `webserver.allowed_origins` entry |
| Chatto client authorization | `GET /oauth/authorize`, `POST /oauth/token` | Authorization Code with S256 PKCE plus rotating refresh grant for a client application connecting to a Chatto server; browser clients use a CIMD URL `client_id`, Desktop uses its built-in identity, and an optional `provider_id` hint can start one server-configured login provider | Public authorization start and CORS token/refresh exchange; the validated client identity and exact callback are bound through code exchange, refresh remains client-bound, and provider hints cannot supply an issuer or endpoint |
| Protected attachments | `GET /assets/files/{assetId}` and image transform variants | Per-user URLs use hourly issuance buckets with 23–24 hours of remaining validity; Chatto streams full responses, while passive S3-backed video, audio, and large files can redirect to short-lived presigned URLs | Signed `access` ticket, authenticated cookie, or bearer token; every request rechecks room membership before resolving storage or exposing binary bytes |
| Protected HLS video | `GET /assets/hls/{assetId}/master.m3u8`, rendition playlists, and segments | Master and media playlists are generated from the durable manifest; segments are complete bounded responses from NATS or S3 | Domain-separated source-video `access` ticket; every request rechecks room membership and every segment ID/role against the durable HLS manifest |
| Operator ConnectRPC | `/api/connect/chatto.operator.v1.*` on the configured Unix socket | Root-equivalent local unary services | Unix-socket filesystem permissions; never mounted on the public listener |
| Trusted NATS services | `svc.chatto.>` and `svc.chatto_ext.>` | Versioned protobuf request/reply through NATS micro services | NATS account permissions; extension providers receive only their configured service and upstream Core subjects |
| Reflection | `/api/connect/grpc.reflection.v1*` and `v1alpha*` | Public service descriptors | Public; restricted resolver excludes internal `chatto.core.*` types |

The public HTTP edge mounts every handler returned by `connectapi.API.Handlers`.
Authenticated services are wrapped with `connectrpc.com/authn` before protobuf
decoding and validation. `ExternalIdentityAuthService`,
`PushSubscriptionCleanupService`, `ServerDiscoveryService`, and reflection are
public; all other public-listener services require an authenticated user. The Operator API uses
`connectapi.API.OperatorHandlers` and is mounted only on the configured Unix
socket.

## Mounted public services

| Package | Public services | Auth policy |
| ------- | --------------- | ----------- |
| `chatto.auth.v1` | `ExternalIdentityAuthService`, `PushSubscriptionCleanupService` | Public capability-token flows |
| `chatto.discovery.v1` | `ServerDiscoveryService` | Public discovery |
| `chatto.api.v1` | `AssetService`, `AssetUploadService`, `BotService`, `MessageSearchService`, `MessageService`, `MyAccountService`, `NotificationPolicyService`, `NotificationService`, `PushNotificationService`, `RoleService`, `RoomDirectoryService`, `RoomService`, `ServerService`, `ThreadService`, `UserService`, `ViewerService`, `VoiceCallService` | Authenticated user |
| `chatto.admin.v1` | `AdminDiagnosticsService`, `AdminEventLogService`, `AdminInviteLinkService`, `AdminOAuthClientService`, `AdminPermissionService`, `AdminRoleService`, `AdminRoomLayoutService`, `AdminServerService`, `AdminUserService` | Authenticated user; methods enforce administrative permissions |

`AdminInviteLinkService` requires `user.invite`. Its resource includes the
full, deterministically reconstructed invite link so authorised operators can
copy it again; raw bearer tokens are not stored in `EVT`. Opening
`/invite/{token}` validates the compact capability, stores only the invitation
ID in the signed browser session, and immediately redirects to registration.

`AdminServerService` provides CRUD operations for Neighbor resources. These
methods require `server.manage-neighbors`. `ServerDiscoveryService.ListNeighbors`
returns structured recommendations with optional testimonials without a
session or an ordering contract. It also returns the legacy origin list for
older clients. The server does not contact the advertised origins.

`BotService` exposes bot lifecycle, administrator-initiated owner reassignment,
and create and revoke operations for as many as 20 named API keys and 20 named
incoming webhooks for each bot. Bot
permission reads and writes use `AdminPermissionService`'s canonical user
permission operations with the bot's user ID as the target. Human owners can
manage their own bots; `bot.manage` allows global management.

Matrix room metadata is limited to rooms visible to both the bot owner and the
managing caller; group metadata follows the room directory's complete group
layout so empty groups remain configurable. Each bot API key authenticates the
normal public and realtime surfaces, but cannot call bot-management or human
account-security operations. Reassignment requires `bot.manage`, preserves the
active keys and configured allowlist, and immediately changes the owner
permission ceiling.

API-key creation returns the raw key once. Safe metadata includes its stable
ID, manager-defined name, creation time, and best-effort last-use telemetry.
Revocation closes only established realtime connections that used the selected
key.
Incoming webhook creation returns the complete URL once. A manager replaces a
webhook when the manager creates a new credential, moves the caller, and
revokes the old credential. Each webhook can be revoked without a change to
other webhooks. Safe metadata includes the creation time and best-effort
last-use telemetry. The separate credentials cannot authenticate ConnectRPC or
realtime requests.

`NotificationPolicyService` provides explicit server, room-group, and room
policy scopes. Its batch read accepts at most 100 scopes, removes duplicates in
first-seen order, and omits missing or inaccessible resource scopes. The
existing `NotificationService` policy methods remain available for server and
room integrations.

`AdminDiagnosticsService.GetSystemInfo` is owner-only and includes
broker-derived status for Chatto's known durable worker queues. The additive
worker list is absent on older servers; clients must treat that as diagnostics
unavailable rather than as a healthy empty set.
JetStream account, stream/consumer, server-statistics, and projection telemetry
is independently optional. Message presence or the projection-availability flag
records whether collection succeeded, so one failure does not suppress unrelated
system diagnostics or turn unavailable metrics into healthy-looking zeroes.

## Mounted operator services

| Package | Service | Access policy |
| ------- | ------- | ------------- |
| `chatto.operator.v1` | `OperatorUserService` | Root-equivalent access over the private Unix socket |

## Trusted NATS services

The `chatto.search.v1` provider contract defines normalized query and readiness
messages under `svc.chatto_ext.search.v1.>`. `search.Client` validates both
sides of request/reply, maps NATS micro error headers, and treats missing
responders or the bounded provider-call deadline as provider unavailability.
Compatible providers share a queue group for replica load balancing. Ready
status and queries use `.status` and `.query`; startup progress uses
`.status.startup` only as a fallback when no ready status responder exists.
The bundled provider joins both ready queues only after replay is current.

This is a trusted server-side integration surface, not a public client API.
Query responses contain thin message and room IDs. The public
`MessageSearchService` prefilters provider queries to the caller's complete
current member-room set. It then uses
`MessageSearchReadModel` and the normal timeline hydrator to recheck room
membership, current body availability, and message/room identity before
returning canonical `Message` resources. Public cursors encrypt and authenticate
the provider cursor and bind it to the viewer and complete public request.

The bundled provider runs under `chatto run` when
`search_provider.enabled = true`; the same unit runs standalone through
`chatto search-provider`. `search.enabled` independently controls whether the
public service accepts queries. `GetStatus` preserves disabled, indexing,
ready, degraded, and unavailable states without affecting other APIs. Exact
provider replay counts stay on the trusted NATS contract and in operator logs;
the authenticated public status does not expose server-wide event-log scale.

`ServerDiscoveryService.GetServer` and `ListNeighbors` support side-effect-free
GET. They also receive wildcard public CORS and conditional-response caching.
Other bundled-client Connect traffic uses POST.

The discovery response includes the server software version as public
pre-authentication state, along with configured provider metadata and the
independently configured direct-registration and direct-login capabilities.
The direct-login capability uses scalar presence so a new client treats an
older server that omits it as enabled. The bundled client refreshes discovery
per server and owns an internal feature-to-minimum-server-version table for compatibility gates.
The 0.5 client requires the 0.5 server baseline before opening realtime
protocol 2, the only accepted behavioral version. The
`chatto.realtime.v1` suffix remains the protobuf namespace.

Public server discovery includes each OIDC provider's issuer for clients that
need to identify or present configured login options. Authling has no special
frontend trust path: a Chatto server uses it only when the operator configures
it as an ordinary OIDC provider.

`MessageSearchService.GetStatus` remains the authority for configured search
availability and transient provider readiness. Viewer permissions remain the
authority for authenticated feature access.

Public URL generation prefers the configured `webserver.url`. Without it, the
HTTP edge uses only the direct request TLS state and host; forwarded protocol
headers are not implicitly trusted. `webserver.trusted_proxies` affects client
IP attribution and realtime same-origin comparison, not public URL authority.

Chatto-streamed protected attachments are sequential full responses. They
advertise `Accept-Ranges: none` and ignore `Range`, returning `200` with the
complete object. NATS-backed video is therefore not seekable. Passive S3-backed
media redirects after authorization to a presigned object URL whose storage
backend provides byte-range delivery.

Processed videos can instead expose HLS. Six-second MPEG-TS segments make
seeking and adaptive rendition switching independent of byte-range support.
HLS child responses remain behind Chatto so membership loss revokes an already
issued playlist ticket on its next playlist or segment request.
