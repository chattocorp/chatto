# Chatto Desktop

This directory packages the official Chatto SvelteKit frontend as an
experimental Electron application. Chatto Desktop, the server, and the frontend
share one version and release commit. Tags use `vX.Y.Z`; desktop archives and
checksums are attached to the same Chatto GitHub release. Tagged builds display
that exact version in the frontend. Manual verification builds display the
development version and commit SHA instead.

There is no desktop-specific frontend. Electron embeds the unchanged static
artifacts from `apps/frontend/build` and exposes them to its renderer at the
fixed, privileged origin `chatto://desktop`. The stable secure origin and
Electron's app-specific persistent session give local storage, IndexedDB,
service workers, registered servers, and delegated access tokens the same
durable namespace on every launch. Electron does not intercept ordinary HTTP
or HTTPS traffic, so requests to Chatto servers use Chromium's normal network
stack.

Desktop shows a password form inside the selected server's view. Its narrow
preload capability submits the password to that server's `/auth/login`
endpoint and installs the returned renewable bearer session. It requires
HTTPS except for loopback development servers and does not follow redirects.
These are programmatic password sessions, without OAuth client grants. The
web frontend continues to use OAuth for remote login.

Configured SSO buttons appear above the password form. Each button starts
Chatto OAuth with the selected provider ID; the Chatto server validates that
ID and starts its configured provider flow. Older servers can show their
normal login page instead. Password-disabled servers show the provider
buttons without a password form.

**Create account** opens server-hosted registration when direct registration
is enabled. Email-code verification and account creation stay in the
authorization window. After consent, Desktop receives the new session and
opens the server. Older servers can show their login page first.

**Continue in the browser** opens the server's page for external providers,
registration, and password recovery. The official desktop origin is accepted
by Chatto's OAuth redirect policy. The desktop shell uses Electron's ordinary
popup support for the same PKCE and
`BroadcastChannel` flow as a browser. Chatto 0.5 servers recognize the built-in
`chatto://desktop` client identity and exact callback; bearer-authenticated API
and realtime access requires no server origin configuration.

## Notifications and background operation

On macOS, closing the main window hides it and keeps Chatto running. Select
Chatto in the Dock to open the window again. **Command-Q** quits the app and
stops its connections and new alerts. On Windows and Linux, closing the main
window quits the app. Chatto adds no menu bar or system tray icon.

In notification settings, enable **Push notifications on this device** to allow
native desktop alerts. This choice applies to every signed-in server and starts
off. Use **Send test notification** to check OS delivery. macOS notification
tests require a signed application. OS notification settings can block alerts.

Native alerts use the existing notification resource reads. The selected server
keeps its normal live connection; inactive servers keep their normal polling
schedule. Desktop alerts do not open additional connections or add polling.
An inactive server's alert can therefore arrive at the next poll.

Only new Important notifications that still need attention produce alerts.
Do Not Disturb suppresses alerts. Initial history and older pages do not produce
alerts. Alerts contain generic text, without message previews. Click an alert
to open its conversation and highlight its message. Reading or removing an
occurrence, signing out, or losing access clears its retained alert when the
client observes that change. Fully quitting stops new alerts. Delivery uses no
external push provider; browser Web Push is separate.

## Run it

From the repository root:

```sh
mise desktop-dev
```

The task builds the shared frontend and opens it in Electron. Electron stores
the default session beneath the platform's application-data directory for
`Chatto Desktop`. The shell stores update preferences separately in
`updates.json` within this profile. This file contains no account credentials.

## Verify and build

### macOS installation

Download `chatto-desktop_<version>_macOS_ARM64.dmg` from the Chatto release.
Open the disk image and drag **Chatto** to **Applications**. Eject the
disk image, then open the app from Applications. Release disk images and apps
are Developer ID signed and notarised. The ZIP download remains available.

Use the **Updates** menu to check, select Stable or Beta, and restart to install.
Registrations, sessions, and preferences remain in the existing profile.
Older builds without this menu need one manual replacement. Moving the app to the Trash does not erase that
profile or sign out registered servers.

### Desktop updates

