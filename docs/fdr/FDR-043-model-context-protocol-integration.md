# FDR-043: Model Context Protocol Integration

**Status:** Experimental
**Last reviewed:** 2026-08-30
**Implementation state:** Tester tool catalog implemented with OAuth, server
and account identity, room and message reads, posting, and room membership.

## Overview

Chatto's Model Context Protocol (MCP) integration lets an agent host inspect a
Chatto server through a standard, user-scoped tool interface. MCP tools adapt
the existing public Chatto operations. They do not replace the public API or
give an agent operator authority. The first catalog has bounded reads and a
small set of user actions for practical agent-host tests.

MCP has three main server primitives. Tools let a model request an operation.
Resources let a host load identified content. Prompts provide reusable
interaction templates. Chatto starts with tools because they provide bounded
arguments, normal authorization checks, and structured results for the first
use cases.

An MCP client runs inside an agent host. The client reads Chatto's tool
definitions, the host decides which calls it permits, and Chatto authorizes
each received call. The model's choice to request a tool never bypasses the
host or Chatto checks. In OAuth text, the MCP "resource" means the audience
identifier for the `/mcp` endpoint. It is different from an MCP Resource
primitive.

## Behavior

- The MCP integration is disabled by default while it is experimental.
- When enabled, Chatto serves MCP over stateless Streamable HTTP at `/mcp` on
  the existing public HTTP server. It serves the canonical `webserver.url`
  origin and each exact non-wildcard `webserver.allowed_origins` entry.
- The implementation prefers MCP `2026-07-28`. The SDK can negotiate its
  older supported versions during their compatibility window, but Chatto does
  not add a separate compatibility promise for them.
- A human grants an MCP client access through Chatto OAuth. The flow uses
  Authorization Code with PKCE and the client's CIMD identity.
- Human MCP access tokens are valid only for the exact MCP resource that the
  user approved. A token for one configured origin is not valid for another
  origin. The current grant has the `chatto:rooms:read`,
  `chatto:rooms:write`, `chatto:messages:read`, and
  `chatto:messages:write` scopes. These scopes are an additional ceiling on
  the user's normal Chatto authority. The consent screen states the room-list,
  room-membership, and message read/write capabilities before approval.
- Resource-bound access and refresh credentials use a separate credential
  class. A validator that supports only general bearer credentials rejects
  them instead of ignoring their resource and scope boundary.
- A bot can use its existing Bot API key. Calls act as that bot and remain
  limited by its explicit permissions and its owner's current authority.
- The MCP endpoint does not accept browser cookies. It does not accept a human
  bearer session that has no MCP resource and scope grant.
- The tool catalog contains `get_server_info`, `get_current_user`,
  `list_rooms`, `list_room_messages`, `post_message`, `join_room`, and
  `leave_room`.
- Identity tools return the effective server identity or the authenticated
  account identity. Server identity includes the canonical server URL and the
  connected MCP URL. Room and message list tools return bounded pages. The
  room-list result also returns the exact number of rooms that the account can
  see, independent of the page size.
  `post_message` creates one root text message. The room membership tools join
  or leave one channel room.
- MCP server metadata uses `Chatto` as its stable implementation name and the
  effective Chatto server name as its display title.
- Static MCP instructions identify the tools as the source of truth for rooms,
  messages, room membership, and account identity on the connected server.
  They tell an agent host to use the advertised server title to distinguish
  the connection from other Chatto connections, to call `get_server_info`
  when the target server is not clear, to complete pagination when needed,
  and not to infer Chatto application data from deployment configuration.
  Configured values never enter these instructions.
- Tool descriptions state that operations apply to the connected Chatto
  server. Identity and list descriptions also explain server matching, exact
  room counts, and continuation fields.
- Tool arguments use stable Chatto resource IDs and explicit bounded page
  limits. Tool results do not contain raw broker coordinates or internal
  storage identifiers.
- The endpoint requires a configured public server Host and rejects wildcard
  or unknown hosts. It rejects cross-origin browser writes, limits admission
  across all configured hosts to 20 requests per second with a burst of 40,
  and gives each request 15 seconds.
