# FDR-047: First-Run Setup

**Status:** Active
**Last reviewed:** 2026-10-10

## Summary

A new server opens a web setup wizard before normal registration. The installer
chooses a server name, an optional description, and a local owner account with
a username, display name, and password. Email delivery is not required. Existing
default rooms remain in place. After setup, the installer signs in normally.

## Decisions

### Terminal setup prepares the configuration

`chatto init --interactive` opens a fullscreen wizard for the listen port,
public URL, access policy, optional OpenID Connect SSO, embedded or external
NATS, search, local LiveKit, and optional SMTP settings. The output path comes from `--config`
or defaults to `chatto.toml`. An existing target file is rejected before the
wizard opens. Existing feature flags set the initial choices. The
welcome page explains the setup process before the first question. The
layout uses the terminal height, keeps navigation in a fixed footer, and
resizes the form viewport with the terminal. In a terminal smaller than
48 columns or 16 rows, setup shows resize guidance and pauses input.
The operator can use vertical arrows or Tab/Shift+Tab to move between questions,
and horizontal arrows to change choices. Each page has an explicit Continue
button. External NATS connection and authentication pages appear only when
external NATS is selected. Email pages appear only when SMTP is enabled.
The Server page groups the listen port and public URL, with guidance about
HTTPS reverse proxies. The Access page controls local registration, password
login, and SSO. Optional SSO pages collect the issuer URL, button label, client
credentials, and account-creation policy. They show the redirect URI to register
with the provider. SSO account creation is separate from local registration.
The final review lists the settings and files to create.
Moving down on the final question does not submit the form.
Cancellation creates no files. Existing files are never replaced.

The form masks SSO, SMTP, and NATS secrets and omits credentials from the review. It
makes no external connections. It explains that the search index contains
decrypted text, that call participants connect to LiveKit, and that the email
provider receives recipient addresses and email content. It also explains that
the SSO provider receives user IP addresses and sign-in requests, and that Chatto
receives provider identities.

The terminal flow writes private configuration files and shows startup
commands. Server naming and owner-account creation remain in the browser
wizard. First-run owner setup requires password login. If the operator selects
SSO-only login, the review and startup instructions explain how to temporarily
enable password login, create the owner, link SSO, and disable password login.
The form rejects a configuration with no login method. This keeps account creation in the existing atomic setup operation.
Plain `chatto init` remains suitable for scripts and does not ask questions.
The wizard does not edit existing files: the configuration serializer cannot
preserve their comments and settings outside the wizard's scope.

### Configuration checks are explicit

`chatto config check` checks TOML keys and Chatto environment names before normal
configuration decoding and validation. It includes indexed provider and bootstrap
fields. It rejects unknown names and deprecated aliases, with replacement
instructions. Normal startup retains its existing compatibility policy and
supported aliases. This lets operators find mistakes before deployment without
making an upgrade reject previously accepted files. The command makes no network
connections and reports no setting values. Value errors omit details because
parser and validation messages can contain secrets or personal data.

Email OTP uses the existing runtime default of 30 minutes in generated examples,
omitted settings, and ENV-only configuration. Keeping this value avoids changing
code lifetimes for existing deployments. Explicit values and ENV overrides still
apply. Generated setup choices, such as enabling embedded NATS and the asset
worker, remain explicit; they are not defaults for omitted settings.

### Setup is enabled by default

The first visitor can complete setup without a session or setup secret. The
operator must control access to a new deployment until setup is complete.
`core.skip_setup_wizard = true` suppresses the wizard for automated installations.
It does not change durable initialization state. The same value must be used on
all replicas. Direct password login must be enabled to create the local owner.

### Completion is permanent and atomic

The initial account, owner role, server settings, and completion record commit
together. Concurrent submissions cannot create two initial owners. A lost
response does not reopen setup: the installer can use normal sign-in. Invalid
input leaves setup available. The wizard keeps a failed draft in memory only.

### Existing servers remain closed

Only an empty event history can become eligible for setup. Existing servers
receive a durable completion record on upgrade. Deleting users, restarting,
restoring snapshots, or changing the opt-out flag does not reopen completed
setup. Operator-created user history also prevents first-run ownership claims.
Development bootstrap records completion after it creates accounts; it remains
unavailable in release builds.

### Setup belongs to the origin server

The wizard collects server details and the owner account in one form. Password
confirmation must match before submission. Account validation errors appear
below the affected field; connection and setup-state errors apply to the form. The page uses
the shared initial-load reveal: sections fade and scale in with a short stagger.
This runs once per full page load on all pages, respects reduced-motion settings,
and stops on keyboard or pointer input. Client-side navigation does not replay it.
The wizard uses the normal app header, frame, and server gutter. Selecting an
uninitialized origin server opens setup instead of sign-in. Users can still add
other servers and use their accounts on those servers. Setup does not block
client-wide navigation or require an account on the origin server.

### Keep the wizard small

SMTP, external login providers, branding, and room layout remain separate
configuration or management tasks. The initial account receives the owner role
because it must have full server control.

## Related records

- [FDR-001: Roles and Permissions](FDR-001-roles-and-permissions.md)
- [FDR-028: Operator API and CLI](FDR-028-operator-api-and-cli.md)
- [ADR-068: Selectable Event Mutation Consistency Boundaries](../adr/ADR-068-selectable-event-mutation-consistency-boundaries.md)
