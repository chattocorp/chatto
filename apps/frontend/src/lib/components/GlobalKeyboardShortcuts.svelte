<!--
@component

Registers global keyboard shortcuts for the application. Include this component
once at the root layout level.

-->
<script lang="ts">
  import { shortcutPreferences, shortcutProps } from '$lib/state/shortcutPreferences.svelte';
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
  {...shortcutProps(shortcutPreferences.bindings.switcher)}
  description={m('ui.open_quick_switcher')}
  ontrigger={() => quickSwitcher.open()}
/>
<KeyboardShortcut
  {...shortcutProps(shortcutPreferences.bindings.help)}
  description={m('common.keyboard_shortcuts.title')}
  ontrigger={openHelp}
/>
<KeyboardShortcut
  {...shortcutProps(shortcutPreferences.bindings.unread)}
  allowInModal={false}
  description={m('chat.notifications.next_unread_source')}
  ontrigger={() => void nextUnread()}
/>
<KeyboardShortcut
  {...shortcutProps(shortcutPreferences.bindings.submit)}
  capture={false}
  description={m('common.submit')}
  when={() => !!focusedForm()}
  ontrigger={() => focusedForm()?.requestSubmit()}
/>
<QuickSwitcher onNextUnread={nextUnread} />
<KeyboardShortcutHelp bind:visible={helpVisible} shortcuts={helpEntries} />
