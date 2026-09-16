# Local OIDC conformance baseline

Run `mise test-conformance` from `authling/`. See the main
[development instructions](../../README.md#official-oidc-conformance-tests).
The task runs discovery and starts one interactive PKCE test. The full Basic
OP plan still needs browser interaction and screenshot evidence.

## Full Basic OP rerun after fixes: 2026-09-16

All 35 modules reached `FINISHED` in a new Basic OP plan with the same pinned
suite and static-client configuration. Discovery also passed. Browser flows
used Chrome DevTools, including a separate signed-out context for
`prompt=none`. Final results use the last completed attempt for each module.

| Basic OP result | Modules |
| --- | ---: |
| Passed | 19 |
| Warning | 4 |
| Review | 4 |
| Skipped by the suite | 8 |
| Failed | 0 |

The missing-response-type, `client_secret_post`, and S256 PKCE tests all passed.
The four warnings listed below remain. The synthetic account has no full name,
which explains the requested `name` warning; this does not verify general
support for requested essential claims.

Screenshot evidence was attached for all four review results. The login and
`max_age=1` flows displayed a fresh login form and completed reauthentication.
The unregistered-redirect and request-object-redirect flows displayed local
rejection pages. No evidence uploads remain outstanding, but the suite still
classifies these results as `REVIEW`, not `PASSED`.

Two browser-driver attempts were rerun: one revisited an earlier authorization
URL, and one started the next module before the screenshot test had finished.
The final attempts have no interrupted tests. Run modules one at a time, visit
each requested browser URL once, and wait for `FINISHED` before the next module.

This full rerun has no failing modules. It is not an all-pass or certification
result: warnings and review results remain, and skipped capabilities were not
verified. The initial results below are retained for comparison.

## Initial manual run: 2026-09-16

The official suite version was 5.2.4, revision 5e25853. The compose file pins
its image by digest. Tests used discovery, static clients, the code response
type, and the `openid` scope. Both configured confidential clients explicitly
set `require_pkce = false`. A separate client configuration tells the suite to
try the POST authentication method with the primary client.

All 35 modules in the Basic OP plan reached a terminal result across the final
attempts. Tests with browser-state or harness-configuration errors were rerun
with corrected setup. The discovery test also passed.

| Basic OP result | Modules |
| --- | ---: |
| Passed | 17 |
| Warning | 4 |
| Review | 4 |
| Skipped by the suite | 8 |
| Failed | 2 |

This is not a certification result. Earlier exploratory attempts remain in
the suite database. A certification submission needs a clean final run and
review of the warnings and screenshot evidence.

### Initial failures (resolved in the full rerun)

- `oidcc-response-type-missing`: Authling returned `unauthorized_client`.
  The test expects `invalid_request` or `unsupported_response_type`.
- `oidcc-server-client-secret-post`: discovery did not advertise
  `client_secret_post`. POST support was not exposed as a supported capability.

### Warnings

- `oidcc-server`: the ID Token includes `preferred_username` and `client_id`
  without those claims being requested.
- `oidcc-ensure-request-with-acr-values-succeeds`: the ID Token omits `acr`
  when the request includes preferred authentication-context values.
- `oidcc-codereuse-30seconds`: code reuse is rejected, but the original access
  token remains usable at UserInfo. The suite expects a 4xx response there.
- `oidcc-claims-essential`: UserInfo omits the requested `name` claim. The
  synthetic account had no full name configured.

### Review and skips

The `prompt=login`, `max_age=1`, unregistered redirect, and request-object
redirect tests finished with screenshot evidence and need human review.
Screenshots showed the reauthentication forms or local rejection pages.
The signed-out `prompt=none` test used a separate, empty browser context.

The suite skipped eight modules based on the configured capabilities:
profile, email, address, phone, combined scopes, the alternate happy flow,
unsigned request objects, and refresh tokens. These are not pass results.

The plain code flow now completes, and the valid S256 PKCE flow passes.
Public and CIMD clients still require S256 PKCE. The compatibility exception
is confined to configured confidential clients and does not disable verifier
validation when a request includes a challenge.

## Local data

`.authling/conformance/last-run.json` identifies the plans and tests created
by the latest task invocation. More tests run from the suite UI remain in its
local MongoDB data. The setup does not send test traffic to a hosted suite.
Only image downloads contact the external registries.

Do not publish raw suite exports or the state directory. They can include
synthetic account identifiers, client secrets, codes, and tokens. A result
summary should contain module names, outcomes, and redacted findings only.