Tagged macOS apps, Windows NSIS installations, and Linux AppImages use
`electron-updater` with the public `chattocorp/chatto` GitHub releases. Windows
ZIP and Linux tarball installations remain manual. Local builds and manual CI
verification builds do not check for updates.

Prerelease builds default to Beta; stable builds default to Stable. A saved
channel choice takes precedence. Beta includes beta and subsequent stable
releases. Switching to Stable does not downgrade an installed beta. Automatic
checks start 30 seconds after launch and repeat every 30 minutes. Stable
updates roll out over 36 hours; manual checks and Beta bypass this delay.
Users can disable automatic checks and downloads in the Updates menu.
The restart action appears only when an update is ready and shows its version.
Installation requires **Restart to Install [version]** and confirmation. An ordinary quit
does not install an update. The frontend stays bundled and startup uses local files.

GitHub receives the user's IP address and update requests. The updater sends
no Chatto account or connected-server information. The **About Updates** menu
explains this connection. Downloads use full archives with SHA-512 checksums;
macOS and Windows also verify application signatures. Linux AppImage updates
trust HTTPS and the GitHub release manifest, without a separate publisher signature.

The release workflow collects all three update artifacts before it generates
the channel manifests. It uploads these to the existing draft release. Only
`finalize-release`, after both server and desktop jobs succeed, makes the release
public. Signing or packaging failures keep the release in draft. Rerun failed
jobs against the same tag before publication. Do not replace published update
artifacts; ship a new version instead.

Before the first updater release, verify an installed signed build updating
to a newer signed build on macOS and Windows, and an AppImage updating on Linux.
Check stable/beta selection, signature rejection, failed downloads, retained
sessions, and restart during a call. Existing installations need one manual
upgrade to obtain the updater.

After `mise desktop-build` on macOS, create a local disk image with:

```sh
mise x -- pnpm --filter chatto-desktop build:macos-dmg
bash tools/test-desktop-macos-dmg.sh apps/desktop/dist/dmg/*.dmg
```

The output is in `apps/desktop/dist/dmg/`. Local and PR disk images are unsigned.
The release workflow adds `--release` to sign, notarise, staple, and validate
the disk image with the existing macOS release credentials. It preserves the
app's existing signature and notarisation ticket. CI mounts the image, copies
the app to a temporary folder, and checks the copied signature and native helper.

Before release, test a signed DMG downloaded through a browser on a clean Mac
with Gatekeeper enabled. Check installation, launch, login, microphone, camera,
and screen sharing. Also replace an existing installation and confirm that
registrations and sessions remain available. The automated copy check does not
replace these interactive checks.

### Windows installation

Download `chatto-desktop_<version>_Windows_X64_Setup.exe` from the Chatto release
and run it. The installer installs for the current Windows user without
administrator rights and adds a **Chatto** Start menu shortcut.
Use **Updates → Release Channel** to select Stable or Beta, and the **Updates**
menu to check for updates and restart to install them. If an older installation
has no Updates menu, close Chatto and run a newer installer once.
ZIP copies do not support automatic updates.

For managed deployment, use `chatto-desktop_<version>_Windows_X64_Setup.msi`.
This package installs for all users and requires administrator rights. To install
it silently, run `msiexec /i "path\to\Chatto_Setup.msi" /qn /norestart` from an
elevated terminal. The MSI does not start Chatto after installation and does not
use the application's automatic updater. Deploy newer MSI packages through your
management tools, or run them manually with Chatto closed.

Remove an existing EXE installation before switching to MSI, or remove MSI
before switching to EXE. Do not install both formats on the same machine.
Removal preserves the profile. MSI uses the three-part release version for
upgrade comparisons: beta builds and the stable build with the same base
version can replace each other. Deployment tools must select the intended
release; MSI does not enforce beta ordering within that base version.

The installer uses the same profile as the ZIP application. Existing server
registrations and sessions remain available. Quit the ZIP application before
using the installed application; the installer does not remove old extracted
folders. The ZIP download remains available.

Remove the installed application through Windows Settings. Uninstall preserves
the local profile, including sessions and preferences. It does not sign out
registered servers or erase local credentials.

### Build commands

