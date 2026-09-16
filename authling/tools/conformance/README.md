# Local OIDC conformance baseline

Run `mise test-conformance` from `authling/`. See the main
[development instructions](../../README.md#official-oidc-conformance-tests).
The task runs discovery and starts one interactive PKCE test. The full Basic
OP plan still needs browser interaction and screenshot evidence.

## Manual run: 2026-09-16

The official suite version was 5.2.4, revision 5e25853. The compose file pins
its image by digest. Tests used discovery, static clients, the code response
type, and the `openid` scope. Both configured confidential clients explicitly
set `require_pkce = false`. A separate client configuration tells the suite to
try the POST authentication method with the primary client, so it can report
that Authling does not support this method.

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

### Failures

- `oidcc-response-type-missing`: Authling returns `unauthorized_client`.
  The test expects `invalid_request` or `unsupported_response_type`.
- `oidcc-server-client-secret-post`: discovery does not advertise
  `client_secret_post`. Authling currently accepts only `client_secret_basic`
  for configured confidential clients.

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
