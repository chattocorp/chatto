# Chatto Mobile

Experimental iOS client. Capacitor embeds the shared static frontend without
a Chatto backend. Android packaging, native push, and native calls are deferred.

## Build and run

Install Xcode with an iOS Simulator runtime. From the repository root:

```sh
mise mobile-ios-build
mise mobile-ios-open
```

`mobile-ios-build` builds the frontend, copies its SPA shell and assets into the
iOS project, and builds an unsigned Simulator app under
`.context/mobile-ios-build/Build/Products/Debug-iphonesimulator/Chatto.app`.
`mobile-ios-open` refreshes the bundle and opens Xcode. Choose an iPhone Simulator
and select Run. For a physical device, select your development team in Xcode's
Signing & Capabilities pane. The project selects the ChattoCorp team by default;
contributors must select a team they can access. Signing secrets are not stored
in this repository.

Run `mise mobile-ios-sync` after frontend changes. Platform commands use the
`mobile-ios-*` namespace; future Android commands will use `mobile-android-*`.
The app serves bundled assets at
the stable `capacitor://localhost` origin. Its persistent webview storage owns
server registrations, preferences, and renewable sessions. This prototype does
not add a separate Keychain credential store.

## Connect a server

Use an HTTPS Chatto server that includes the mobile OAuth registration from
[PR #2456](https://github.com/chattocorp/chatto/pull/2456).
Existing releases without that registration cannot complete
mobile sign-in. The client identity is `eu.chattocorp.chatto.mobile`; its only callback is
`eu.chattocorp.chatto.mobile:/oauth/callback`. The native system authentication session
owns sign-in presentation. PKCE, state validation, token exchange, and session
registration remain in the shared frontend. Cancellation does not add a server.
The app does not bypass certificate validation or enable insecure HTTP access.

### Local development

Start `mise dev` in a separate terminal and use the HTTPS server URL that it
prints in the app's Add Server form. Keep that terminal running. The iOS app
contains a frontend bundle; it does not start the development server or load
frontend edits automatically. After frontend changes, run `mise mobile-ios-sync`
and run the app again in Xcode.

The Simulator must trust the certificate authority that issued the local
server certificate. A physical iPhone also needs a server address that it can
reach from its network; `localhost` on the phone refers to the phone itself.
Keep certificate validation enabled on both.

The client contacts only the servers selected by the user and the services
needed by those servers, such as identity providers, asset hosts, and LiveKit.
These systems can see the connecting device's IP address and the data needed
for the requested operation. There is no mobile push gateway or analytics
service in this prototype. Native bridge logging is disabled.

## Automated TestFlight updates

The `Publish iOS to TestFlight` workflow runs on Chatto `v*` tags, including
prereleases. It rebuilds the shared frontend, signs an iOS device archive, and
uploads it to App Store Connect. To upload between releases, run the workflow
manually from `main` in GitHub Actions. The selected commit must be reachable
from `main` or a `release-*` branch. This workflow is independent of server and
desktop publication; check its result separately.

Before the first run, create the `mobile-testflight` GitHub environment. Limit
its deployment branches and tags to `main`, `release-*`, and `v*`. Add required
reviewers if uploads need approval. Store these environment secrets:

| Secret                     | Value                                                                                                                |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `IOS_CERTIFICATE_BASE64`   | Base64-encoded Apple Distribution `.p12` certificate with its private key.                                           |
| `IOS_CERTIFICATE_PASSWORD` | Password used to export the `.p12` file.                                                                             |
| `IOS_PROFILE_BASE64`       | Base64-encoded App Store Connect provisioning profile for `eu.chattocorp.chatto.mobile`, made with that certificate. |
| `ASC_API_KEY_BASE64`       | Base64-encoded App Store Connect team API private key (`.p8`).                                                       |
| `ASC_API_KEY_ID`           | ID of that API key.                                                                                                  |
| `ASC_API_ISSUER_ID`        | Issuer ID of that team API key.                                                                                      |

Use an API key with permission to upload builds, such as the App Manager role.
The existing App Store Connect app, certificate, and profile must belong to the
same Apple team. Renew expired certificates and profiles in these secrets.
See [GitHub's certificate setup guide](https://docs.github.com/en/actions/how-tos/deploy/deploy-to-third-party-platforms/sign-xcode-applications).
The workflow uses a temporary keychain and removes the signing files after use.

The app version comes from `apps/frontend/package.json`, without the prerelease
suffix. The build number is `(100 + workflow run number).run attempt.0`.
Retries get a new build number. The workflow fails before the first component
exceeds 9999 or the attempt exceeds 99. When retrying an older run after a newer
upload, start a new manual run instead, so its build number increases. Check the
latest uploaded build before the first automated run or a later manual upload;
manual build numbers must not overtake the automation's range.

An accepted upload still needs Apple processing and any required compliance
information. Configure automatic distribution for the internal TestFlight group
in App Store Connect if testers should receive each processed build. External
testing still requires Apple's review. This workflow does not publish to the
public App Store or wait for processing and tester distribution. GitHub sends
the signed app and symbols to Apple; it sends no Chatto server credentials or
member data.

## Manual TestFlight updates

1. Check out the code to distribute and run `mise mobile-ios-open`. This rebuilds
   the frontend and copies it into the native project. Xcode alone does not
   rebuild the frontend. Sync also generates a 1024-pixel app icon without an
   alpha channel, as required for upload.
2. In Xcode, select the Chatto target and open General. Set Version to the intended
   release, such as `0.5.0`, and increase Build above the latest uploaded build.
3. Run the app on a physical device to check the changes.
4. Select Any iOS Device (arm64), then Product > Archive.
5. In Organizer, select the new archive, then Distribute App > App Store Connect.
   Use automatic signing and upload the build.
6. In App Store Connect > TestFlight, wait for processing, complete any required
   compliance information, and add test notes. Add the build to the internal
   tester group unless automatic distribution is enabled for that group.

External testing requires TestFlight App Review. Uploading a build does not
publish it on the public App Store. TestFlight builds expire after 90 days.
See [Apple's TestFlight guide](https://developer.apple.com/testflight/).

## Current limits

- No native push notifications, incoming-call integration, or share extension.
- Calls still use the shared web client. Continuing a call with the phone
  locked is a required follow-up, not a supported property of this build.
- No offline message store or in-app updater.
  TestFlight manages beta updates.
- Simulator build and launch do not verify physical-device media, authentication
  with real providers, session restoration after an OS eviction, or networking
  across Wi-Fi and mobile data.

See [FDR-048](../../docs/fdr/FDR-048-chatto-mobile.md) and
[ADR-099](../../docs/adr/ADR-099-capacitor-mobile-client.md).
