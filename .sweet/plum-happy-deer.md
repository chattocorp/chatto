---
type: feature
---

Open Settings with Cmd+, on macOS or Ctrl+, on other platforms. Customise app shortcuts in App Preferences → Keyboard shortcuts. In Chatto Desktop, bind a system-wide shortcut to mute or unmute the microphone during a call.

On Linux, if you installed the old Chatto launcher, reinstall the updated launcher and pin it again if needed. For a custom launcher, rename `chatto-desktop.desktop` to `run.chatto.desktop.desktop`, set `StartupWMClass=run.chatto.desktop`, and update any `--class=chatto-desktop` launch argument to `--class=run.chatto.desktop`. Wayland uses the new launcher to identify Chatto for system-wide shortcuts.
