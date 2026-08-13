# macOS game capture probe

This standalone diagnostic implements the first part of the
[macOS native-capture spike](https://github.com/chattocorp/chatto/issues/2022),
which follows the broader
[game-grade screen-sharing discovery](https://github.com/chattocorp/chatto/issues/1051):
can macOS deliver a game's video frames and application audio to a native Chatto
helper with useful timing information?

Its diagnostic capture command uses ScreenCaptureKit directly and can retain a
`.mov` for visual inspection. The packaged proof of concept additionally uses
LiveKit's Swift SDK to publish the selected window and its application audio
directly from the helper; Electron IPC is control-only and carries no media.

## Requirements

- macOS 15 or newer
- Xcode 16 or newer, or matching Command Line Tools
- Screen & System Audio Recording permission for the built executable

## Build and list sources

From the repository root:

```sh
cd apps/desktop/native/macos-capture-probe
xcrun swift build
xcrun swift run chatto-macos-capture-probe list
```

The source list reports displays followed by ordinary, visible application
windows of at least 320×180 points. Each window includes its temporary window
ID, application name, title, bundle identifier, and dimensions. The
`capture-with` value can be copied into a capture command. This mechanical
filter removes menu-bar items, wallpapers, and other macOS implementation
windows; it does not attempt to recognise games. Window IDs are valid only
while that window exists, so run `list` again rather than persisting them.

Window titles can contain sensitive information. They are printed only in
response to the explicit diagnostic `list` command; Chatto must not write a
future user-facing source list to application logs.

The first ScreenCaptureKit request may prompt for Screen & System Audio
Recording access. Grant access in **System Settings → Privacy & Security →
Screen & System Audio Recording**, then run the command again. macOS may require
the process to be restarted after permission changes.

## Capture a game

For a borderless or windowed game, start with its window ID:

```sh
xcrun swift run chatto-macos-capture-probe capture \
  --window 1234 \
  --duration 30
```

ScreenCaptureKit filters audio at application level. A single-window capture
therefore receives audio from the window's owning application, including audio
from any of that application's other windows.

For a fullscreen game, select its application and the display where it is
running:

```sh
xcrun swift run chatto-macos-capture-probe capture \
  --application com.example.game \
  --on-display 1 \
  --duration 30
```

For a display-wide control measurement:

```sh
xcrun swift run chatto-macos-capture-probe capture \
  --display 1 \
  --duration 30
```

Display capture includes the display's system audio rather than isolating one
application. Keep unrelated audio stopped during that test.

The defaults request 1920×1080 at 60 frames per second and 48 kHz stereo audio.
Use `--max-width`, `--max-height`, and `--fps` to change the requested output.

To retain a diagnostic recording, provide a new `.mov` path:

```sh
xcrun swift run chatto-macos-capture-probe capture \
  --window 1234 \
  --duration 15 \
  --output captures/game-window.mov
```

The probe refuses to overwrite an existing file. Recordings contain the selected
screen content and its audio; keep `captures/` private and delete files when they
are no longer needed. The directory is excluded from Git.

## Interpret the result

The summary reports:

- complete and non-complete ScreenCaptureKit video buffers;
- actual pixel-buffer dimensions and observed complete-frame cadence;
- sampled pixel brightness and the number of sampled frames whose content changed;
- frame intervals that were long enough to imply missing updates;
- audio buffer and frame counts, format, presentation-time span, and peak level;
- whether both complete video and non-silent 32-bit float PCM audio arrived.

An idle game may legitimately produce fewer new frames because ScreenCaptureKit
can report unchanged content without a new surface. Exercise continuous motion
and continuous game audio when measuring capture cadence.

The diagnostic capture does not establish end-to-end A/V synchronization,
network latency, protected-content behavior, or Windows viability. The direct
publisher establishes the intended LiveKit path, but its actual 60 fps cadence,
encoder choice, congestion behavior, and remote playback still need measurement.

## Chatto Desktop integration

The helper is an official part of every macOS Chatto Desktop build. Build the
complete application with the ordinary host-platform task:

```sh
mise desktop-build
```

This places the executable in a background application at
`Chatto Desktop.app/Contents/Helpers/Chatto Capture Helper.app`. The helper has
the stable bundle identifier `run.chatto.desktop.capture-helper`, and the
packager signs it in dependency order with the rest of the Electron bundle.
Current local and CI artifacts use ad-hoc signing; a production build still
needs a stable Developer ID identity and notarisation. macOS CI asserts that
the helper is executable, can start without invoking capture, has its framework
rpath, and satisfies the complete app bundle's strict signature validation.
Ad-hoc builds disable hardened runtime because they have no Team ID; builds
using a real identity enable it.

After installing a development or distribution certificate, select it for a
build by setting `CHATTO_MACOS_SIGN_IDENTITY` to the identity name shown by
`security find-identity -v -p codesigning`. Leaving the variable unset keeps
the ad-hoc development default.

To make the packaged parent launch the helper and list capture sources:

```sh
'apps/desktop/dist/Chatto Desktop.app/Contents/MacOS/Chatto Desktop' \
  --chatto-macos-capture-probe-list
```

This private switch exists only for packaging and macOS privacy-attribution
diagnostics. The normal macOS app exposes the source list through its
narrow game-capture bridge.

To exercise the packaged proof of concept, first open the game or other window
to capture, then launch a new Chatto Desktop instance through Launch Services:

```sh
open -n "apps/desktop/dist/Chatto Desktop.app" \
  --args --chatto-macos-capture-poc
```

Chatto presents a mechanical list of ordinary visible windows; it does not use
a game catalogue or attempt to recognise games. After the user chooses a
window, the nested helper records its video and owning application's audio for
15 seconds. The app then offers to open or reveal the temporary `.mov` file.
The recording is stored in a private `chatto-capture-poc-*` directory beneath
the current user's temporary directory and is not uploaded or retained by the
repository.

The macOS bundle exposes a complete real-time game-streaming path
through the normal Chatto call interface. Launch the app without a private
switch, join a call, and use the gamepad control in the call toolbar. The shared
frontend opens its own picker containing the helper's ordinary visible windows.
Choosing a window requests a fresh companion-publisher credential from Chatto,
passes it to the helper over stdin, and starts a direct LiveKit publication.
The initial profile captures up to a 1920-pixel maximum edge at 60 fps and
publishes three aspect-ratio-preserving H.264 resolution classes: 1920-edge at
60 fps and up to 8 Mbps, 1280-edge at 60 fps and up to 4 Mbps, and 640-edge at
30 fps and up to 1 Mbps. Exact widths and heights retain the source window's
aspect ratio. LiveKit selects a suitable layer for each receiver, while
dynacast pauses layers that no receiver is consuming. The publication also
includes 128 kbps stereo application audio and uses the active call's shared
E2EE key. Only `started`, `error`, and `ended` lifecycle messages cross the
desktop bridge. The active gamepad control stops the helper. Starting browser
screen sharing also stops game capture; camera and microphone remain
independent. Non-macOS builds omit this platform provider and therefore do not
show this control.

The helper uses a separate opaque LiveKit identity because reusing the joined
member's identity would disconnect the frontend connection. Token metadata
links the companion to its owning participant. The frontend presents the
companion's video and audio under that logical participant and suppresses the
owner's local game-audio playback. Server webhooks and reconciliation exclude
companions from durable call membership, while stale-room cleanup still removes
their connections.

The first run may request **Screen & System Audio Recording** permission for
Chatto Desktop. Quit and relaunch the app after granting it. Rebuilding an
ad-hoc-signed bundle changes its code identity and can make an existing privacy
grant stop matching; during discovery, run
`tccutil reset ScreenCapture run.chatto.desktop`, launch the unchanged bundle,
and grant access again. A stable Apple Development or Developer ID signature
should avoid this ad-hoc-build limitation.

In the ad-hoc-signed prototype, launching the app through macOS
Launch Services caused TCC to identify `run.chatto.desktop` as the responsible
subject for the nested `run.chatto.desktop.capture-helper` process. That is the
desired user-facing permission boundary: Chatto Desktop owns the permission
even though the native helper performs capture. Repeat this check with an Apple
Development signature before relying on permission persistence across builds.
