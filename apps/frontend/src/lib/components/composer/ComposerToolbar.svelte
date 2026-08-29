<!--
@component

Compact message-level actions for the composer input row. Formatting commands
live in `ComposerFormattingToolbar` so this row can stay aligned with the
48-pixel app-shell controls.
-->
<script lang="ts">
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
    alsoSendToChannel?: boolean;
    onToggleAlsoSendToChannel?: () => void;
    onsubmit: () => void;
  } = $props();
</script>

<div class="mb-1.5 flex shrink-0 items-center gap-1" data-testid="composer-action-toolbar">
  <div class="flex items-center gap-0.5">
    {#if !isEditing && canAttach}
      <button
        type="button"
        onclick={() => fileInputElement?.click()}
        disabled={inputDisabled}
        class="flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-muted transition-[color,scale] duration-100 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
        aria-label={m('composer.attach_file')}
        title={m('composer.attach_file')}
      >
        <span class="iconify icon-[uil--image-upload] text-[15px]"></span>
      </button>
    {/if}

    <ComposerTimestampPicker disabled={inputDisabled} {editorApi} {effectiveTimezone} />
  </div>

  <div class="flex items-center gap-0.5">
    {#if showCreateThread}
      <button
        type="button"
        onpointerdown={(event) => event.preventDefault()}
        onclick={onToggleCreateThread}
        disabled={inputDisabled || createThreadRequired}
        aria-label={m('composer.post_as_thread')}
        aria-pressed={createThread}
        title={m('composer.post_as_thread')}
        class={[
          'flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-xs font-medium transition-[background-color,color] duration-100 disabled:cursor-not-allowed @min-[560px]:w-auto @min-[560px]:gap-1 @min-[560px]:px-1.5',
          inputDisabled && 'opacity-50',
          createThread
            ? 'bg-action/10 text-action'
            : 'text-muted enabled:hover:bg-surface-emphasized enabled:hover:text-text'
        ]}
      >
        <span class="iconify icon-[uil--comment-alt-lines] text-[15px]"></span>
        <span class="hidden @min-[560px]:inline">{m('composer.thread_label')}</span>
      </button>
    {/if}

    {#if showAlsoSendToChannel}
      <button
        type="button"
        onpointerdown={(event) => event.preventDefault()}
        onclick={onToggleAlsoSendToChannel}
        disabled={inputDisabled}
        aria-label={m('composer.also_send_to_channel')}
        aria-pressed={alsoSendToChannel}
        title={m('composer.also_send_to_channel')}
        class={[
          'flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-xs font-medium transition-[background-color,color] duration-100 disabled:cursor-not-allowed disabled:opacity-50 @min-[560px]:w-auto @min-[560px]:gap-1 @min-[560px]:px-1.5',
          alsoSendToChannel
            ? 'bg-action/10 text-action'
            : 'text-muted enabled:hover:bg-surface-emphasized enabled:hover:text-text'
        ]}
      >
        <span class="iconify icon-[uil--megaphone] text-[15px]"></span>
        <span class="hidden @min-[560px]:inline">{m('composer.echo_label')}</span>
      </button>
    {/if}
  </div>

  <button
    type="button"
    onpointerdown={(event) => event.preventDefault()}
    onclick={onsubmit}
    disabled={!canSubmit}
    class="flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-xs font-medium text-muted transition-[background-color,color,scale] duration-100 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50 @min-[560px]:w-auto @min-[560px]:gap-1 @min-[560px]:px-1.5"
    aria-label={m('composer.send')}
    title={m('composer.send')}
  >
    <span class="iconify icon-[uil--telegram-alt] text-[15px]"></span>
    <span class="hidden @min-[560px]:inline">{m('composer.send_label')}</span>
  </button>
</div>