- A listed tool can reject a specific target even when the credential has the
  complete MCP grant.
- Every tool call applies current Chatto RBAC, room membership, message access,
  search visibility, and absence rules. A successful earlier call does not
  grant authority to a later call.
- A failed tool call returns an MCP error result. When Chatto can confirm that
  an RBAC permission is missing, the error names that permission and tells the
  agent to ask an administrator. Policy denials and hidden resources remain
  generic so the error does not disclose private state.
- A tool returns only the data needed for its declared operation. It does not
  add related private resources for agent convenience.
- User-controlled Chatto content is untrusted data. It cannot change tool
  descriptions, server instructions, authorization, or the available tool
  catalog.
- OAuth consent, session revocation, blocked-client policy, account lifecycle,
  and bot-key rotation affect MCP access in the same way that they affect the
  underlying credential.
- The network MCP server has no Operator API, bootstrap, password-reset,
  credential-enrolment, account-deletion, role-management, server-management,
  raw diagnostics, or event-log tools in its first release.
- Disabling MCP removes its routes from the public HTTP server. It does not
  disable ConnectRPC, realtime, OAuth, the CLI, or the local Operator API.

## Design Decisions

### 1. MCP adapts canonical Chatto operations

**Decision:** Implement each MCP tool as an adapter over the same application
operation used by the public API. Do not call Chatto through loopback HTTP and
do not put domain behavior in MCP handlers.
**Why:** ADR-042 and ADR-044 require transports to share authorization,
validation, consistency, and response behavior. A thin adapter prevents MCP
from becoming a second product API with different rules.
**Tradeoff:** MCP result shapes need explicit mapping, and not every public RPC
should become a tool.

### 2. Keep the tester catalog small

**Decision:** Expose identity, bounded room and message reads, root text
posting, and channel membership changes. Do not expose reactions, edits,
deletions, attachments, moderation, administration, resources, or prompts.
**Why:** This catalog is sufficient to test discovery, OAuth, schemas,
pagination, RBAC, membership, reads, and writes in a real agent host. It does
not create a second complete Chatto API.
**Tradeoff:** Message posting changes state and is not idempotent. An agent
must not retry it after an uncertain result. Read access can disclose
sensitive content and needs full authorization.

### 3. Use explicit human consent and existing bot identity

**Decision:** Human clients use OAuth with a resource-bound MCP scope. Bots use
their existing API key and explicit bot permission model. Ambient browser
cookies and unrelated human bearer sessions are not accepted.
**Why:** A user must know when an agent host receives access. Unattended agents
also need a non-human identity with least privilege. Chatto already provides
CIMD consent for humans and explicit permission allowlists for bots.
**Tradeoff:** MCP hosts must complete OAuth or let an operator configure a bot
credential. A normal signed-in browser session cannot silently become MCP
authority.

### 4. Keep MCP scopes separate from RBAC

**Decision:** MCP OAuth scopes limit the classes of MCP operations that a
human client can request. Normal Chatto permissions and resource visibility
still decide what the user can do and see.
**Why:** A user can grant an agent less authority than the user's complete
account. RBAC remains the canonical product authorization model.
**Tradeoff:** A call can fail because of either the OAuth ceiling or the
current product authority. Errors and consent text must make this distinction
clear without disclosing hidden resources.

### 5. Target the dated stateless MCP specification

**Decision:** Prefer MCP specification `2026-07-28` over stateless Streamable
HTTP. Use MCP's own discovery and capability rules after a client reaches the
endpoint. Let the official SDK negotiate its older supported versions during
their compatibility window. Do not describe the protocol as "MCP 2.0."
**Why:** The dated version is unambiguous. Its stateless request model fits the
existing public HTTP server and does not require MCP session affinity or a
shared MCP session store.
**Tradeoff:** An older agent host can connect only while the official SDK
supports its version. Chatto can make an older version an explicit product
contract only after a compatibility and resource cost review.

### 6. Use standard MCP and OAuth discovery

