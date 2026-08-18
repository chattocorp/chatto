# Chatto Desktop

This directory packages the official Chatto SvelteKit frontend as an
experimental Electron application. Chatto Desktop has its own pre-1.0 version
and release-please component; tags use `chatto-desktop/vX.Y.Z`, independently of
the Chatto server version.

There is no desktop-specific frontend. Electron embeds the unchanged static
artifacts from `apps/frontend/build` and exposes them to its renderer at the
fixed, privileged origin `chatto://desktop`. The stable secure origin and
Electron's app-specific persistent session give local storage, IndexedDB,
service workers, registered servers, and delegated access tokens the same
durable namespace on every launch. Electron does not intercept ordinary HTTP
or HTTPS traffic, so requests to Chatto servers use Chromium's normal network
stack.

The official desktop origin is accepted by Chatto's OAuth redirect policy. The
desktop shell uses Electron's ordinary popup support for the same PKCE and
`BroadcastChannel` flow as a browser. Chatto 0.5 servers recognize the built-in
`chatto://desktop` client identity and exact callback; bearer-authenticated API
and realtime access requires no server origin configuration.

## Run it

From the repository root:

```sh
mise desktop-dev
```

The task builds the shared frontend and opens it in Electron. Electron stores
the default session beneath the platform's application-data directory for
`Chatto Desktop`; the shell writes no separate credentials or settings file.

## Verify and build

```sh
mise test-desktop
mise desktop-build
```

The build task first produces the frontend, then packages the host-platform
bundle beneath `apps/desktop/dist/`. macOS builds include the native
ScreenCaptureKit game-capture helper and its pinned LiveKit frameworks. CI
checks and packages macOS, Windows, and Linux bundles, verifies the nested macOS
helper, and validates platform signatures. Ordinary local and pull-request macOS
builds use ad-hoc signing; ordinary Windows and Linux builds remain unsigned.
Release builds use Developer ID signing and Apple notarisation on macOS and
Microsoft Artifact Signing on Windows. CI verifies every shipped Windows
executable and library against ChattoCorp's expected publisher identity and
requires an RFC 3161 timestamp before creating the release archive.

### Configure macOS release signing

An Apple Developer Program Account Holder must create a **Developer ID
Application** certificate for direct distribution. Export the certificate and
private key from Keychain Access as a password-protected `.p12` file. In App
Store Connect, create a team API key with App Manager access and download its
one-time `.p8` private key.

Base64-encode both files without line wrapping:

```sh
base64 -i chatto-developer-id.p12 | tr -d '\n'
base64 -i AuthKey_KEY_ID.p8 | tr -d '\n'
```

Add these secrets to the protected `desktop-signing` Actions environment, not
as repository-wide secrets:

| Secret | Value |
| --- | --- |
| `CHATTO_MACOS_CERTIFICATE_BASE64` | Base64-encoded `.p12` file |
| `CHATTO_MACOS_CERTIFICATE_PASSWORD` | Password used when exporting the `.p12` file |
| `CHATTO_MACOS_NOTARY_API_KEY_BASE64` | Base64-encoded App Store Connect `.p8` key |
| `CHATTO_MACOS_NOTARY_API_KEY_ID` | App Store Connect API key ID |
| `CHATTO_MACOS_NOTARY_API_ISSUER_ID` | App Store Connect team issuer ID |

The environment permits deployments only from `chatto-desktop/v*` tags and the
`main` branch used for manually dispatched verification builds. Its required
reviewer must approve a release before any matrix runner can access the signing
secrets. The desktop release workflow then fails closed when any secret is
absent. On its macOS runner it imports the certificate into a temporary
keychain, signs every nested executable in dependency order, submits the app to
Apple's notary service, and removes the temporary credentials even when the
build fails.

Every macOS release attempt uploads a `desktop-notarisation-log-*` artifact with
90-day retention. The log includes the complete Apple notary-service response
for successful submissions as well as failures, while the Electron notarisation
library redacts authentication arguments. Review accepted submissions for
warnings before publishing the resulting archive.

For a local Developer ID build, install the certificate in the login keychain
and set the signing identity and all three notarisation variables before running
`mise desktop-build`:

```sh
export CHATTO_MACOS_SIGN_IDENTITY='Developer ID Application: ChattoCorp GmbH (TEAMID)'
export CHATTO_MACOS_NOTARY_API_KEY="$PWD/AuthKey_KEY_ID.p8"
export CHATTO_MACOS_NOTARY_API_KEY_ID='KEY_ID'
export CHATTO_MACOS_NOTARY_API_ISSUER_ID='ISSUER_UUID'
mise desktop-build
```

