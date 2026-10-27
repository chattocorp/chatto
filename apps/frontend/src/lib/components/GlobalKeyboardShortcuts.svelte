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
  import KeyboardShortcut from './KeyboardShortcut.svelte';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { createNextUnreadNavigation } from '$lib/notifications/nextUnreadSource';

  const nextUnread = createNextUnreadNavigation(getAppUiState());

  function focusedForm() {
    return document.activeElement?.closest('form');
  }
</script>

<KeyboardShortcut key="k" command ontrigger={() => quickSwitcher.open()} />
<KeyboardShortcut
  key="ArrowDown"
  alt
  shift
  allowInModal={false}
  ontrigger={() => void nextUnread()}
/>
<KeyboardShortcut
  key="Enter"
  command
  capture={false}
  when={() => !!focusedForm()}
  ontrigger={() => focusedForm()?.requestSubmit()}
/>
<QuickSwitcher onNextUnread={nextUnread} />
