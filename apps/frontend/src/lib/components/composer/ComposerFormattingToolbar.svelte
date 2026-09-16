<!--
@component

Formatting-only shelf for the message composer. The owning composer controls
whether the shelf is visible and keeps message-level actions in its compact
input row.
-->
<script lang="ts">
  import PillButtonGroup from '$lib/ui/PillButtonGroup.svelte';
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

  const formattingGroups: {
    command: ComposerFormattingCommand;
    icon: string;
  }[][] = [
    [
      { command: 'bold', icon: 'icon-[mdi--format-bold]' },
      { command: 'italic', icon: 'icon-[mdi--format-italic]' },
      { command: 'inlineCode', icon: 'icon-[mdi--code-tags]' }
    ],
    [
      { command: 'heading', icon: 'icon-[mdi--format-header-2]' },
      { command: 'blockquote', icon: 'icon-[mdi--format-quote-open]' },
      { command: 'codeBlock', icon: 'icon-[mdi--code-block-braces]' }
    ],
    [
      { command: 'bulletList', icon: 'icon-[mdi--format-list-bulleted]' },
      { command: 'orderedList', icon: 'icon-[mdi--format-list-numbered]' }
    ]
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

<div {id} class="w-fit max-w-full self-start" data-testid="composer-formatting-shelf">
  <div
    class="flex min-w-0 [scrollbar-width:none] gap-1.5 overflow-x-auto overscroll-x-contain [&::-webkit-scrollbar]:hidden"
    data-testid="composer-formatting-toolbar"
  >
    {#each formattingGroups as formattingControls (formattingControls[0].command)}
      <PillButtonGroup
        compact
        gaps={false}
        label={m('composer.formatting_options')}
        class="w-max shrink-0"
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
            class="pill-button"
          >
            <span class={['iconify', control.icon]}></span>
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
              class="pill-button"
            >
              <span class="iconify icon-[mdi--format-indent-decrease] rtl:scale-x-[-1]"
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
              class="pill-button"
            >
              <span class="iconify icon-[mdi--format-indent-increase] rtl:scale-x-[-1]"
              ></span>
            </button>
          {/if}
        {/each}
      </PillButtonGroup>
    {/each}
  </div>
</div>
