# FDR-043: Model Context Protocol Integration

**Status:** Experimental
**Last reviewed:** 2026-08-29
**Implementation state:** Initial walking slice implemented with OAuth and
`list_rooms`.

## Overview

Chatto's Model Context Protocol (MCP) integration lets an agent host inspect a
Chatto server through a standard, user-scoped tool interface. MCP tools adapt
the existing public Chatto operations. They do not replace the public API or
give an agent operator authority. The first release is read-only so Chatto can
validate the security and compatibility model before it adds writes.

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
- On the first MCP rollout, every serving replica must be upgraded while MCP
  stays disabled. Enable MCP only after all replicas understand resource-bound
  grants. Do not roll one replica back to an older version after grants exist.
- When enabled, the `mcp` runtime unit serves MCP over stateless Streamable
  HTTP at `/mcp` on a separate listener. The canonical public URL can route
  through a reverse proxy.
- The implementation prefers MCP `2026-07-28`. The SDK can negotiate its
  older supported versions during their compatibility window, but Chatto does
  not add a separate compatibility promise for them.
- A human grants an MCP client access through Chatto OAuth. The flow uses
  Authorization Code with PKCE and the client's CIMD identity.
- Human MCP access tokens are valid only for the server's canonical MCP
  resource. The initial `chatto:rooms:read` scope is an additional ceiling on
  the user's normal Chatto authority.
- A bot can use its existing Bot API key. Calls act as that bot and remain
  limited by its explicit permissions and its owner's current authority.
- The MCP endpoint does not accept browser cookies. It does not accept a human
  bearer session that has no MCP resource and scope grant.
- The initial tool catalog contains `list_rooms`. It returns a bounded page of
  rooms visible to the caller.
- Tool arguments use stable Chatto resource IDs and explicit bounded page
  limits. Tool results do not contain raw broker coordinates or internal
  storage identifiers.
- The listener requires the Host from `[mcp].url`, rejects cross-origin browser
  writes, limits admission to 20 requests per second with a burst of 40, and
  gives each request 15 seconds.
- The server can omit a tool when the credential type or MCP scope cannot use
  that class of operation. A listed tool can still reject a specific target.
- Every tool call applies current Chatto RBAC, room membership, message access,
  search visibility, and absence rules. A successful earlier call does not
  grant authority to a later call.
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
- Disabling MCP removes its endpoint. It does not
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

### 2. Start with a small read-only catalog

**Decision:** The first walking slice exposes a bounded room-directory read.
It exposes no mutation tools, resources, or prompts. Later read tools must use
the same operation-adapter rule.
**Why:** A room list tests discovery, OAuth, tool schemas, pagination, and
normal visibility rules without returning message content. A small catalog
lets Chatto test host behavior before a model can read more sensitive content
or change state.
**Tradeoff:** Agents cannot post, react, update read state, moderate content,
or manage the server through the first release. Read access can still disclose
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
text. Tool results keep user-controlled content in structured result fields.
No user-controlled value can add tools or change server instructions.
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

## Permissions

The initial MCP integration adds no RBAC permissions.

- `chatto:rooms:read` is an OAuth grant ceiling for a human MCP client. It is
  not an RBAC permission.
- `room.list` controls room-directory visibility where applicable.
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

- Which low-risk mutation should be first: posting, replying, reacting, or
  updating read state?
- Which product scopes should cover later message, thread, search, and write
  tools?
- When should Chatto add MCP resources or prompts instead of more tools?
- Do real agent hosts require support for an older MCP specification version?
- Should Chatto ship a separate local operator MCP bridge after the public
  read-only endpoint has operational experience?
- Which audit events and safe usage metrics should identify MCP calls without
  recording message content or other PII?
