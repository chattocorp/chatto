<!--
@component

Registers global keyboard shortcuts for the application. Include this component
once at the root layout level.

**Shortcuts:**
- `Cmd+Enter` (Mac) / `Ctrl+Enter` (Windows/Linux): Submit the form containing
  the currently focused element
- `Cmd+K` (Mac) / `Ctrl+K` (Windows/Linux): Open the quick switcher palette
- `Alt+Shift+ArrowDown`: Open the next unread notification source
-->
<script lang="ts">
  import QuickSwitcher from './QuickSwitcher.svelte';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { createNextUnreadNavigation } from '$lib/notifications/nextUnreadSource';

  const nextUnread = createNextUnreadNavigation(getAppUiState());

  // Claim this combination before the editor's duplicate-line binding can change the draft.
  function handleNotificationShortcut(e: KeyboardEvent) {
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

<svelte:window onkeydown={handleKeydown} onkeydowncapture={handleNotificationShortcut} />
<QuickSwitcher onNextUnread={nextUnread} />