```sh
mise test-desktop
mise desktop-build
```

To build and open a local macOS app with its own screen-recording permission, use:

```sh
mise desktop-run-dev
```

`mise desktop-build-dev` builds the app without opening it. To open an existing
build, run `open "apps/desktop/dist/Chatto Dev.app"` from the repository
root. Quit a running dev app before rebuilding it to test the new binary.

This creates **Chatto Dev** (`run.chatto.desktop.dev`) with a separate
capture-helper identity and app-data directory. Sign in and grant permission
to the dev app once. The official app keeps `run.chatto.desktop`; switching
between these installs does not replace its permission identity.

An ad-hoc signature changes when the binary changes, so macOS can still ask
again after a dev rebuild. Use the same installed code-signing certificate
for each build to keep its permission identity stable. List valid identities
with `security find-identity -v -p codesigning`. An Apple Development certificate
is sufficient for local builds; notarisation is not required.

Create `mise.local.toml` at the repository root with the exact certificate name:

```toml
[tasks.desktop-build-dev.env]
CHATTO_MACOS_SIGN_IDENTITY = "Apple Development: Your Name (IDENTIFIER)"
```

This file is gitignored. Both `mise desktop-build-dev` and
`mise desktop-run-dev` now use that identity. The setting applies only to the
development build task. After you switch from ad-hoc signing, macOS can require
one more permission grant and app restart. Later builds should retain that
grant while the signing identity stays the same. Quit the dev app before
rebuilding it. Do not reset the official app's permissions to repair a dev build.

Local development builds skip online signature timestamps to reduce build time.
Certificate signing stays enabled. Release builds and builds configured for
notarisation retain online timestamps. The build prints when signing starts
and reports the time when signing and packaging finish. A signing failure stops
the build.

### Screen-share diagnostics and codec selection

Native macOS screen sharing defaults to tuned H.264 in development and release
builds. It sends one layer, prioritizes resolution, and uses hardware VBR on
macOS 26 or later. The target is at most 6 Mbps and the video maximum is
8 Mbps; both follow WebRTC's network allowance. Older macOS versions and Macs
that reject VBR setup use tuned average-rate H.264 at 90% of that allowance.
Audio and transport overhead are additional. Receivers must support H.264
Constrained High profile at level 4.2 or above.

The **Diagnostics** menu retains explicit average-rate and VBR comparisons,
stock H.264, and HEVC. Select a mode, then stop and start sharing. Select
**Default: Tuned H.264 (Automatic VBR) for Next Share** to restore the default.
Explicit VBR reports an error if unsupported. The stock H.264 and HEVC options
use 8/4/1 Mbps simulcast limits. HEVC needs receiver and LiveKit server support;
an explicit server rejection retries H.264. No mode publishes two codecs at once.
If a viewer cannot receive HEVC, return to H.264 and restart sharing.

To test resolution priority, select **Test H.264 Single Layer (8 Mbps,
Resolution First) for Next Share**, then restart sharing. This test publishes
one video layer with the same capture settings and frame-rate limit. It prefers
to preserve resolution and lets frame rate decrease under pressure. Its video
ceiling is 8 Mbps; audio and transport overhead are additional. It does not force a
constant bitrate. All viewers receive the same encoded resolution. Select
either regular codec option to restore simulcast and frame-rate priority on the
next share. Diagnostic samples include `maintainsResolution` to identify the
adaptation preference.

To report a problem, share a moving scene for at least 15 seconds, then select
**Diagnostics → Copy Screen Sharing Diagnostics**. Paste the report into the
issue or conversation. Reports contain the last 60 lifecycle and performance
records from this app process. Native publisher samples arrive every second
for the first 20 seconds, then every five seconds. They also appear on the
terminal when the app is launched from one.

Each sample reports negotiated codec/profile, encoder category, capture and
encoded dimensions, fps, target and measured bitrate, mean QP, encoding time,
quality-limit reason, packet-loss counters, retransmissions, and round-trip
delay when the SDK supplies them. Missing values mean unavailable; QP values
must be compared within the same codec. The first sample has no measured rate
because it establishes the counter baseline. Reports cover the native
publisher, not the viewer's decoder or the browser screen-share path.

