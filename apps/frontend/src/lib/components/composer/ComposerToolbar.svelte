<!--
@component

Compact message-level actions for the composer input row. Formatting commands
live in `ComposerFormattingToolbar` so this row can stay aligned with the
48-pixel app-shell controls.
-->
<script lang="ts">
  import CompactActionButton from '$lib/ui/CompactActionButton.svelte';
  import { m } from '$lib/i18n/messages';
  import ComposerTimestampPicker from './ComposerTimestampPicker.svelte';
  import type { ComposerEditorApi } from './editorTypes';

  let {
    editorApi,
    inputDisabled,
    canAttach,
    isEditing,
    canSubmit,
    fileInputElement,
    effectiveTimezone,
    showCreateThread = false,
    createThread = false,
    createThreadRequired = false,
    onToggleCreateThread = () => {},
    showAlsoSendToChannel = false,
    echoToConversation = false,
    alsoSendToChannel = false,
    onToggleAlsoSendToChannel = () => {},
    onsubmit
  }: {
    editorApi: ComposerEditorApi | null;
    inputDisabled: boolean;
    canAttach: boolean;
    isEditing: boolean;
    canSubmit: boolean;
    fileInputElement?: HTMLInputElement;
    effectiveTimezone?: string;
    showCreateThread?: boolean;
    createThread?: boolean;
    createThreadRequired?: boolean;
    onToggleCreateThread?: () => void;
    showAlsoSendToChannel?: boolean;
    echoToConversation?: boolean;
    alsoSendToChannel?: boolean;
    onToggleAlsoSendToChannel?: () => void;
    onsubmit: () => void;
  } = $props();
</script>

<div
  class="col-span-2 mb-1 flex shrink-0 items-center gap-1 justify-self-end"
  data-testid="composer-action-toolbar"
>
  <div class="flex items-center gap-0.5">
    {#if !isEditing && canAttach}
      <CompactActionButton
        label={m('composer.attach_file')}
        type="button"
        onclick={() => fileInputElement?.click()}
        disabled={inputDisabled}
        title={m('composer.attach_file')}
      >
        <span class="iconify icon-[uil--image-upload] text-[15px]"></span>
      </CompactActionButton>
    {/if}

    <ComposerTimestampPicker disabled={inputDisabled} {editorApi} {effectiveTimezone} />
  </div>

  <div class="flex items-center gap-0.5">
    {#if showCreateThread}
      <CompactActionButton
        label={m('composer.post_as_thread')}
        type="button"
        onpointerdown={(event) => event.preventDefault()}
        onclick={onToggleCreateThread}
        disabled={inputDisabled || createThreadRequired}
        aria-pressed={createThread}
        title={m('composer.post_as_thread')}
        class={[
          '@min-[560px]:gap-1',
          inputDisabled && 'opacity-50',
          createThread ? 'bg-action/10 text-action' : 'text-muted'
        ]}
      >
        <span class="iconify icon-[uil--comment-alt-lines] text-[15px]"></span>
        <span class="hidden @min-[560px]:inline">{m('composer.thread_label')}</span>
      </CompactActionButton>
    {/if}

    {#if showAlsoSendToChannel}
      <CompactActionButton
        label={m(
          echoToConversation
            ? 'composer.also_send_to_conversation'
            : 'composer.also_send_to_channel'
        )}
        type="button"
        onpointerdown={(event) => event.preventDefault()}
        onclick={onToggleAlsoSendToChannel}
        disabled={inputDisabled}
        aria-pressed={alsoSendToChannel}
        title={m(
          echoToConversation
            ? 'composer.also_send_to_conversation'
            : 'composer.also_send_to_channel'
        )}
        class={[
          '@min-[560px]:gap-1',
          alsoSendToChannel ? 'bg-action/10 text-action' : 'text-muted'
        ]}
      >
        <span class="iconify icon-[uil--megaphone] text-[15px]"></span>
        <span class="hidden @min-[560px]:inline">{m('composer.echo_label')}</span>
      </CompactActionButton>
    {/if}
  </div>

  <CompactActionButton
    label={m('composer.send')}
    type="button"
    onpointerdown={(event) => event.preventDefault()}
    onclick={onsubmit}
    disabled={!canSubmit}
    class="@min-[560px]:gap-1"
    title={m('composer.send')}
  >
    <span class="iconify icon-[uil--telegram-alt] text-[15px]"></span>
    <span class="hidden @min-[560px]:inline">{m('composer.send_label')}</span>
  </CompactActionButton>
</div>
