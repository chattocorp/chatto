# ADR-072: Optional Host Capabilities in the Shared Frontend

**Date:** 2026-08-13

**Status:** Accepted

**Supersedes:** The blanket prohibition on preload bridges in
[ADR-067](ADR-067-electron-desktop-client.md). Its sandbox, context-isolation,
and least-privilege requirements remain in force.

## Context

Chatto ships one SvelteKit frontend in ordinary web browsers and in Chatto
Desktop's sandboxed Electron renderer. Most product behavior should remain
identical, but a desktop host can sometimes provide a materially better native
implementation. macOS screen sharing is the first case: the browser must use
its system picker, while Desktop can enumerate windows and displays, show
previews in Chatto's UI, and publish through a native LiveKit helper.

Branching on user agents, URL schemes, or a global "is desktop" flag would
couple unrelated features to one host distinction. Giving the renderer a broad
Electron or operating-system API would weaken the sandbox and make future
Windows and Linux implementations part of the product frontend.

## Decision

The shared frontend will treat host integrations as narrow, optional
capabilities. A host may expose capability-specific methods beneath the
`window.chattoDesktop` namespace through an isolated preload bridge. The
frontend must access each capability through a focused adapter module that:

- feature-detects the capability itself, not the host, user agent, platform, or
  application origin;
- validates every host response before exposing it to application state;
- uses temporary opaque identifiers instead of platform-native coordinates;
- keeps credentials, filesystem paths, processes, and general Electron APIs
  outside the renderer contract; and
- leaves the ordinary browser implementation as the complete default path when
  the capability is absent.

Visible components continue to express one product action. They may select a
capability implementation at the point of use, but must not fork the rest of
the application into desktop and web variants. Capability availability is
independent of server protocol support, server configuration, and viewer
permission; those remain separate checks.

The first capability is `window.chattoDesktop.screenShare`. It lists bounded,
temporary window/display descriptions with in-memory preview bytes and starts
a native companion publisher for an explicitly selected source. Electron owns
the helper's framed binary preview protocol and converts it to structured-clone
data for the renderer. Media never crosses Electron IPC, and preview bytes are
not written to disk or retained after the chooser closes.

Source enumeration requires a current user activation and is cancellable. The
host replaces native window/display coordinates with short-lived, single-use
random offers, invalidates older offers on every enumeration, and binds window
offers to the enumerated owning application before publishing. Chatto Desktop's
own windows are excluded because their application audio contains remote call
playback. Helper processes are serialized and time-bounded so repeatedly
opening or closing the chooser cannot accumulate background enumerations.

When `screenShare` is absent, the same frontend control invokes LiveKit's
browser screen-sharing path and therefore the browser or operating system's
picker. When it is present, the control opens Chatto's native source chooser.
Stopping, pending state, and logical participant presentation remain shared.

## Consequences

Chatto keeps one accessible, translated frontend and one screen-share product
concept while allowing hosts to improve selected operations. New host
integrations require an explicit small contract, validation tests on both sides
of the bridge, and a working browser baseline. A capability can be added on one
desktop platform without teaching the frontend which platform supplied it.

The preload bridge is now a privileged compatibility and security boundary.
Electron maintainers must keep context isolation, renderer sandboxing, request
size limits, opaque identifiers, and least authority intact. Capability
contracts may need their own protocol version when they evolve; the presence of
`chattoDesktop` alone must never be used as a general desktop switch.

The browser baseline can offer less control than a host capability. That is
acceptable when browser security requires its own chooser or when exposing the
same native behavior is impossible. Product copy and state semantics should
still converge after the user has selected a source.

## Related

- [ADR-067](ADR-067-electron-desktop-client.md)
- [FDR-016](../fdr/FDR-016-voice-calls.md)
- [FDR-034](../fdr/FDR-034-chatto-desktop.md)