Each record identifies whether simulcast is enabled and lists its configured
video layer ceilings in `limitsBps`. `availableOutgoingBps`, when available,
is WebRTC's bandwidth estimate for the shared publisher transport, including
audio. It is not a separate budget for each layer or a speed-test result.

Reports stay in memory and contain no account or source names, participant or
track IDs, server addresses, IP addresses, tokens, or encryption keys. Copying
puts the report on the clipboard; the app does not send it to a service.

The build task first produces the frontend, then packages the host-platform
bundle beneath `apps/desktop/dist/`. macOS builds include the native
ScreenCaptureKit game-capture helper and its pinned LiveKit frameworks. CI
checks and packages macOS, Windows, and Linux bundles, verifies the nested macOS
helper, and validates platform signatures. Ordinary local and pull-request macOS
builds use ad-hoc signing; ordinary Windows and Linux builds remain unsigned.
Release builds use Developer ID signing and Apple notarisation on macOS and
Microsoft Artifact Signing on Windows. CI verifies every shipped Windows
executable and library has a valid RFC 3161-timestamped signature, and verifies
`chatto-desktop.exe` against ChattoCorp's expected publisher identity before
creating the release archive.

On Windows x64, after `mise desktop-build`, build the installer with:

```sh
mise x -- pnpm --filter chatto-desktop build:windows-installer
```

The output is in `apps/desktop/dist/installer/`. electron-builder wraps the
existing bundle in a per-user NSIS installer and a per-machine MSI package.
Both release installers use the Windows signing hook. The application ID
`run.chatto.desktop` is the persistent upgrade identity; do not change it between
releases. CI tests silent installation, reinstall, shortcut and uninstall
registration, removal, and profile preservation on a disposable Windows runner.
PR installer artifacts are explicitly labelled unsigned.
The MSI `upgradeCode` in `scripts/windows-installer-config.mjs` is also a
persistent identity; do not change it. CI tests MSI installation, machine-wide
registration, shortcuts, upgrade from a synthetic older package, repair,
removal, and profile preservation. The unsigned upgrade fixture is not published.
Before release, also test an upgrade from an older signed MSI on Windows.

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

| Secret                               | Value                                        |
| ------------------------------------ | -------------------------------------------- |
| `CHATTO_MACOS_CERTIFICATE_BASE64`    | Base64-encoded `.p12` file                   |
| `CHATTO_MACOS_CERTIFICATE_PASSWORD`  | Password used when exporting the `.p12` file |
| `CHATTO_MACOS_NOTARY_API_KEY_BASE64` | Base64-encoded App Store Connect `.p8` key   |
| `CHATTO_MACOS_NOTARY_API_KEY_ID`     | App Store Connect API key ID                 |
| `CHATTO_MACOS_NOTARY_API_ISSUER_ID`  | App Store Connect team issuer ID             |

The environment permits deployments only from `v*` tags and the
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
2. In the GitHub repository's Actions OIDC settings, enable immutable OIDC
   subject claims. Do this before creating the Azure federated credential:
   repositories created before 15 July 2026 otherwise
   continue to issue name-based subjects that Azure will reject when configured
   with the immutable subject below. Repository administrators can configure and
   verify the setting with the GitHub CLI:

   ```sh
   gh api --method PUT \
     -H "X-GitHub-Api-Version: 2026-03-10" \
     repos/chattocorp/chatto/actions/oidc/customization/sub \
     -F use_default=true \
     -F use_immutable_subject=true
   gh api \
     -H "X-GitHub-Api-Version: 2026-03-10" \
     repos/chattocorp/chatto/actions/oidc/customization/sub
   ```

   The response must report `"use_default": true`,
   `"use_immutable_subject": true`, and the prefix
   `repo:chattocorp@261891647/chatto@1205013299`. The prefix shown by this API
   intentionally omits the job context; an OIDC token requested by the signing
   job appends `:environment:desktop-windows-signing`.

