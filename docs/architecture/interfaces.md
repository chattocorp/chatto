# Interface Inventory

Key files: [`cli/internal/connectapi/api.go`](../../cli/internal/connectapi/api.go),
[`cli/internal/http_server/connect.go`](../../cli/internal/http_server/connect.go),
[`cli/internal/http_server/client_config.go`](../../cli/internal/http_server/client_config.go),
[`cli/internal/http_server/cimd.go`](../../cli/internal/http_server/cimd.go),
[`cli/internal/http_server/oauth.go`](../../cli/internal/http_server/oauth.go),
[`cli/internal/http_server/oidc.go`](../../cli/internal/http_server/oidc.go),
[`cli/internal/http_server/assets.go`](../../cli/internal/http_server/assets.go),
[`cli/internal/http_server/realtime.go`](../../cli/internal/http_server/realtime.go),
[`cli/internal/search/service.go`](../../cli/internal/search/service.go),
[`cli/internal/search/client.go`](../../cli/internal/search/client.go),
[`cli/internal/connectapi/message_search.go`](../../cli/internal/connectapi/message_search.go),
[`proto/chatto/`](../../proto/chatto/)

This inventory records mounted transport and service boundaries. The generated
[ConnectRPC API reference](../../apps/docs-website/src/content/docs/reference/connectrpc-api/index.mdx)
is authoritative for individual RPCs, request and response messages, and public
method documentation.

Related decisions: [ADR-044](../adr/ADR-044-connectrpc-service-conventions.md),
[ADR-045](../adr/ADR-045-public-api-stability-tiers.md), and
[ADR-053](../adr/ADR-053-versioned-nats-service-namespaces.md).

## Transport boundaries

| Surface | Mount | Contract | Access boundary |
| ------- | ----- | -------- | --------------- |
| Public ConnectRPC | `/api/connect/chatto.{auth,discovery,api,admin}.v1.*` | Unary Connect, gRPC, and gRPC-Web services | Explicit per-service public or authenticated-user policy; method-level authorization remains inside operation models |
| Realtime WebSocket | `GET /api/realtime` | Binary `chatto.realtime.v1.Realtime*` frames | Bearer token in the hello frame or same-origin cookie; per-event authorization in `StreamMyEvents` |
| Client bootstrap | `GET /client-config.json` | Versioned, non-secret selection of the Authling issuer and frontend CIMD client ID | Public, same-origin frontend configuration; always mounted and returned with `Cache-Control: no-store` |
| Server OIDC client metadata | `GET /oauth/client-metadata.json` | CIMD public-client identity and exact callbacks for Chatto server login | Public; mounted only when an OIDC provider uses this deployment's metadata URL as its client ID |
| Frontend OAuth client metadata | `GET /oauth/frontend-client-metadata.json` | CIMD public-client identity and exact SPA callbacks for connecting to Chatto servers and optional Authling account data | Public; always mounted, with the Authling callback included only when `frontend.authling_issuer` is configured |
| Chatto client authorization | `GET /oauth/authorize`, `POST /oauth/token` | Authorization Code with S256 PKCE for a client application connecting to a Chatto server; browser clients use a CIMD URL `client_id`, Desktop uses its built-in identity, and an optional `provider_id` hint can start one server-configured login provider | Public authorization start and CORS token exchange; the validated client identity and exact callback are bound through code exchange, and provider hints cannot supply an issuer or endpoint |
| Protected attachments | `GET /assets/files/{assetId}` and image transform variants | Per-user URLs use hourly issuance buckets with 23–24 hours of remaining validity; Chatto streams full responses, while passive S3-backed video, audio, and large files can redirect to short-lived presigned URLs | Signed `access` ticket, authenticated cookie, or bearer token; every request rechecks room membership before resolving storage or exposing binary bytes |
| Protected HLS video | `GET /assets/hls/{assetId}/master.m3u8`, rendition playlists, and segments | Master and media playlists are generated from the durable manifest; segments are complete bounded responses from NATS or S3 | Domain-separated source-video `access` ticket; every request rechecks room membership and every segment ID/role against the durable HLS manifest |
| Operator ConnectRPC | `/api/connect/chatto.operator.v1.*` on the configured Unix socket | Root-equivalent local unary services | Unix-socket filesystem permissions; never mounted on the public listener |
| Trusted NATS services | `svc.chatto.>` and `svc.chatto_ext.>` | Versioned protobuf request/reply through NATS micro services | NATS account permissions; extension providers receive only their configured service and upstream Core subjects |
| Reflection | `/api/connect/grpc.reflection.v1*` and `v1alpha*` | Public service descriptors | Public; restricted resolver excludes internal `chatto.core.v1` persistence types |

The public HTTP edge mounts every handler returned by `connectapi.API.Handlers`.
Authenticated services are wrapped with `connectrpc.com/authn` before protobuf
decoding and validation. `ExternalIdentityAuthService`,
`ServerDiscoveryService`, and reflection are public; all other public-listener
services require an authenticated user. The Operator API uses
`connectapi.API.OperatorHandlers` and is mounted only on the configured Unix
socket.

## Mounted public services

| Package | Public services | Auth policy |
| ------- | --------------- | ----------- |
| `chatto.auth.v1` | `ExternalIdentityAuthService` | Public capability-token flows |
| `chatto.discovery.v1` | `ServerDiscoveryService` | Public discovery |
| `chatto.api.v1` | `AssetService`, `AssetUploadService`, `MessageSearchService`, `MessageService`, `MyAccountService`, `NotificationPreferencesService`, `NotificationService`, `PushNotificationService`, `RoleService`, `RoomDirectoryService`, `RoomService`, `ServerService`, `ThreadService`, `UserService`, `ViewerService`, `VoiceCallService` | Authenticated user |
| `chatto.admin.v1` | `AdminDiagnosticsService`, `AdminEventLogService`, `AdminInviteLinkService`, `AdminPermissionService`, `AdminRoleService`, `AdminRoomLayoutService`, `AdminServerService`, `AdminUserService` | Authenticated user; methods enforce administrative permissions |

`AdminInviteLinkService` requires `user.invite`. Its resource includes the
full, deterministically reconstructed invite link so authorised operators can
copy it again; raw bearer tokens are not stored in `EVT`. Opening
`/invite/{token}` validates the compact capability, stores only the invitation
ID in the signed browser session, and immediately redirects to registration.

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

`ServerDiscoveryService.GetServer` is the only Connect method for which the
bundled client enables side-effect-free GET. It also receives wildcard public
CORS and conditional-response caching. Other bundled-client Connect traffic
uses POST.

The discovery response includes the server software version as public
pre-authentication state. The bundled client refreshes it per server and owns
an internal feature-to-minimum-server-version table for compatibility gates.
The 0.5 client requires the 0.5 server baseline before opening realtime
protocol 2, the only accepted behavioral version. The
`chatto.realtime.v1` suffix remains the protobuf namespace.

The bundled frontend loads `/client-config.json` from its own origin before it
offers Authling account-data synchronization. This client-owned bootstrap is
separate from Chatto server discovery. A standalone frontend can publish the
same schema without a Chatto origin server, and no connected remote server can
change the selected global identity provider.

Public server discovery includes each OIDC provider's issuer. The frontend uses
that field only to compare a server provider with its own trusted Authling
issuer. A match lets the client add the provider's server-local ID to the
Chatto authorization request. `/oauth/authorize` validates that the ID belongs
to a configured provider before it skips the regular server login screen.

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