### Configure Windows release signing

Windows releases use Microsoft Artifact Signing (formerly Trusted Signing) with
GitHub's short-lived OpenID Connect credentials. No code-signing private key is
exported to GitHub Actions.

An Azure administrator must complete the one-time service setup:

1. Create an Artifact Signing account and an organization-validated **Public
   Trust** certificate profile for ChattoCorp GmbH. Record the account endpoint,
   account name, profile name, and the complete certificate subject shown by the
   issued profile.
2. Create a Microsoft Entra application and service principal for the release
   workflow. Add a **GitHub Actions deploying Azure resources** federated
   credential for organization `chattocorp` (`261891647`), repository `chatto`
   (`1205013299`), and environment `desktop-windows-signing`, with audience
   `api://AzureADTokenExchange`. Azure generates the immutable-ID subject
   `repo:chattocorp@261891647/chatto@1205013299:environment:desktop-windows-signing`.
3. Grant that service principal only the **Artifact Signing Certificate Profile
   Signer** role, scoped to the Chatto Desktop signing account.
4. Create a protected `desktop-windows-signing` Actions environment and add the
   following secrets and variables to it. Keep the macOS credentials in the
   existing `desktop-signing` environment so the two platforms cannot access
   one another's signing credentials.

| Kind | Name | Value |
| --- | --- | --- |
| Secret | `CHATTO_WINDOWS_AZURE_CLIENT_ID` | Entra application (client) ID |
| Secret | `CHATTO_WINDOWS_AZURE_TENANT_ID` | Entra directory (tenant) ID |
| Secret | `CHATTO_WINDOWS_AZURE_SUBSCRIPTION_ID` | Azure subscription ID |
| Variable | `CHATTO_WINDOWS_SIGNING_ENDPOINT` | Regional account endpoint, including `https://` |
| Variable | `CHATTO_WINDOWS_SIGNING_ACCOUNT_NAME` | Artifact Signing account name |
| Variable | `CHATTO_WINDOWS_CERTIFICATE_PROFILE_NAME` | Public Trust certificate profile name |
| Variable | `CHATTO_WINDOWS_EXPECTED_PUBLISHER` | Complete certificate subject, exactly as Windows reports it |

Configure `desktop-windows-signing` to permit deployments only from
`chatto-desktop/v*` tags and the `main` branch used for manually dispatched
verification builds. Require a reviewer, who must approve a release before the
Windows runner can request an Azure token.
Enable prevention of self-review when another authorised reviewer is available;
do not enable it for a single-reviewer environment, because that would make
releases impossible. The release workflow fails before building when any
setting is missing. After building, it signs every `.exe`, `.dll`, and native
`.node` module recursively with SHA-256 and an RFC 3161 timestamp, then rejects
missing, invalid, untimestamped, or unexpectedly published signatures before
packaging the ZIP.

Before the first tagged release, manually run the `release` workflow with the
`desktop` target on `main`, approve the protected environment, and inspect the
Windows verification ZIP on a clean supported Windows installation. In File
Explorer, the Digital Signatures tab on `chatto-desktop.exe` must show the same
publisher configured in `CHATTO_WINDOWS_EXPECTED_PUBLISHER`; PowerShell's
`Get-AuthenticodeSignature` must report `Valid`.

Artifact Signing keeps the signing keys and short-lived leaf certificates in
Azure, so routine renewal does not require rotating a PFX secret. Renew the
organization validation and certificate profile before Azure reports expiry.
When replacing a profile, preserve the exact publisher subject, update only
`CHATTO_WINDOWS_CERTIFICATE_PROFILE_NAME`, and complete the manual verification
build before releasing. Do not change `CHATTO_WINDOWS_EXPECTED_PUBLISHER` unless
the publisher identity is deliberately migrating and the installer/update plan
has been reviewed.

For emergency revocation, first remove the Entra role assignment or federated
credential to stop new signatures, then revoke or disable the affected
certificate profile in Azure. Remove the GitHub environment values, review
Azure and GitHub audit logs, replace the Entra application if its identity was
compromised, and publish incident-specific user guidance before signing with a
replacement identity.

Electron handles camera, microphone, and notification permission requests only
for the fixed app origin. Screen sharing presents a native source picker.
Navigation outside the app is restricted to OAuth popup windows or opened in
the system browser; renderer Node.js integration is disabled and the renderer
is sandboxed.

## Prototype boundaries

This scaffold does not yet provide auto-update, OS deep links, installers, or
end-to-end desktop tests. Some identity providers
reject authentication inside embedded user agents, so a system-browser OAuth
handoff may still be required before treating this as a general release.