3. Create a Microsoft Entra application and service principal for the release
   workflow. Add a **GitHub Actions deploying Azure resources** federated
   credential for organization `chattocorp` (`261891647`), repository `chatto`
   (`1205013299`), and environment `desktop-windows-signing`, with audience
   `api://AzureADTokenExchange`. Azure generates the immutable-ID subject
   `repo:chattocorp@261891647/chatto@1205013299:environment:desktop-windows-signing`.
4. Grant that service principal only the **Artifact Signing Certificate Profile
   Signer** role, scoped to the Chatto Desktop signing account.
5. Create a protected `desktop-windows-signing` Actions environment and add the
   following secrets and variables to it. Keep the macOS credentials in the
   existing `desktop-signing` environment so the two platforms cannot access
   one another's signing credentials.

| Kind     | Name                                      | Value                                                       |
| -------- | ----------------------------------------- | ----------------------------------------------------------- |
| Secret   | `CHATTO_WINDOWS_AZURE_CLIENT_ID`          | Entra application (client) ID                               |
| Secret   | `CHATTO_WINDOWS_AZURE_TENANT_ID`          | Entra directory (tenant) ID                                 |
| Secret   | `CHATTO_WINDOWS_AZURE_SUBSCRIPTION_ID`    | Azure subscription ID                                       |
| Variable | `CHATTO_WINDOWS_SIGNING_ENDPOINT`         | Regional account endpoint, including `https://`             |
| Variable | `CHATTO_WINDOWS_SIGNING_ACCOUNT_NAME`     | Artifact Signing account name                               |
| Variable | `CHATTO_WINDOWS_CERTIFICATE_PROFILE_NAME` | Public Trust certificate profile name                       |
| Variable | `CHATTO_WINDOWS_EXPECTED_PUBLISHER`       | Complete certificate subject, exactly as Windows reports it |

Configure `desktop-windows-signing` to permit deployments only from
`v*` tags and the `main` branch used for manually dispatched
verification builds. Require a reviewer, who must approve a release before the
Windows runner can request an Azure token.
Enable prevention of self-review when another authorised reviewer is available;
do not enable it for a single-reviewer environment, because that would make
releases impossible. The release workflow fails before building when any
setting is missing or when the repository is not using the documented default
immutable OIDC subject. After building, it signs every `.exe`, `.dll`, and
native `.node` module recursively with SHA-256 and an RFC 3161 timestamp, then
rejects missing, invalid, or untimestamped signatures before packaging the ZIP.
The installer build then uses the same Azure login and ArtifactSigning module
to sign and verify its generated uninstaller before embedding it, and to sign
and verify the final installer. Both require the expected publisher and a valid
timestamp. A signing or verification failure stops desktop publication.
The main `chatto-desktop.exe` must also report the expected ChattoCorp publisher;
bundled third-party libraries may retain their original valid publisher, such
as Microsoft.

Before the first tagged release, manually run the `release` workflow with the
`desktop` target on `main`, approve the protected environment, and inspect the
Windows verification ZIP on a clean supported Windows installation. In File
Explorer, the Digital Signatures tab on `chatto-desktop.exe` must show the same
publisher configured in `CHATTO_WINDOWS_EXPECTED_PUBLISHER`; PowerShell's
`Get-AuthenticodeSignature` must report `Valid`.

For installer changes, also test the signed `*_Setup.exe` on Windows before
releasing. Check the publisher shown by Windows, installation as a standard
user, Start menu launch, existing ZIP-profile reuse, an upgrade from the previous
installed version, the prompt with the application running, and uninstall from
Windows Settings. Confirm that registrations and sessions survive the upgrade.
The automated reinstall test does not replace these interactive checks.

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

Electron allows camera, microphone, notification, clipboard write, and fullscreen
permission requests only for the fixed app origin. Clipboard reads and automatic
fullscreen without a user gesture remain blocked.
Screen sharing presents a native source picker.
Navigation outside the app is restricted to OAuth popup windows or opened in
the system browser; renderer Node.js integration is disabled and the renderer
is sandboxed.

## Prototype boundaries

This scaffold does not yet provide OS deep links, Linux distro packages, or
end-to-end desktop tests. Some identity providers
reject authentication inside embedded user agents, so a system-browser OAuth
handoff may still be required before treating this as a general release.
