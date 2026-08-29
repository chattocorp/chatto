<!--
@component

Formatting-only shelf for the message composer. The owning composer controls
whether the shelf is visible and keeps message-level actions in its compact
input row.
-->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type {
    ComposerEditorApi,
    ComposerFormattingCommand,
    ComposerFormattingState,
    ComposerIndentState
  } from './editorTypes';

  let {
    id,
    formattingState,
    indentState,
    editorApi,
    inputDisabled
  }: {
    id: string;
    formattingState: ComposerFormattingState;
    indentState: ComposerIndentState;
    editorApi: ComposerEditorApi | null;
    inputDisabled: boolean;
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
	{id}
	class="composer-surface flex min-h-8 w-fit max-w-full items-center self-start overflow-hidden px-1 py-1"
	data-testid="composer-formatting-shelf"
>
  <div
    class="flex min-w-0 [scrollbar-width:none] flex-nowrap items-center gap-0.5 overflow-x-auto overscroll-x-contain [&::-webkit-scrollbar]:hidden"
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
          'flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded transition-[background-color,color,scale] duration-100 active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-50',
          active
            ? 'bg-surface-emphasized text-text'
            : 'text-muted enabled:hover:bg-surface-emphasized enabled:hover:text-text'
        ]}
      >
        <span class={['iconify text-[15px]', control.icon]}></span>
      </button>
      {#if control.command === 'orderedList'}
        <button
          type="button"
          onpointerdown={(event) => event.preventDefault()}
          onclick={() => editorApi?.adjustIndent('outdent')}
          disabled={inputDisabled || !editorApi || !indentState.canOutdent}
          aria-label={m('composer.format.outdent')}
          aria-keyshortcuts="Shift+Tab"
          title={m('composer.format.outdent')}
          class="flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] duration-100 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
        >
          <span class="iconify icon-[mdi--format-indent-decrease] text-[15px] rtl:scale-x-[-1]"
          ></span>
        </button>
        <button
          type="button"
          onpointerdown={(event) => event.preventDefault()}
          onclick={() => editorApi?.adjustIndent('indent')}
          disabled={inputDisabled || !editorApi || !indentState.canIndent}
          aria-label={m('composer.format.indent')}
          aria-keyshortcuts="Tab"
          title={m('composer.format.indent')}
          class="flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-muted transition-[background-color,color,scale] duration-100 active:scale-[0.96] enabled:hover:bg-surface-emphasized enabled:hover:text-text disabled:cursor-not-allowed disabled:opacity-50"
        >
          <span class="iconify icon-[mdi--format-indent-increase] text-[15px] rtl:scale-x-[-1]"
          ></span>
        </button>
      {/if}
    {/each}
  </div>
</div>
