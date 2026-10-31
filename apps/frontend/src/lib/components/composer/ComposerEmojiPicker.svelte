<!-- @component Inserts emoji from the shared picker at the editor selection. -->
<script lang="ts">
  import { tick } from 'svelte';
  import { CompactActionButton, ContextMenu } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import EmojiPicker from '../EmojiPicker.svelte';
  import type { ComposerEditorApi } from './editorTypes';

  let {
    serverId,
    disabled,
    editorApi
  }: {
    serverId: string;
    disabled: boolean;
    editorApi: ComposerEditorApi | null;
  } = $props();

  // An editor or availability change dismisses the picker and discards its target.
  let trigger = $derived.by<HTMLButtonElement | null>(() => {
    void editorApi;
    void disabled;
    return null;
  });
  const anchor = $derived(trigger?.getBoundingClientRect());

  async function closePicker() {
    const button = trigger;
    trigger = null;
    await tick();
    button?.focus();
  }

  async function insertEmoji(emoji: string) {
    const editor = editorApi;
    if (disabled || !editor) return;
    trigger = null;
    // Remove the touch modal before focusing the editor outside it.
    await tick();
    if (!disabled && editorApi === editor) editor.insertText(emoji);
  }
</script>

<CompactActionButton
  touchFriendly
  label={m('composer.insert_emoji')}
  title={m('composer.insert_emoji')}
  type="button"
  disabled={disabled || !editorApi}
  aria-haspopup="dialog"
  aria-expanded={trigger !== null}
  onpointerdown={(event) => event.preventDefault()}
  onclick={(event) => (trigger = event.currentTarget)}
>
  <span aria-hidden="true" class="iconify icon-[uil--smile] text-[15px]"></span>
</CompactActionButton>

{#if trigger}
  <ContextMenu {anchor} role="dialog" ariaLabel={m('composer.insert_emoji')} onclose={closePicker}>
    <EmojiPicker {serverId} onSelect={insertEmoji} onClose={closePicker} />
  </ContextMenu>
{/if}
