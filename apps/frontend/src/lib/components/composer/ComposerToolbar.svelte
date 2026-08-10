<script lang="ts">
  import { prefersTouchActions } from '$lib/utils/inputCapabilities';
  import { m } from '$lib/i18n/messages';
  import ComposerTimestampPicker from './ComposerTimestampPicker.svelte';
  import type {
    ComposerFormattingCommand,
    ComposerFormattingState,
    TipTapEditorApi
  } from './editorTypes';

  let {
    formattingState,
    editorApi,
    inputDisabled,
    canAttach,
    isEditing,
    canSubmit,
    isRichComposer,
    nextEnterWillSend,
    fileInputElement,
    effectiveTimezone,
    showCreateThread = false,
    createThread = false,
    onToggleCreateThread = () => {},
    showAlsoSendToChannel = false,
    alsoSendToChannel = false,
    onToggleAlsoSendToChannel = () => {},
    onsubmit
  }: {
    formattingState: ComposerFormattingState;
    editorApi: TipTapEditorApi | null;
    inputDisabled: boolean;
    canAttach: boolean;
    isEditing: boolean;
    canSubmit: boolean;
    isRichComposer: boolean;
    nextEnterWillSend: boolean;
    fileInputElement?: HTMLInputElement;
    effectiveTimezone?: string;
    showCreateThread?: boolean;
    createThread?: boolean;
    onToggleCreateThread?: () => void;
    showAlsoSendToChannel?: boolean;
    alsoSendToChannel?: boolean;
    onToggleAlsoSendToChannel?: () => void;
    onsubmit: () => void;
  } = $props();

  const formattingControls: {
    command: ComposerFormattingCommand;
    icon: string;
  }[] = [
    { command: 'bold', icon: 'icon-[mdi--format-bold]' },
    { command: 'italic', icon: 'icon-[mdi--format-italic]' },
    { command: 'inlineCode', icon: 'icon-[mdi--code-tags]' },
    { command: 'heading', icon: 'icon-[mdi--format-header-2]' },
    { command: 'bulletList', icon: 'icon-[mdi--format-list-bulleted]' },
    { command: 'orderedList', icon: 'icon-[mdi--format-list-numbered]' },
    { command: 'blockquote', icon: 'icon-[mdi--format-quote-open]' },
    { command: 'codeBlock', icon: 'icon-[mdi--code-block-braces]' }
  ];
  const shortcutHints = getShortcutHints();
  const submitHint = $derived(
    shortcutHints && isRichComposer
      ? nextEnterWillSend
        ? shortcutHints.enterAgain
        : shortcutHints.submit
      : null
  );

  function getShortcutHints(): { submit: string; enterAgain: string } | null {
    if (typeof navigator === 'undefined' || prefersTouchActions()) return null;

    const userAgentDataPlatform =
      'userAgentData' in navigator
        ? (navigator.userAgentData as { platform?: string } | undefined)?.platform
        : undefined;
    const platform = userAgentDataPlatform ?? navigator.platform ?? '';
    const usesReturn = /Mac|iPhone|iPad|iPod/i.test(platform);
    return usesReturn
      ? { submit: 'Cmd+Return to Send', enterAgain: 'Return again to Send' }
      : { submit: 'Ctrl+Return to Send', enterAgain: 'Enter again to Send' };
  }

  function formattingLabel(command: ComposerFormattingCommand): string {
    switch (command) {
      case 'bold':
        return m('composer.format.bold');
      case 'italic':
        return m('composer.format.italic');
      case 'inlineCode':
        return m('composer.format.inline_code');
      case 'heading':
        return m('composer.format.heading');
      case 'bulletList':
        return m('composer.format.bullet_list');
      case 'orderedList':
        return m('composer.format.ordered_list');
      case 'blockquote':
        return m('composer.format.blockquote');
      case 'codeBlock':
        return m('composer.format.code_block');
    }
  }
</script>

<div
  class="mt-0 flex min-h-7 items-center justify-between gap-2 border-t border-border/60 pt-0.5"
  data-testid="composer-toolbar"
>
  <div class="flex items-center gap-1">
    <div
      class="flex min-w-0 flex-nowrap items-center gap-0.5"
      data-testid="composer-formatting-toolbar"
    >
      {#each formattingControls as control (control.command)}
        {@const label = formattingLabel(control.command)}
        {@const active = formattingState[control.command]}
        <button
          type="button"
          onpointerdown={(event) => event.preventDefault()}
          onclick={() => editorApi?.toggleFormatting(control.command)}
          disabled={inputDisabled || !editorApi}
          aria-label={label}
          aria-pressed={active}
          title={label}
          class={[
            'flex h-6 w-6 cursor-pointer items-center justify-center rounded transition-[background-color,color,scale] duration-100 active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-50',
            active
              ? 'bg-surface-emphasized text-text'
              : 'text-muted enabled:hover:bg-surface-emphasized enabled:hover:text-text'
          ]}
        >
          <span class={['iconify text-[15px]', control.icon]}></span>
        </button>
      {/each}
    </div>

    <div class="mx-1 h-4 w-px bg-border/60"></div>

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

  <div class="flex items-center gap-2">
    <div class="flex items-center gap-0.5">
      {#if showCreateThread}
        <button
          type="button"
          onpointerdown={(event) => event.preventDefault()}
          onclick={onToggleCreateThread}
          disabled={inputDisabled}
          aria-label={m('composer.post_as_thread')}
          aria-pressed={createThread}
          title={m('composer.post_as_thread')}
          class={[
            'flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-xs font-medium transition-[background-color,color] duration-100 @min-[560px]:w-auto @min-[560px]:gap-1 @min-[560px]:px-1.5 disabled:cursor-not-allowed disabled:opacity-50',
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
            'flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-xs font-medium transition-[background-color,color] duration-100 @min-[560px]:w-auto @min-[560px]:gap-1 @min-[560px]:px-1.5 disabled:cursor-not-allowed disabled:opacity-50',
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

    {#if submitHint && canSubmit}
      <span
        aria-hidden="true"
        title={submitHint}
        class="px-0.5 text-xs leading-none font-medium whitespace-nowrap text-muted/75"
      >
        {submitHint}
      </span>
    {/if}

    <button
      type="button"
      onpointerdown={(event) => event.preventDefault()}
      onclick={onsubmit}
      disabled={!canSubmit}
      class="flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-xs font-medium text-muted transition-[background-color,color,scale] duration-100 @min-[560px]:w-auto @min-[560px]:gap-1 @min-[560px]:px-1.5 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
      aria-label={m('composer.send')}
      title={isRichComposer ? m('composer.send_ctrl_enter') : m('composer.send_enter')}
    >
      <span class="iconify icon-[uil--telegram-alt] text-[15px]"></span>
      <span class="hidden @min-[560px]:inline">{m('composer.send_label')}</span>
    </button>
  </div>
</div>
