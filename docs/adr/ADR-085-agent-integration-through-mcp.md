# ADR-085: Provide User-Scoped Agent Integration through MCP

**Date:** 2026-08-29

**Status:** Accepted

## Context

Chatto has a protobuf-first public API for clients, integrations, bots, and
administration. It also has a separate local Operator API for bootstrap and
recovery. Agents can use the public API, but each agent host must currently
learn Chatto's service contracts and build a custom adapter.

The [Model Context Protocol (MCP)](https://modelcontextprotocol.io/specification/2026-07-28/architecture)
is an open protocol for agent hosts to discover and call tools and to read
resources. MCP can make Chatto available to general agent hosts without making
the MCP contract the primary Chatto product API.

MCP does not supply an application authorization model. A network MCP server
must still authenticate a caller, limit its authority, authorize each target,
and treat returned content as untrusted. These requirements are important for
Chatto because messages can contain private data and hostile instructions.

The term "MCP 2.0" is ambiguous. MCP specification versions use dates. SDKs
use their own release numbers. The current specification version is
`2026-07-28`; it uses stateless HTTP requests and replaces the earlier
initialization session with per-request metadata and `server/discover`.

Chatto must also keep two existing authority boundaries:

- Public and Admin APIs act as an authenticated user and apply normal RBAC.
- The Operator API acts as the system actor and treats access to its Unix
  socket as root-equivalent authority.

Putting both boundaries behind one network MCP endpoint would make tool
discovery hide a critical security difference. It would also create a remote
root credential and a second operator authentication model.

## Decision

Chatto will provide an experimental, user-scoped MCP server for agent
integrations. MCP is an adapter over Chatto application operations. It is not
the canonical product API, a new domain layer, or a replacement for
ConnectRPC.

The first implementation will use these boundaries:

1. Chatto mounts stateless Streamable HTTP at `/mcp` on its public HTTP server
   only when an operator sets `[mcp].enabled` to `true`. The MCP endpoint uses
   the origin from `webserver.url` and each exact non-wildcard origin from
   `webserver.allowed_origins`. It does not own a separate listener, lifecycle,
   or URL setting.
2. The preferred MCP specification version is `2026-07-28`. Chatto does not
   call this version "MCP 2.0." The SDK can negotiate its older supported
   versions during their compatibility window, but Chatto does not add a
   separate compatibility promise for them.
3. The initial catalog stays small. It has server and account identity,
   bounded room and message reads, root text posting, and channel membership
   changes. Later tools can adapt other existing public operations after this
   transport and authorization path has operational evidence.
4. Human clients use Chatto's OAuth Authorization Code flow with PKCE and CIMD
   client identity. Each configured origin is a separate MCP resource. MCP
   access tokens are bound to the exact resource and to explicit MCP scopes.
5. Bot API keys can authenticate the MCP endpoint as their bot account. The
   bot's explicit permission allowlist and owner ceiling continue to apply.
6. Each MCP call uses the same authenticated application operation as the
   equivalent public API call. The operation enforces RBAC, membership,
   message-access rules, validation, pagination, and resource visibility.
7. MCP tool discovery can reduce the visible catalog for the credential, but
   the catalog is not an authorization boundary. Every call checks authority
   again.
8. Tool output uses canonical public resource shapes or a deliberate bounded
   projection of them. It does not expose persisted event payloads, NATS
   subjects, JetStream positions, internal cursors, or projection internals.
9. Chatto treats room names, profiles, messages, attachments, and other
   user-controlled values as untrusted data. Tool descriptions and server
   instructions do not include text from those values. Structured results
   keep data separate from protocol instructions.

MCP authorization adds a ceiling above Chatto authorization. A human call must
have the required MCP OAuth scope and the normal Chatto authority for the
operation and target. OAuth scopes do not replace RBAC permissions. A bot call
uses the bot's existing explicit authority model and does not gain human OAuth
authority.

The MCP endpoint does not accept ambient browser cookies or a normal
first-party bearer session that was not issued for the MCP resource. This rule
prevents a general Chatto login from becoming agent authority without an
explicit grant.

The MCP server remains separate from `chatto.operator.v1`. Chatto will not
mount root-equivalent MCP tools on the public listener. A future local operator
MCP bridge can run as a separate stdio process and call the existing Operator
API through its Unix socket. That bridge must have a separate tool catalog and
must preserve the socket access boundary.

MCP does not replace bootstrap. Bootstrap must work before a normal user can
complete OAuth consent. Chatto will continue to use configuration and the local
Operator API for bootstrap and recovery. Any future network bootstrap flow
needs a separate decision for one-time authority, expiry, replay protection,
audit behavior, and shutdown after use.

The first MCP implementation uses the official Tier 1 Go SDK. Chatto does not
copy MCP protocol types into protobuf definitions or create a custom transport
binding.

An operator gives one configured MCP URL to an agent host. The host uses MCP
and OAuth metadata to discover protocol and authorization details. The
authorization server keeps the canonical `webserver.url` issuer for every MCP
resource. Chatto's ConnectRPC discovery service does not advertise MCP because
a general MCP host does not use that Chatto-specific service.

MCP tool names, argument schemas, result schemas, error behavior, and
authorization meaning are public integration contracts. Chatto will evolve
them additively where practical. A tool contract can change independently from
the protobuf package version, but the same compatibility review and release
guidance apply.

## Consequences

General agent hosts can use Chatto through a standard protocol. They do not
need a Chatto-specific ConnectRPC adapter for the supported tasks.

ConnectRPC remains the complete and typed integration contract. MCP tools can
combine several public operations into one bounded agent task without moving
domain rules into the transport.

The experimental endpoint expands the public HTTP surface and adds a protocol
dependency. It does not add another listener or process lifecycle. The
implementation must add rate limits, request-size limits, bounded pagination,
timeouts, cancellation, audit attribution, OAuth protected-resource metadata,
resource indicators, token audience checks, and scope handling. Adding the SDK
also requires the normal dependency and `NOTICE` review.

The initial write catalog is limited to root text posting and channel
membership. It does not include content changes, deletion, moderation,
administration, or operator authority.

Message content stays sensitive even when a tool is read-only. Operators and
users must still make an explicit grant, and Chatto must apply current message
access rules to every result.

The Operator API remains local and root-equivalent. Remote automation must use
a normal user or bot identity, or deliberately run trusted tooling where it can
access the Operator socket.

This change adds an integration endpoint but does not change the public
ConnectRPC discovery schema. An MCP client uses the server's public origin with
the `/mcp` path. The missing feature does not affect ConnectRPC, realtime,
browser, desktop, or bot API behavior.
