# MCP Interoperability Checks

**Last checked:** 2026-10-10
**Status:** Experimental; application checks do not establish full conformance.

This record tracks [issue #2214](https://github.com/chattocorp/chatto/issues/2214).
Use it with [ADR-085](adr/ADR-085-agent-integration-through-mcp.md) and
[FDR-043](fdr/FDR-043-model-context-protocol-integration.md). The public
[MCP guide](../apps/docs-website/src/content/docs/guides/integrations/mcp.mdx)
contains client setup and security boundaries.

## Protocol and host matrix

Checks on 2026-10-10 used an isolated local Chatto development server on Linux,
synthetic accounts, and the seeded general channel. Browser sign-in and consent
used Playwright. Codex calls used the app-server API without a model turn.

| Client | Version | Result |
| --- | --- | --- |
| Codex CLI | `0.162.0` | Protected-resource and issuer discovery, public CIMD, browser consent, S256 PKCE exchange, seven-tool discovery, and all seven tool calls succeeded. Calls after a two-second access lifetime succeeded without new consent. |
| MCP Inspector CLI | `2.9.0`, modern protocol era | The same flow and tools succeeded with a locally served native CIMD document. Calls after access expiry and a server restart succeeded with `--stored-auth-only`. |
| Claude Code | `2.1.295` | Discovery produced the resource and scopes, and the client requested S256 PKCE. Chatto rejected its localhost callback with `400 invalid_request` before consent. No token was issued. |

Inspector's default room-read grant exposed only three tools. The full-catalog
repeat set all four scopes in the session config's `oauth.scopes` string.
This test environment required `MCP_INSPECTOR_SECRET_STORE=file` and a private
`MCP_STORAGE_DIR` under `.context/` to retain credentials between CLI processes;
without persistent credentials, `--stored-auth-only` required new authorization.
These are synthetic test credentials, not a recommendation to move normal
host credentials out of the operating system's credential store.

Calls after the short access lifetime establish refresh behavior: an expired
access token alone cannot authenticate the endpoint. Inspector's older
`--use-stored-auth` flag returned `no_stored_token`; use `--stored-auth-only`.

**Compatibility decision:** Support stateless MCP `2026-07-28` for the
experimental release. The tested working clients do not need the older
`initialize` handshake. Do not relax CIMD callback validation for Claude Code.
Recheck each host version before release; this matrix is not a promise for
all host versions or deployment topologies. Public HTTPS and reverse-proxy
host checks still require a release-environment run.

## Repeat the application protocol checks

The default runs two scenarios from the pinned official suite:

- `tools-list`: tool discovery, name validation, deterministic order, and wire schemas.
- `http-header-validation`: request-header validation against the discovered catalog.

Both use Chatto's real tools. Header validation is pending and unscored in the
upstream frozen requirement set; it is still useful application coverage.
These checks do not establish full MCP conformance or verify OAuth.
On 2026-10-10, both selected scenarios passed: four catalog/schema checks and
14 header/schema checks. The default command exited with status 0; the explicit
full diagnostic run still exited with status 1.

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
`0.2.0-alpha.12` and runs the selected scenarios at `2026-07-28`. The stable
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

The full suite expects a purpose-built server with its test fixtures. The
[upstream integration guide](https://github.com/modelcontextprotocol/conformance/blob/main/SDK_INTEGRATION.md#example-server-pattern)
points to an everything-server, and the
[Go SDK runner configuration](https://github.com/modelcontextprotocol/conformance/blob/main/src/sdk-runner/known-sdks.ts)
builds `conformance/everything-server`. Selecting a protocol requirement set
does not adapt these fixtures to Chatto's catalog.

For a full-suite diagnostic run, add `CHATTO_MCP_FULL_SUITE=1` to the command
above. This selects the frozen `2026-07-28` requirement set and preserves its
failure exit status. Do not combine it with `CHATTO_MCP_SCENARIO`. Neither mode
uses an expected-failure baseline. A full-suite failure caused by absent test
fixtures is not evidence of a Chatto defect.

On 2026-10-10, the pinned suite ran against an isolated `mise dev` server
at commit `54e9c436887c4a10e5bbe3a58bb49fe2d3bbc0a1`, with a synthetic bot
credential. The required scenarios reported 70 successful checks, 35 failures,
three skipped checks, four warnings, and one informational result. The command
exited with status 1. The required failures have these causes:

| Cause                                            | Failed checks | Interpretation                                                                                                                                                      |
| ------------------------------------------------ | ------------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Absent fixture tools or input-required workflows |            21 | The suite cannot exercise the expected behavior with Chatto's catalog. These checks remain unmeasured.                                                              |
| Absent prompt, resource, or completion methods   |            13 | Chatto does not advertise these capabilities. The scenarios call them without a capability check, including three caching checks.                                   |
| Relay Host/Origin policy                         |             1 | The DNS-rebinding scenario sends an Origin header on its valid-localhost probe. The relay rejects all Origin headers with `403` before Chatto receives the request. |

The DNS-rebinding scenario's rejected-host success also measures the relay,
not Chatto. Neither result proves Chatto's Host/Origin behavior. Do not weaken
the relay's browser protection to make this scenario pass. Check that boundary
directly against Chatto.

The suite also ran 13 extension or pending scenarios that do not contribute to
the `2026-07-28` conformance result. They reported 28 successful checks,
36 failures, and one skipped check. Thus, the suite's combined summary of
98 successful checks and 71 failures includes unscored checks. Do not use that
summary as the required-profile result.

This failure review found no demonstrated Chatto defect. It does not establish
conformance: fixture-dependent checks remain unmeasured, and the required
suite still fails. Raw results are in the local, gitignored
`.context/mcp-verification/conformance/` directory; they are not release
artifacts. The development server was stopped after the run.

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

The full diagnostic run keeps its nonzero exit status when a required check
fails. The default checks establish only the selected application behavior;
they do not turn the full diagnostic result into a conformance pass.
Do not add diagnostic tools to Chatto's public catalog only to satisfy SDK
fixture checks.

## Authorization and privacy checks

### Catalog admission review

**Reviewed:** 2026-10-08. Apply the
[FDR-043 admission checklist](fdr/FDR-043-model-context-protocol-integration.md#tool-admission-policy)
with each catalog change and before a stable Chatto release that includes MCP.
API completeness and CRUD symmetry are not admission criteria.

The seven tools form one tester workflow: identify the server and account,
choose a visible room, join a channel when needed, read context, send a text
reply, and leave the channel. The local Codex and Inspector flows above are
host evidence for this workflow. They do not establish deployment conformance
or large-instance cost bounds.

| Tool                 | Workflow, effect, and output                                                                                              | Disposition                                                                                                 |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `get_server_info`    | Match the connected server to the user's task; return one server identity. No write.                                      | Pass.                                                                                                       |
| `get_current_user`   | Confirm the account that acts for the host; return that account's identity. No write.                                     | Pass.                                                                                                       |
| `list_rooms`         | Select a visible conversation; return at most 100 rooms and a continuation, with the exact visible count. No write.       | Existing tester exception for backend cost; see below.                                                      |
| `list_room_messages` | Read context in one joined room; return at most 100 messages and a continuation. No write.                                | Existing tester exception for backend cost; see below.                                                      |
| `post_message`       | Send one root text reply as the current account; body limit is 10000 bytes. Return one message.                           | Pass. Non-idempotent; never automatically repeat a completed or uncertain post.                             |
| `join_room`          | Join one visible channel as the current account; return one room. Changes membership.                                     | Pass. Idempotent membership intent; bounded retry must obey current scope, authority, and outcome guidance. |
| `leave_room`         | Leave one channel as the current account; return the room ID and result. Changes membership and access to future content. | Pass. Idempotent membership intent; bounded retry must obey current scope, authority, and outcome guidance. |

All tools require a verified user or bot credential. The scope and permission
rules in FDR-043 apply to every call; discovery is not target authorization.
The adapters use existing application identity, directory, timeline, message,
and membership behavior. Registration is an explicit seven-tool allowlist.
Results contain only each tool's declared fields. Resources, prompts, and
excluded operator, credential, diagnostic, and storage classes are absent.
The current schemas and structured error/outcome contracts have SDK and raw
HTTP coverage. These checks establish the current experimental contract, not
a general compatibility guarantee for every host.

**Existing cost exceptions:** `list_rooms` obtains the complete authorized
directory before it sorts and returns a page. Its work and memory grow with
the directory, even for a one-room page. `list_room_messages` hydrates a bounded
page, but finding visible entries can scan more history when the account has
only relationship-scoped read access. Page limits do not bound these scans.
Both tools retain their existing request deadline and output limits. The
exceptions retain the current read workflow for host tests; they do not claim
a measured work budget for large directories or sparse message visibility.
The MCP maintainer must measure these cases and resolve or explicitly renew
the exceptions before catalog growth or a stable Chatto release that includes
MCP.
They are not precedents for admitting another unbounded read.

Review sources: [tool registration and room paging](../cli/internal/mcpserver/handler.go),
[tool adapters](../cli/internal/mcpserver/tools.go),
[scope mapping](../cli/internal/mcpserver/scopes.go),
[directory reads](../cli/internal/core/room_directory_read_model.go), and
[timeline reads](../cli/internal/core/room_timeline_read_model.go).
Verification includes the
[SDK workflow and raw catalog tests](../cli/internal/mcpserver/handler_test.go),
[tool failure contracts](../cli/internal/mcpserver/errors_test.go),
[grant-subset checks](../cli/internal/mcpserver/scopes_test.go), and the host
matrix in this record.

`cli/internal/mcpserver/errors_test.go` checks every tool's argument-failure
contract through the official Go SDK and independent raw protocol requests.
It validates structured errors against the advertised output schemas and
checks the JSON text fallback. Canonical-operation cases cover membership,
confirmed missing RBAC permissions, generic policy denial, and identical
errors for absent rooms, hidden channels, and nonparticipant direct-message
rooms. Boundary fault fixtures check conflict, timeout, temporary failure,
rate-limit delay, and uncertain or completed message-post outcomes. These
fixtures do not simulate a real lost JetStream commit acknowledgement.
Scope-subset tests check HTTP scope challenges and the structured scope body.
The HTTP OAuth test still checks the official client's fresh-consent upgrade.
These automated checks do not add an independent production host to the matrix.

On 2026-10-08, Inspector CLI `2.9.0` accepted all seven success/error output
schemas on a real local server. With a synthetic bot key, a rejected post
returned `permission_denied`, `missingPermissions: ["message.post"]`,
`contact_administrator`, and `not_applied` as structured fields, plus the text
fallback. Inspector reported the tool failure with exit status `5`. This check
used an isolated development data directory and did not repeat browser OAuth.

`cli/internal/http_server/mcp_interop_test.go` uses two independent Chatto
cores and HTTP listeners with one NATS store. It verifies:

- A valid grant works on both replicas.
- A grant for an alias fails on the canonical resource.
- A grant with unknown scopes fails with an OAuth resource challenge.
- A single-scope grant refreshes on another replica without gaining scopes.
  Both replicas reject a call outside the refreshed grant with `403`.
- Revocation rejects new MCP requests and refresh on both replicas.
- Client blocking rejects new MCP requests and refresh on both replicas.
- Expired access and invalid refresh credentials are rejected over HTTP.
- Rejected MCP request logs exclude credential, room-ID, and message-content
  canaries. A successful identity call returns the private display name without
  logging it or the account's credential, login, or password canaries.

On 2026-10-10, `mise x -- go test -tags test_endpoints
./internal/http_server -run 'TestMCP' -timeout 180s` passed from `cli/`.
This includes the cross-replica, expired-credential, and request-log checks.
It is a targeted test run, not a full backend test run or a new host-flow audit.

The existing core block-event test also verifies rejection on another replica
before best-effort token cleanup. Existing OAuth tests cover consent, PKCE,
resource binding, refresh rotation, and code exchange. The SDK's default
logger discards its diagnostics. HTTP metrics use fixed metric names and
bounded state/reason labels; MCP does not add request-content labels.

Scope-subset checks on 2026-10-08 also cover all 15 nonempty grants through
OAuth consent, code exchange, refresh, filtered discovery, and successful
covered tool calls. Missing tool scope returns `403 insufficient_scope`
before domain work. False method/name headers cannot bypass the scope check.
The official Go client starts with room-read access and completes fresh
consent for message-write access after a `403` challenge. Its next tool list
contains only identity, room-list, and posting tools. The host matrix above
records earlier full-grant tests; it does not verify this scope upgrade in
Codex or Inspector. The public guide gives an explicit full-scope Codex login
command for hosts that need the complete catalog.

These checks do not replace a full deployment log and metric audit. Before
release, collect diagnostics from a synthetic complete host flow, including
failed OAuth requests, and search for identity, message, token, code, secret,
and query canaries. Include reverse-proxy access logs in that audit.

The local server debug logs from the complete host flows contained no message,
test login/email/password, access-token, authorization-code, or OAuth-query
canaries. HTTP request logging was checked separately by the request-log test.

The 2026-10-10 local repeat checked server debug logs, Codex diagnostics, and a
metrics scrape after successful host flows and rejected OAuth requests. None
contained the test message, login/display-name, password, credential,
authorization-code, secret, or complete-query canaries. Inspector tool responses
were stored separately as private test output. This check did not include a
deployment's reverse-proxy logs or establish that every possible private value
is absent from diagnostics.

## Release checklist

- [ ] Run the selected application protocol checks. Record the scenario names,
      suite version, and results without claiming full conformance.
- [ ] Repeat the host matrix against public HTTPS and the deployment's proxy.
      Verify the Claude Code blocker again or complete its flow after a host fix.
- [ ] Run cross-replica revocation and client-blocking HTTP tests.
- [ ] Complete the deployment log, diagnostic, and metric canary audit.
- [ ] Verify the public setup guide and record the exact host versions.
- [ ] Keep MCP marked as experimental in release notes and configuration.

Keep #2214 open until the remaining conformance and deployment checks are
complete.
