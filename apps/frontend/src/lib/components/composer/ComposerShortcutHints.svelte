<!-- @component
Advertises the mounted composer's shortcuts without installing competing editor
handlers. The editor and send preference determine the descriptions when help opens.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { getKeyboardShortcuts, type ShortcutDescription } from '$lib/keyboardShortcuts';
  import { m } from '$lib/i18n/messages';
  import { userPreferences } from '$lib/state/userPreferences.svelte';
  import { usesAppleShortcutModifier } from '$lib/utils/platformShortcuts';

  let { disabled = false }: { disabled?: boolean } = $props();
  const registry = getKeyboardShortcuts();

  function descriptions(): ShortcutDescription[] {
    if (disabled) return [];
    const commandSends = userPreferences.composerSendMode === 'modifier-enter';
    const visual = userPreferences.composerEditor === 'visual';
    const apple = usesAppleShortcutModifier();
    const entries: Omit<ShortcutDescription, 'group'>[] = [
      { key: 'Enter', command: commandSends, label: m('composer.send') },
      { key: 'Enter', command: !commandSends, label: m('common.keyboard_shortcuts.new_line') },
      { key: 'b', command: true, label: m('composer.format.bold') },
      { key: 'i', command: true, label: m('composer.format.italic') },
      { key: 'u', command: true, label: m('composer.format.underline') },
      { key: '`', label: m('composer.format.inline_code') },
      { key: 'z', command: true, label: m('common.keyboard_shortcuts.undo') },
      {
        key: apple ? 'z' : 'y',
        command: true,
        shift: apple,
        label: m('common.keyboard_shortcuts.redo')
      },
      { key: 'Tab', label: m('common.keyboard_shortcuts.complete') },
      { key: 'Escape', label: m('common.cancel') },
      { key: 'ArrowUp', label: m('common.keyboard_shortcuts.edit_last') }
    ];
    if (visual && apple)
      entries.push({ key: 'y', command: true, label: m('common.keyboard_shortcuts.redo') });
    if (visual && !apple)
      entries.push({
        key: 'z',
        command: true,
        shift: true,
        label: m('common.keyboard_shortcuts.redo')
      });
    if (visual)
      entries.push(
        { key: 'Enter', shift: true, label: m('common.keyboard_shortcuts.new_line') },
        { key: 'e', command: true, label: m('composer.format.inline_code') },
        { key: '1–6', command: true, alt: true, label: m('composer.format.heading') },
        { key: '7', command: true, shift: true, label: m('composer.format.ordered_list') },
        { key: '8', command: true, shift: true, label: m('composer.format.bullet_list') },
        { key: 'b', command: true, shift: true, label: m('composer.format.blockquote') },
        { key: 'c', command: true, alt: true, label: m('composer.format.code_block') }
      );
    entries.push(
      { key: 'Tab', label: m('composer.format.indent') },
      { key: 'Tab', shift: true, label: m('composer.format.outdent') }
    );
    return entries.map((entry) => ({ ...entry, group: 'editor' }));
  }

  // Catalogue registration follows the composer lifetime; editor keymaps stay untouched.
  onMount(() => registry?.register(descriptions));
</script>
