# FDR-055: Keyboard Shortcut Help

**Status:** Active
**Last reviewed:** 2026-11-26

## Overview

Keyboard shortcut help shows the app and composer shortcuts for the current view.
It lets users discover shortcuts without leaving their draft or changing settings.

## Behavior

- Narrow-screen help leaves the app header interactive. A header click closes help and runs the header action on the first click. Desktop help remains modal.

- By default, Cmd+Shift+K on Apple platforms, or Ctrl+Shift+K elsewhere, opens help.
  Cmd/Ctrl+/ keeps room search and filters.
- Help lists registered app shortcuts and filters in the current view. A form's
  submit shortcut appears when the focused field belongs to a form.
- When a message composer is available, help also lists sending, newlines,
  formatting, undo, redo, completion, and editing shortcuts. The list follows
  the current editor and send-mode preferences. The editor continues to own
  these bindings. Some actions require selected text or an empty draft.
- The list describes the view before help takes focus. Multiple composers with
  the same shortcuts do not create duplicate rows. Closing a view removes its
  shortcuts from the next help list.
- Escape or Close dismisses help and restores focus. Opening help does not
  change the draft or its selection.
- Below 768 px viewport width, help fills the visible content area below the
  app header, which stays visible and undimmed. Help uses a back
  arrow to close. The header and search stay available while shortcuts scroll,
  including above the software keyboard. Wider windows keep the floating
  palette. Resizing retains the query and input focus.
- Labels use the current language. Keycaps use the platform's modifier names.
- Search filters the list by action label or displayed keys. Closing help clears
  the search, so the next opening shows the complete list for that view.

- In App Preferences → Keyboard shortcuts, users can change, clear, or restore
  the shortcuts for the quick switcher, shortcut help, next unread source,
  Settings, form submission, and room search or filters. By default, Cmd+, on
  macOS and Ctrl+, on other platforms open the Settings overview. Changes apply to all servers
  on this device. Select a row to record a shortcut. Use the row menu to clear
  the shortcut or restore its default. Help shows the current bindings. While typing,
  Alt or Option with a printable key can enter text in the focused field.
- Chatto Desktop also offers a system-wide microphone toggle. The user must
  assign a shortcut first. A system-wide shortcut requires Control or Super
  (Command on macOS) and a trigger key. On
  macOS it can combine Control, Command, and Option, for example ⌃⌘⌥M.
  During a connected voice call, the shortcut works
  while another application has focus. Leaving the call releases the shortcut.
  If registration fails, Chatto asks the user to choose another combination.

## Design Decisions

### 1. Keep the filter shortcut

**Decision:** Use Cmd/Ctrl+Shift+K for help and preserve Cmd/Ctrl+/ for room search and filters.
**Why:** Users can continue to reach filters with the existing shortcut.
Cmd+Shift+/ can open a native browser menu before the page receives the key.
**Tradeoff:** Browsers can reserve shortcuts and prevent the app from receiving them.

### 2. Describe the current view

**Decision:** List app bindings from their owners and composer bindings from
the active editor configuration.
**Why:** Help should not advertise controls that have left the current view.
**Tradeoff:** Editor help must stay aligned with the editor's keymaps.

## Related

- [FDR-015: Quick Switcher](FDR-015-quick-switcher.md)
- [FDR-012: Notifications](FDR-012-notifications.md)
