<!--
@component

Registers global keyboard shortcuts for the application. Include this component
once at the root layout level.

**Shortcuts:**
- `Cmd+Enter` (Mac) / `Ctrl+Enter` (Windows/Linux): Submit the form containing
  the currently focused element
- `Cmd+K` (Mac) / `Ctrl+K` (Windows/Linux): Open the quick switcher palette
- `Alt+Shift+ArrowDown`: Open the next unread notification source
- Production builds cancel reload shortcuts, including their hard-reload variants.
-->
<script lang="ts">
  import { dev } from '$app/environment';
  import QuickSwitcher from './QuickSwitcher.svelte';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { createNextUnreadNavigation } from '$lib/notifications/nextUnreadSource';

  const nextUnread = createNextUnreadNavigation(getAppUiState());

  // Capture reloads and notification navigation before editors or dialogs consume them.
  function handleCapturedShortcut(e: KeyboardEvent) {
    if (
      !dev &&
      !e.altKey &&
      (e.key === 'F5' || ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'r'))
    ) {
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    if (e.altKey && e.shiftKey && !e.metaKey && !e.ctrlKey && e.key === 'ArrowDown') {
      if (
        e.defaultPrevented ||
        e.isComposing ||
        document.querySelector('dialog[open], [aria-modal="true"]')
      )
        return;
      e.preventDefault();
      e.stopPropagation();
      if (!e.repeat) void nextUnread();
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault();
      quickSwitcher.open();
      return;
    }

    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      const form = document.activeElement?.closest('form');
      if (form) {
        e.preventDefault();
        form.requestSubmit();
      }
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} onkeydowncapture={handleCapturedShortcut} />
<QuickSwitcher onNextUnread={nextUnread} />
