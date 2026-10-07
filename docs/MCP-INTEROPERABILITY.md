# MCP Interoperability Checks

**Last checked:** 2026-10-07
**Status:** Experimental; conformance is incomplete.

This record tracks [issue #2214](https://github.com/chattocorp/chatto/issues/2214).
Use it with [ADR-085](adr/ADR-085-agent-integration-through-mcp.md) and
[FDR-043](fdr/FDR-043-model-context-protocol-integration.md). The public
[MCP guide](../apps/docs-website/src/content/docs/guides/integrations/mcp.mdx)
contains client setup and security boundaries.

## Protocol and host matrix

Checks used a real local Chatto server, synthetic accounts, and a separate
test room on macOS. Browser sign-in and consent used Chrome DevTools MCP.
Codex calls used the app-server API without a model turn.

| Client | Version | Result |
| --- | --- | --- |
| Codex CLI | `0.160.1` | Protected-resource and issuer discovery, public CIMD, browser consent, S256 PKCE exchange, seven-tool discovery, and join/post/read/leave succeeded. Calls after a two-second access lifetime succeeded without new consent. |
| MCP Inspector CLI | `2.9.0`, modern protocol era | The same flow and tools succeeded with a locally served native CIMD document. Calls after a two-second access lifetime succeeded with `--stored-auth-only`. |
| Claude Code | `2.1.291` | Discovery produced the correct resource and scopes. Authorization stopped at callback validation. Its CIMD omits native application type and registers a portless localhost callback; the client requests a runtime port. No token was issued. |

Calls after the short access lifetime establish refresh behavior: an expired
access token alone cannot authenticate the endpoint. Inspector's older
`--use-stored-auth` flag returned `no_stored_token`; use `--stored-auth-only`.

**Compatibility decision:** Support stateless MCP `2026-07-28` for the
experimental release. The tested working clients do not need the older
`initialize` handshake. Do not relax CIMD callback validation for Claude Code.
Recheck each host version before release; this matrix is not a promise for
all host versions or deployment topologies. Public HTTPS and reverse-proxy
host checks still require a release-environment run.

## Repeat the official conformance check

Start an isolated development server from the repository root:

```sh
CHATTO_DEV_DATA_ROOT="$PWD/.context/mcp-check-data" mise dev
```

In another terminal, use the synthetic bootstrap bot credential:

```sh
CHATTO_MCP_URL=http://chatto.<workspace>.localhost:<port>/mcp \
CHATTO_MCP_TOKEN_FILE="$PWD/.context/mcp-check-data/bootstrap/test_bot.key" \
mise test-mcp-conformance
```

Use the URL printed by `mise dev`. For another test server, supply its exact
MCP URL and a credential file for a test account. Never use production
credentials or message content for this check. Stop `mise dev` after the run.

The task pins the official `@modelcontextprotocol/conformance` package to
`0.2.0-alpha.12` and uses its frozen `2026-07-28` requirement set. The stable
`0.1.16` package does not contain this requirement set. Results go to
`.context/mcp-conformance-results/`. Set `CHATTO_MCP_RESULTS_DIR` for a separate
run. Keep raw results private; review them before sharing.

The suite has no bearer-header option. A temporary loopback relay adds the
credential read from the file. Chatto still validates that credential. The
relay rejects browser-origin requests and spaces requests below Chatto's
admission limit. It closes when the suite ends. It does not test OAuth,
rate-limit recovery, or the target's Host/Origin protection. Direct Chatto
HTTP tests cover those host and origin boundaries.

For one scenario, set `CHATTO_MCP_SCENARIO`, for example:

```sh
CHATTO_MCP_URL=http://chatto.<workspace>.localhost:<port>/mcp \
CHATTO_MCP_TOKEN_FILE="$PWD/.context/mcp-check-data/bootstrap/test_bot.key" \
CHATTO_MCP_SCENARIO=tools-list mise test-mcp-conformance
```

### Current conformance results

- `tools-list` passes tool discovery, tool-name validation, deterministic
  order, and wire-schema checks.
- `caching` passes tool-list caching and wire-schema checks. Three checks fail
  because they call absent prompt/resource catalogs without checking the
  advertised capabilities first. The resource-read check is skipped.
- `server-stateless` passes 24 checks and skips two absent catalog-change
  capabilities. Four checks fail as **not testable** because Chatto does not
  expose `test_missing_capability`, `test_streaming_elicitation`, or
  `test_logging_tool`.
- The complete requirement set also expects SDK fixture tools, prompts,
  resources, and input-required workflows. It fails against Chatto's small
  tool catalog. Optional task-extension and pending checks are reported
  separately by the suite.

The initial run found a real capability mismatch: the Go SDK returned an
empty prompt list without advertising prompts. Chatto now rejects absent
resource, prompt, and completion methods with `404` / `-32601`. Its descriptor
advertises tools without logging or catalog-change subscriptions.

No expected-failure baseline hides these results. The task keeps its nonzero
exit status when a required check fails. A fixture-aware application profile
or an upstream suite change is still needed before conformance can pass.
Do not add diagnostic tools to Chatto's public catalog only to satisfy SDK
fixture checks.

## Authorization and privacy checks

`cli/internal/http_server/mcp_interop_test.go` uses two independent Chatto
cores and HTTP listeners with one NATS store. It verifies:

- A valid grant works on both replicas.
- A grant for an alias fails on the canonical resource.
- A grant with a missing scope fails with an OAuth resource challenge.
- Revocation rejects new MCP requests and refresh on both replicas.
- Client blocking rejects new MCP requests and refresh on both replicas.
- Expired access and invalid refresh credentials are rejected over HTTP.
- MCP request logs exclude credential, room-ID, and message-content canaries.

The existing core block-event test also verifies rejection on another replica
before best-effort token cleanup. Existing OAuth tests cover consent, PKCE,
resource binding, refresh rotation, and code exchange. The SDK's default
logger discards its diagnostics. HTTP metrics use fixed metric names and
bounded state/reason labels; MCP does not add request-content labels.

These checks do not replace a full deployment log and metric audit. Before
release, collect diagnostics from a synthetic complete host flow, including
failed OAuth requests, and search for identity, message, token, code, secret,
and query canaries. Include reverse-proxy access logs in that audit.

The local server debug logs from the complete host flows contained no message,
test login/email/password, access-token, authorization-code, or OAuth-query
canaries. HTTP request logging was checked separately by the request-log test.

## Release checklist

- [ ] Rerun the pinned suite and resolve or explicitly classify each required
  failure. Record unmeasured checks separately from passed checks.
- [ ] Repeat the host matrix against public HTTPS and the deployment's proxy.
  Verify the Claude Code blocker again or complete its flow after a host fix.
- [ ] Run cross-replica revocation and client-blocking HTTP tests.
- [ ] Complete the deployment log, diagnostic, and metric canary audit.
- [ ] Verify the public setup guide and record the exact host versions.
- [ ] Keep MCP marked as experimental in release notes and configuration.

Keep #2214 open until the remaining conformance and deployment checks are
complete.
