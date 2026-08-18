# Windows native game-capture helper

This helper implements the Windows portion of
[issue #2021](https://github.com/chattocorp/chatto/issues/2021). Chatto Desktop
uses it to enumerate ordinary visible windows, capture a selected window with
isolated owning-process-tree audio, and publish both directly to LiveKit with
the call's shared E2EE key. Diagnostic commands remain available for focused
native verification.

The capture stack is Windows Graphics Capture into D3D11 textures, WASAPI
process-loopback audio, direct NVIDIA NVENC H.264 with Media Foundation
hardware fallback, and a pinned public Chatto fork of the LiveKit C++ SDK.
The direct NVIDIA path keeps production video frames on the GPU: capture copies
each frame into a shareable D3D11 texture, the NVIDIA video processor scales it
and converts BGRA to BT.709 NV12, and NVENC reads the registered NV12 surface.
It uses the driver's low-latency P5 preset, quarter-resolution multipass, and
spatial adaptive quantisation. Media Foundation remains a compatibility
fallback and performs a CPU readback when direct NVENC is unavailable. The fork
passes complete encoded access units into WebRTC without a second software
encode and forwards keyframe and rate-control requests back to the selected
encoder. A single-slot latest-frame handoff keeps
that work off the Windows capture callback and discards superseded frames
instead of accumulating latency. Production Desktop builds compile and embed
the executable plus its two LiveKit DLLs.

Publication uses one full-cadence H.264 video layer. LiveKit C++ 1.7 exposes only a
simulcast switch rather than custom screen-share layers, and its default lowest
screen-share layer is capped at 3 fps; compact adaptive-stream tiles therefore
turn a game stream into a slideshow. A custom game-oriented simulcast ladder
remains follow-up work for a newer SDK or a pinned fork.
Dynacast is disabled for this single layer so Desktop's local preview may
throttle or unsubscribe without suspending the independent native publisher.

During publication, the helper treats a missing source window or a two-second
frame stall as a recoverable application transition. It first selects another
ordinary visible window belonging to the same executable, preferring the
original process, and can restart capture on the same still-valid window. If no
replacement appears within three seconds, it disconnects the companion so the
share ends. Desktop requests shutdown with a line-delimited `stop` control
command and force-terminates only if the helper does not exit in time.
WGC texture resizes do not change the published track dimensions: every frame
is scaled from its current texture size into the stable output selected when
the share started.
WGC can stall when a monitor-covering borderless game enters DirectFlip or
Independent Flip and bypasses desktop composition. If the selected window owns
its complete foreground monitor, the helper immediately captures that monitor
through WGC instead of waiting for sparse window-WGC heartbeat frames to stop.
DXGI Desktop Duplication remains the final
fallback if monitor WGC also stalls. Both display paths end when the game leaves
that presentation. Failure to initialise a fallback is non-fatal and retried
with a cooldown. Publisher metrics identify the active backend as `wgc-window`,
`wgc-monitor`, or `dxgi-display`.
Windows invalidates Desktop Duplication during some presentation-producer
transitions with `DXGI_ERROR_ACCESS_LOST`; the helper treats that as a
recoverable signal and recreates the duplication interface for the new
producer.

Non-content publisher metrics are emitted every two seconds even while capture
or publication is stalled. Alongside capture, GPU copy, GPU conversion, encoder
submission, bitstream wait, encoding, and RTP counters, they report the latest
WGC texture dimensions and how often those dimensions changed. Desktop
separately acknowledges a received stop command before waiting for the helper
to disconnect and exit.

## Requirements

- Windows 11, or a Windows 10 version that supports the APIs under test
- Visual Studio 2022 Build Tools with the Desktop development with C++ workload
- Windows 11 SDK
- CMake 3.25 or newer

CMake downloads the public `chattocorp/client-sdk-cpp`
`v1.7.0-chatto.3` prerelease and verifies the archive's pinned SHA-256 unless
`CHATTO_LIVEKIT_SDK_ROOT` points at an already extracted SDK. That C++ fork
pins the public `chattocorp/rust-sdks` pre-encoded-video FFI release.
CMake also downloads and verifies the permissively licensed FFmpeg
`nv-codec-headers` 13.0.19.1 release. NVENC itself is loaded dynamically from
the installed NVIDIA display driver; no NVIDIA runtime DLL is bundled.

## Build and test

Open a Developer PowerShell for Visual Studio, then run:

```powershell
cmake -S apps/desktop/native/windows-capture-probe `
  -B apps/desktop/native/windows-capture-probe/build
cmake --build apps/desktop/native/windows-capture-probe/build `
  --config RelWithDebInfo
ctest --test-dir apps/desktop/native/windows-capture-probe/build `
  -C RelWithDebInfo --output-on-failure
```

On a development machine with a GPU, the opt-in smoke mode also proves that the
selected hardware backend emits keyframed Annex-B H.264 and accepts a dynamic
bitrate change. It is intentionally not a CI test because hosted Windows
runners may not expose a hardware encoder:

```powershell
apps/desktop/native/windows-capture-probe/build/RelWithDebInfo/chatto-windows-h264-encoder-tests.exe `
  --hardware
```

Check Windows Graphics Capture availability and list candidate windows:

```powershell
apps/desktop/native/windows-capture-probe/build/RelWithDebInfo/chatto-windows-capture-probe.exe support
apps/desktop/native/windows-capture-probe/build/RelWithDebInfo/chatto-windows-capture-probe.exe list
```

Copy a temporary `hwnd` value from the list and capture its frames for 15
seconds:

```powershell
apps/desktop/native/windows-capture-probe/build/RelWithDebInfo/chatto-windows-capture-probe.exe `
  capture --window 0x123456 --duration 15 --fps 60
```

Add `--preview` to open a native Win32 window backed by a flip-model DXGI swap
chain. Captured textures are copied directly on the GPU; the preview title shows
live observed FPS, frame and content-sample counts, latest isolated audio peak,
captured audio frames, and audio discontinuities. Closing the preview ends video
acquisition and process-audio capture early.

The summary reports delivered frames, current content dimensions, native
system-relative timestamp span, observed cadence, longest interval, inferred
gaps, frame-pool resizes, and whether the source closed. Each delivered surface
must expose an `ID3D11Texture2D`; the capture fails instead of counting an
unusable frame. A sparse CPU readback samples a bounded pixel grid roughly four
times per second and reports aggregate luminance, black samples, and changing
sample hashes; it never retains or writes an image. By default, it concurrently captures audio
from the selected window's owning process tree and reports the A/V start delta
on the shared system-relative 100-nanosecond timeline. Use `--video-only` to
isolate video diagnostics. The frame rate is a diagnostic expectation used for
gap detection; Windows Graphics Capture controls actual delivery.

The completed summary also reports probe wall time, total process CPU time,
single-core-equivalent CPU percentage, and peak working-set size. GPU utilisation
and impact on the game's own frame pacing still require an external measurement
method.

Capture 48 kHz stereo audio emitted by a process and its child-process tree:

```powershell
apps/desktop/native/windows-capture-probe/build/RelWithDebInfo/chatto-windows-capture-probe.exe `
  audio --process 1234 --duration 15
```

The audio summary reports packets, frames, format, QPC-derived timestamp span,
peak level, silent packets, discontinuities, and timestamp errors. Samples stay
in memory only and are discarded after their levels and timing are measured.

The source list reports temporary native window coordinates, executable names,
and dimensions. It mechanically excludes hidden, cloaked, owned, and very small
windows; it does not try to recognise games. Native coordinates are diagnostic
input only and must not become a renderer or public API contract.

Window titles can contain sensitive information and are omitted by default.
The explicit `list --include-titles` option prints them only to help a person
identify a source during local diagnostics. Do not paste that output into logs
or issue reports without reviewing it.

## Production boundary

The `list-json` and `publish` commands are private Desktop protocols. Electron
replaces native handles with short-lived single-use offers, obtains static JPEG
previews itself, and sends only a selected offer plus a fresh LiveKit
credential to the helper. Before publishing, the helper re-resolves the window
and compares a SHA-256 binding of its executable path. Raw paths and handles do
not cross renderer IPC. Media remains in the native helper; only source
descriptions, credentials, acknowledged lifecycle status, and aggregate
non-content performance timings cross Electron.

Windows offers both window and display capture; its internal monitor WGC and
DXGI paths also protect window sharing across direct-presentation transitions.
Audio-device recovery, GPU scaling and colour conversion, direct handoff of
capture textures to the encoder, a game-oriented simulcast ladder, and broader
DX11/DX12/Vulkan game validation remain follow-up work.