**Decision:** An operator gives the configured MCP URL to an agent host. The
host uses MCP and OAuth metadata for discovery. Do not add MCP details to
Chatto's ConnectRPC discovery service.
**Why:** A general MCP host does not use Chatto's protobuf API. MCP already
reports its protocol capabilities and tool catalog.
**Tradeoff:** Chatto cannot automatically suggest the MCP URL to a
Chatto-specific client until that product use case has a separate design.

### 7. Treat all Chatto content as untrusted data

**Decision:** Tool descriptions and protocol instructions are static Chatto
text. The configured server name is display metadata and a structured tool
result field. Other user-controlled content also stays in structured result
fields. No user-controlled value can add tools or change server instructions.
**Why:** Messages and profiles can contain text that tries to control an agent.
The server must not present that text as trusted protocol guidance.
**Tradeoff:** The MCP host and model must still handle prompt injection in
returned data. Server-side structure reduces confusion but cannot make hostile
content safe.

### 8. Preserve the local Operator API boundary

**Decision:** Do not expose `chatto.operator.v1` through the network MCP server.
A future operator MCP integration must be a separate local stdio bridge that
uses the configured Operator socket.
**Why:** The Operator API is root-equivalent and uses local filesystem access
as its security boundary. Combining it with user tools would create remote
root authority and make tool selection a critical privilege boundary.
**Tradeoff:** Remote operator automation cannot use the public MCP endpoint. A
trusted operator must use the CLI, the Unix socket, or a future local bridge.

### 9. Keep bootstrap outside MCP

**Decision:** Continue to bootstrap and recover Chatto through configuration
and the local Operator API. Do not add initial-owner or recovery tools to the
network MCP server.
**Why:** A normal user cannot approve OAuth access before the server has a
usable identity and recovery path. MCP transport does not solve this circular
authority problem.
**Tradeoff:** A remote-only deployment still needs an out-of-band bootstrap
procedure. A future one-time network bootstrap flow needs its own security
decision.

### 10. Treat each configured origin as a separate resource

**Decision:** Serve MCP on the canonical server origin and each exact public
alias. Bind human OAuth credentials to the exact origin that the client uses.
Keep the canonical server origin as the authorization-server issuer.
**Why:** Operators can expose one Chatto server through more than one public
host. Exact resource binding prevents a credential for one host from becoming
authority on a different host.
**Tradeoff:** A human must approve a new grant when the client changes to a
different configured origin. Wildcard CORS entries cannot expose MCP.

## Permissions

The MCP integration adds no RBAC permissions.

- `chatto:rooms:read`, `chatto:rooms:write`, `chatto:messages:read`, and
  `chatto:messages:write` are OAuth grant ceilings for a human MCP client.
  They are not RBAC permissions.
- `room.list` controls room-directory visibility where applicable.
- `room.join` controls channel joins where applicable.
- `message.post` controls root message posting.
- `message.read` controls broad message access in channel rooms.
- `message.read-interactions` controls relationship-scoped message access.
- DM membership continues to authorize DM reads.
- Bot calls also require the bot's explicit permission and sufficient current
  authority from its owner.

Each underlying operation can require other existing permissions or resource
relationships. The operation remains the source of truth.

## Related

- **ADRs:** ADR-024 (opaque bearer tokens), ADR-042 (protobuf-first public API),
  ADR-044 (ConnectRPC service conventions), ADR-045 (public API stability
  tiers), ADR-071 (CIMD-identified OAuth clients), ADR-079 (renewable bearer
  sessions), ADR-080 (message-read permission), ADR-085 (user-scoped MCP
  integration)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-023 (Authentication & Sessions),
  FDR-028 (Operator API & CLI), FDR-031 (Client–Server Compatibility
  Discovery), FDR-033 (Message Search), FDR-038 (Bot Accounts), FDR-039
  (Message Access & Interactions)

## Open Questions

- Which product scopes should cover later thread, search, and other write
  tools?
- When should Chatto add MCP resources or prompts instead of more tools?
- Do real agent hosts require support for an older MCP specification version?
- Should Chatto ship a separate local operator MCP bridge after the public
  endpoint has operational experience?
- Which audit events and safe usage metrics should identify MCP calls without
  recording message content or other PII?
