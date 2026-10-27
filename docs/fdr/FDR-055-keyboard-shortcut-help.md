# FDR-055: Keyboard Shortcut Help

**Status:** Active
**Last reviewed:** 2026-10-27

## Overview

Keyboard shortcut help shows the app and composer shortcuts for the current view.
It lets users discover shortcuts without leaving their draft or changing settings.

## Behavior

- Cmd+? on Apple platforms, or Ctrl+? elsewhere, opens help. Use Shift to type
  the question mark where the keyboard layout requires it.
- Cmd/Ctrl+/ keeps its existing function: focus the filter in views that have one.
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
- Labels use the current language. Keycaps use the platform's modifier names.
- Search filters the list by action label or displayed keys. Closing help clears
  the search, so the next opening shows the complete list for that view.

## Design Decisions

### 1. Keep the filter shortcut

**Decision:** Use Cmd/Ctrl+? for help and retain Cmd/Ctrl+/ for filters.
**Why:** Users can continue to reach filters with the existing shortcut.
**Tradeoff:** Typing the question mark depends on the keyboard layout.

### 2. Describe the current view

**Decision:** List app bindings from their owners and composer bindings from
the active editor configuration.
**Why:** Help should not advertise controls that have left the current view.
**Tradeoff:** Editor help must stay aligned with the editor's keymaps.

## Related

- [FDR-015: Quick Switcher](FDR-015-quick-switcher.md)
- [FDR-012: Notifications](FDR-012-notifications.md)
