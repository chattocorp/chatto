# macOS native screen-capture helper

This helper grew out of the first standalone diagnostic for the
[macOS native-capture spike](https://github.com/chattocorp/chatto/issues/2022),
which follows the broader
[game-grade screen-sharing discovery](https://github.com/chattocorp/chatto/issues/1051):
can macOS deliver a game's video frames and application audio to a native Chatto
helper with useful timing information?

Its diagnostic capture command uses ScreenCaptureKit directly and can retain a
`.mov` for visual inspection. The packaged proof of concept additionally uses
LiveKit's Swift SDK to publish the selected window and its application audio
directly from the helper; Electron IPC is control-only and carries no media.
The shipping integration supports application windows and complete displays
through one native screen-share capability.

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

The private `list-previews` command used by Chatto Desktop returns a bounded
binary frame containing a versioned JSON manifest followed by static JPEG
previews. Electron validates and converts that frame to temporary structured
clone data for the shared frontend. Preview images stay in memory and are
released when the chooser closes.

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

## Local encoder benchmark

The separate `chatto-video-benchmark` executable compares VideoToolbox settings
without screen capture, a server, or network traffic. It is a developer tool and
is not copied into the desktop app. It requires hardware encoding but no screen
recording permission.

From the repository root, select a new output directory:

```sh
mise desktop-video-benchmark -- --output .context/video-benchmark
```

Each run uses the same generated 1920×1122 BGRA frames at 60 FPS. The six-second
sequence contains two seconds each of a still scene, a slow pan, and a fast pan.
Text, textured tiles, and moving objects exercise compression. This synthetic
fixture is not a substitute for a game recording. The default two repetitions
use forward and reverse configuration order to expose order-related variation.

The matrix starts with the current SDK's Baseline AutoLevel settings at a 6 Mbps
target. Each comparison changes one control: speed priority, room below the
hard rate limit, High profile, low-latency rate control with High profile, QP
ceiling, an 8 Mbps target, or HEVC. A follow-up combines High profile and rate-limit
headroom, compared with High profile alone. The report names the comparison for each
variant. It records property-setting errors and read-back values. An unsupported
setting does not establish that a variant was tested successfully.
If encoder creation, hardware verification, or decoding fails, the tool records
the error and continues with the other variants. It exits with a nonzero status
after the matrix if any run failed. Completed results remain available.

All configured five-second bitrate ceilings are at or below 8 Mbps. Like the
SDK, the benchmark permits shorter bursts above that rate. Reports show actual
one-second and five-second peaks; these limits are not packet-level limits.
Reports flag measured five-second output above the requested ceiling or 8 Mbps.
Property acceptance alone does not prove that the encoder obeyed the limit.
The `tuned-6`, `tuned-vbr-100ms-6`, and `tuned-vbr-250ms-6` variants compare
the current Constrained High encoder with macOS 26 VBR at a 6 Mbps target
and an 8 Mbps maximum. VBR replaces `AverageBitRate` and `DataRateLimits`
with `VariableBitRate` and the VBV controls. Each VBR variant uses zero
initial VBV delay and its named buffer duration. The five-second output
measurements remain useful for comparison, but VBR's maximum describes a
buffer model rather than the old five-second data limit.

The benchmark holds each target constant. It does not reproduce WebRTC's
changing bandwidth estimates, packet pacing, simulcast, or receiver behavior.

Each run directory contains a playable `encoded.mov`, per-frame sizes and encode
callback latencies in `frames.json`, and a `result.json`. The root `results.json`
contains all runs. `summary.csv` provides one row per run, `environment.json`
records the fixture and OS version, and `comparison.png` shows native-size crops.
`reference.png` and `decoded.png` show matching frames from
the fast pan. The quality score is PSNR from RGB-weighted luma error, sampled at
four frames per second across all pixels. Color conversion also contributes
error. Compare the videos and frame drops as well as the scores. High PSNR alone
does not establish good motion quality.

Use `--only baseline-6,quality-6`, `--repeats 1`, or smaller even `--width` and
`--height` values for a quick check. Source frames are generated before their
scheduled submission. `maximumSubmitLatenessMs` identifies runs where the host
could not maintain the requested input cadence. Each variant uses a new encoder
session; no encoded output is reused across variants. Existing output directories
are rejected to protect prior measurements.

## Chatto Desktop integration

The helper is an official part of every macOS Chatto Desktop build. Build the
complete application with the ordinary host-platform task:

```sh
mise desktop-build
```

This places the executable in a background application at
`Chatto.app/Contents/Helpers/Chatto Capture Helper.app`. The helper has
the stable bundle identifier `run.chatto.desktop.capture-helper`, and the
packager signs it in dependency order with the rest of the Electron bundle.
Local and pull-request artifacts use ad-hoc signing; release artifacts use a
stable Developer ID identity and Apple notarisation. macOS CI asserts that
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
'apps/desktop/dist/Chatto.app/Contents/MacOS/Chatto' \
  --chatto-macos-capture-probe-list
```

This private switch exists only for packaging and macOS privacy-attribution
diagnostics. The normal macOS app exposes the source list through its narrow
native screen-share capability.

To exercise the packaged proof of concept, first open the game or other window
to capture, then launch a new Chatto Desktop instance through Launch Services:

```sh
open -n "apps/desktop/dist/Chatto.app" \
  --args --chatto-macos-capture-poc
```

Chatto presents a mechanical list of ordinary visible windows; it does not use
a game catalogue or attempt to recognise games. After the user chooses a
window, the nested helper records its video and owning application's audio for
15 seconds. The app then offers to open or reveal the temporary `.mov` file.
The recording is stored in a private `chatto-capture-poc-*` directory beneath
the current user's temporary directory and is not uploaded or retained by the
repository.

The macOS bundle exposes a complete real-time native screen-sharing path
through the normal Chatto call interface. Launch the app without a private
switch, join a call, and use the screen-share control in the call toolbar. The
shared frontend opens its own picker with static previews, grouped into
**Applications** and **Entire Screen**. Choosing a source requests a fresh
companion-publisher credential from Chatto, passes it to the helper over stdin,
and starts a direct LiveKit publication. The desktop host exposes only
short-lived, single-use random source offers to the renderer, verifies the
selected window still belongs to the enumerated application, and excludes
Chatto Desktop's own windows from the application list.
The default captures up to a 1920-pixel maximum edge at 60 fps and publishes
one aspect-ratio-preserving tuned H.264 layer with resolution priority.
Automatic mode uses VBR on macOS 26 or later and retries unsupported VBR setup
with tuned average-rate control. Older systems select average-rate control
directly. The frontend requests video dimensions in physical pixels for HiDPI.
The tuned encoder also fits frames within H.264 level 4.2's 8704-macroblock
limit at 60 FPS. The 1920×1122 test size stays unchanged. A 1920×1200 source
scales to 1872×1170, and a 1920×1920 source scales to 1488×1488. This preserves
the full image instead of rejecting taller sources or raising the decoder
requirement. VideoToolbox scales the input; reports show the coded dimensions.

The stock H.264 and HEVC Diagnostics options publish three resolution classes: 1920-edge at
60 fps and up to 8 Mbps, 1280-edge at 60 fps and up to 4 Mbps, and 640-edge at
30 fps and up to 1 Mbps. Exact widths and heights retain the source window's
aspect ratio. LiveKit selects a suitable layer for each receiver, while
dynacast pauses layers that no receiver is consuming. These are bitrate
ceilings, not guaranteed transmission rates. If receivers need all three
layers, the combined video ceiling is 13 Mbps. The desktop
Diagnostics menu selects the preferred codec for the next share and copies
the publisher's bounded diagnostic report. Both codecs use the same limits,
and there is no parallel backup-codec publication. An explicit server rejection
of HEVC retries H.264. Receivers must support the chosen codec; use the H.264
menu option and restart sharing if a viewer cannot receive HEVC.
The **Test H.264 Single Layer (8 Mbps, Resolution First) for Next Share** option
disables simulcast and prioritizes resolution, allowing frame rate to decrease
under pressure. It keeps the same capture settings and full-layer ceiling,
with no lower layers or backup codec. Diagnostic samples include
`maintainsResolution`, the layer mode and, when available, the outgoing bandwidth
estimate from the selected publisher transport candidate pair. No candidate identifiers
or addresses enter the report.

The **Test Tuned H.264 (8 Mbps, Resolution First) for Next Share** option uses
a custom hardware VideoToolbox encoder through the LiveKit encoder factory.
It sends one H.264 High layer and preserves resolution. The average target is
90% of WebRTC's current video allowance. The five-second data limit is the
allowance itself, capped at 8 Mbps. This gives rate control headroom without
raising the limit above the network allowance. For example, an 8 Mbps allowance
sets a 7.2 Mbps average target; a 3 Mbps allowance sets 2.7 Mbps. Zero pauses
encoding. As with the SDK encoder, shorter bursts can exceed the five-second
limit; this is not an instantaneous wire-rate cap. Audio, retransmissions, and
transport overhead are additional.

This experiment requires hardware H.264 High encoding and a receiver that
accepts High profile at level 4.2 or above. It does not fall back to Baseline or
software encoding. If negotiation or encoding fails,
select another Diagnostics mode and start a new share.

The copied report identifies `ChattoVideoToolboxH264`. Its `tunedEncoder` object
includes the requested allowance, average target, five-second ceiling, accepted
average target, hardware status, actual SPS profile, encoded/dropped frame
counts, rejected callbacks, and the last VideoToolbox error code and operation.
An absent QP means unavailable. The custom encoder does not estimate QP.

Run the hardware test on a Mac:

```sh
CHATTO_TEST_HARDWARE_ENCODER=1 mise test-desktop-macos-capture
```

The ordinary test task includes rate-policy and
packet-conversion tests without requiring hardware. Set
`CHATTO_TEST_ENCODED_OUTPUT` to a scratch `.h264` path to save the synthetic
test stream for an independent decoder check.

The default VBR mode and **Test Tuned H.264 VBR (6 Mbps Target, 8 Mbps Max) for Next Share**
option use the newer VBR controls on macOS 26 or later. They target the smaller
of 6 Mbps or 90% of WebRTC's current allowance. Its VBV maximum follows that
allowance, capped at 8 Mbps. Zero still pauses encoding. The VBV buffer duration
is 250 ms with zero initial delay. This buffer setting is not a measured
end-to-end delay. Local repeated tests measured about 22 ms median and 34 ms
95th-percentile compression callback delay, compared with about 6 and 7 ms
for the average-rate control. Live capture, transport, and decoding add delay.

VBR requires hardware support. The explicit test option does not silently fall
back. Automatic mode retries failed VBR setup once with average-rate control;
this fallback remains active for that encoder. The report's
`tunedEncoder.requestedRateControl` identifies `auto`, `vbr`, or `average`.
`rateControl` identifies the actual mode; `fallbackStatus` retains a failed VBR
setup status separately from the working encoder's `lastStatus`.
`averageBps` and `appliedAverageBps` carry the VBR target in this mode;
`appliedCeilingBps` is the accepted VBV maximum and `vbvBufferMs` is the accepted
buffer duration. `queueDrops` counts input frames rejected by our bounded queue;
`peakPendingFrames` is the largest observed queue size.
VBR permits six pending frames; the average-rate mode permits three. This keeps
the helper from building an unbounded input queue when encoding is slow.

Window publication also includes 128 kbps stereo owning-application audio. Display publication is
video-only because system audio would include Chatto's remote call playback and
echo it back into the room. Both paths use the active call's shared E2EE key.
The helper also emits allowlisted `diagnostic` JSON lines every second for the
first 20 seconds, then every five seconds.
The desktop host keeps these in memory; only `started`, `error`, and `ended`
lifecycle messages cross the renderer bridge. The active screen-share control stops the helper; camera and microphone
remain independent. Non-macOS builds omit this platform provider, so the same
control continues through LiveKit and the browser's native source chooser.

The helper uses a separate opaque LiveKit identity because reusing the joined
member's identity would disconnect the frontend connection. Token metadata
links the companion to its owning participant. The frontend presents the
companion's video and audio under that logical participant and never attaches
the owner's local companion audio. Server webhooks and reconciliation exclude
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
