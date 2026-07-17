# Interface Inventory

Key files: [`cli/internal/connectapi/api.go`](../../cli/internal/connectapi/api.go),
[`cli/internal/http_server/connect.go`](../../cli/internal/http_server/connect.go),
[`cli/internal/http_server/realtime.go`](../../cli/internal/http_server/realtime.go),
[`proto/chatto/`](../../proto/chatto/)

This inventory records mounted transport and service boundaries. The generated
[ConnectRPC API reference](../../apps/docs-website/src/content/docs/reference/connectrpc-api/index.mdx)
is authoritative for individual RPCs, request and response messages, and public
method documentation.

Related decisions: [ADR-044](../adr/ADR-044-connectrpc-service-conventions.md) and
[ADR-045](../adr/ADR-045-public-api-stability-tiers.md).

## Transport boundaries

| Surface | Mount | Contract | Access boundary |
| ------- | ----- | -------- | --------------- |
| Public ConnectRPC | `/api/connect/chatto.{auth,discovery,api,admin}.v1.*` | Unary Connect, gRPC, and gRPC-Web services | Explicit per-service public or authenticated-user policy; method-level authorization remains inside operation models |
| Realtime WebSocket | `GET /api/realtime` | Binary `chatto.realtime.v1.Realtime*` frames | Bearer token in the hello frame or same-origin cookie; per-event authorization in `StreamMyEvents` |
| Operator ConnectRPC | `/api/connect/chatto.operator.v1.*` on the configured Unix socket | Root-equivalent local unary services | Unix-socket filesystem permissions; never mounted on the public listener |
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
| `chatto.api.v1` | `AssetService`, `AssetUploadService`, `MessageService`, `MyAccountService`, `NotificationPreferencesService`, `NotificationService`, `PushNotificationService`, `RoleService`, `RoomDirectoryService`, `RoomService`, `ServerService`, `ThreadService`, `UserService`, `ViewerService`, `VoiceCallService` | Authenticated user |
| `chatto.admin.v1` | `AdminDiagnosticsService`, `AdminEventLogService`, `AdminPermissionService`, `AdminRoleService`, `AdminRoomLayoutService`, `AdminServerService`, `AdminUserService` | Authenticated user; methods enforce administrative permissions |

## Mounted operator services

| Package | Service | Access policy |
| ------- | ------- | ------------- |
| `chatto.operator.v1` | `OperatorUserService` | Root-equivalent access over the private Unix socket |

`ServerDiscoveryService.GetServer` is the only Connect method for which the
bundled client enables side-effect-free GET. It also receives wildcard public
CORS and conditional-response caching. Other bundled-client Connect traffic
uses POST.

The discovery response includes the server software version, stable protocol
capability keys for mounted public packages and negotiated contracts, and an
optional minimum bundled-web-client version. The
`chatto.realtime.projection.v1` capability is the bundled 0.5 client's gate for
opening realtime protocol 2, the only accepted behavioral version. The
`chatto.realtime.v1` suffix is the protobuf namespace. This metadata is public
pre-authentication state.
It describes wire support, not enabled server features or the authenticated
viewer's permission-derived capabilities. Multi-server clients refresh it per
server and use version comparison only to classify older servers that omit
capability metadata.

Public URL generation prefers the configured `webserver.url`. Without it, the
HTTP edge uses only the direct request TLS state and host; forwarded protocol
headers are not implicitly trusted. `webserver.trusted_proxies` affects client
IP attribution and realtime same-origin comparison, not public URL authority.
