<!--
@component

Message-level actions that wrap below the editor in narrow composer containers.
Formatting commands live in `ComposerFormattingToolbar`.
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
  class="flex min-w-0 flex-wrap items-center justify-between gap-1 @min-[560px]/composer:desktop-presentation:mb-1 @min-[560px]/composer:shrink-0 @min-[560px]/composer:flex-nowrap"
  data-testid="composer-action-toolbar"
>
  <div class="flex items-center gap-0.5">
    {#if !isEditing && canAttach}
      <CompactActionButton
        wrapperClass="mobile-presentation:pill-button-group-touch"
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

  <div class="ms-auto flex max-w-full flex-wrap items-center justify-end gap-0.5 @min-[560px]/composer:flex-nowrap">
    {#if showCreateThread}
      <CompactActionButton
        wrapperClass="mobile-presentation:pill-button-group-touch"
        label={m('composer.post_as_thread')}
        type="button"
        onpointerdown={(event) => event.preventDefault()}
        onclick={onToggleCreateThread}
        disabled={inputDisabled || createThreadRequired}
        aria-pressed={createThread}
        title={m('composer.post_as_thread')}
        class={[
          '@min-[560px]/composer:gap-1',
          inputDisabled && 'opacity-50',
          createThread ? 'bg-action/10 text-action' : 'text-muted'
        ]}
      >
        <span class="iconify icon-[uil--comment-alt-lines] text-[15px]"></span>
        <span class="hidden @min-[560px]/composer:inline">{m('composer.thread_label')}</span>
      </CompactActionButton>
    {/if}

    {#if showAlsoSendToChannel}
      <CompactActionButton
        wrapperClass="mobile-presentation:pill-button-group-touch"
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
          '@min-[560px]/composer:gap-1',
          alsoSendToChannel ? 'bg-action/10 text-action' : 'text-muted'
        ]}
      >
        <span class="iconify icon-[uil--megaphone] text-[15px]"></span>
        <span class="hidden @min-[560px]/composer:inline">{m('composer.echo_label')}</span>
      </CompactActionButton>
    {/if}
    <CompactActionButton
      wrapperClass="mobile-presentation:pill-button-group-touch"
      label={m('composer.send')}
      type="button"
      onpointerdown={(event) => event.preventDefault()}
      onclick={onsubmit}
      disabled={!canSubmit}
      class="@min-[560px]/composer:gap-1"
      title={m('composer.send')}
    >
      <span class="iconify icon-[uil--telegram-alt] text-[15px]"></span>
      <span class="hidden @min-[560px]/composer:inline">{m('composer.send_label')}</span>
    </CompactActionButton>
  </div>
</div>
