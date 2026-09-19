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
- No offline message store, automated release workflow, or in-app updater.
  TestFlight manages beta updates.
- Simulator build and launch do not verify physical-device media, authentication
  with real providers, session restoration after an OS eviction, or networking
  across Wi-Fi and mobile data.

See [FDR-048](../../docs/fdr/FDR-048-chatto-mobile.md) and
[ADR-099](../../docs/adr/ADR-099-capacitor-mobile-client.md).
