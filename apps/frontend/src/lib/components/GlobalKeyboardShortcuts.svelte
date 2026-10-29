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
  import KeyboardShortcutHelp from './KeyboardShortcutHelp.svelte';
  import { getKeyboardShortcuts, type ShortcutDescription } from '$lib/keyboardShortcuts';
  import { m } from '$lib/i18n/messages';
  import { quickSwitcher } from '$lib/state/globals.svelte';
  import { getAppUiState } from '$lib/state/appUi.svelte';
  import { createNextUnreadNavigation } from '$lib/notifications/nextUnreadSource';

  const nextUnread = createNextUnreadNavigation(getAppUiState());
  const shortcuts = getKeyboardShortcuts();
  let helpVisible = $state(false);
  let helpEntries = $state<ShortcutDescription[]>([]);

  function openHelp() {
    if (helpVisible) return;
    helpEntries = shortcuts?.snapshot() ?? [];
    helpVisible = true;
  }

  function focusedForm() {
    return document.activeElement?.closest('form');
  }
</script>

<KeyboardShortcut
  key="k"
  command
  description={m('ui.open_quick_switcher')}
  ontrigger={() => quickSwitcher.open()}
/>
<KeyboardShortcut
  key="k"
  command
  shift
  description={m('common.keyboard_shortcuts.title')}
  ontrigger={openHelp}
/>
<KeyboardShortcut
  key="ArrowDown"
  alt
  shift
  allowInModal={false}
  description={m('chat.notifications.next_unread_source')}
  ontrigger={() => void nextUnread()}
/>
<KeyboardShortcut
  key="Enter"
  command
  capture={false}
  description={m('common.submit')}
  when={() => !!focusedForm()}
  ontrigger={() => focusedForm()?.requestSubmit()}
/>
<QuickSwitcher onNextUnread={nextUnread} />
<KeyboardShortcutHelp bind:visible={helpVisible} shortcuts={helpEntries} />
