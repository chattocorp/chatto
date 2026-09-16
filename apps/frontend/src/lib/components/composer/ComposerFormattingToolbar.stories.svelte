<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ComposerFormattingToolbar from './ComposerFormattingToolbar.svelte';

  const { Story } = defineMeta({
    title: 'Composer/Formatting toolbar',
    component: ComposerFormattingToolbar,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import type { ComposerEditorApi, ComposerFormattingState } from './editorTypes';

  let visible = $state(false);

  let formattingState = $state<ComposerFormattingState>({
    bold: true,
    italic: false,
    inlineCode: false,
    heading: false,
    bulletList: false,
    orderedList: false,
    blockquote: false,
    codeBlock: false
  });
  const editorApi: ComposerEditorApi = {
    getText: () => '',
    setContent: () => {},
    focus: () => {},
    performEnter: () => {},
    getTextBeforeCursor: () => '',
    isInCodeBlock: () => false,
    replaceTextBeforeCursor: () => {},
    insertText: () => {},
    toggleFormatting: (command) => (formattingState[command] = !formattingState[command]),
    adjustIndent: () => true,
    insertQuote: () => {}
  };
</script>

<Story name="Reveal motion" asChild>
  <div class="flex w-80 flex-col items-start gap-1">
    {#if visible}
      <ComposerFormattingToolbar
        id="formatting-motion"
        {formattingState}
        {editorApi}
        indentState={{ canIndent: true, canOutdent: false }}
        inputDisabled={false}
      />
    {/if}
    <button class="btn" onclick={() => (visible = !visible)} aria-expanded={visible}>
      Toggle formatting
    </button>
  </div>
</Story>

<Story name="Compact" asChild>
  <ComposerFormattingToolbar
    id="formatting-preview"
    {formattingState}
    {editorApi}
    indentState={{ canIndent: true, canOutdent: false }}
    inputDisabled={false}
  />
</Story>

<Story name="Narrow" asChild>
  <div class="w-56">
    <ComposerFormattingToolbar
      id="formatting-narrow"
      {formattingState}
      {editorApi}
      indentState={{ canIndent: true, canOutdent: false }}
      inputDisabled={false}
    />
  </div>
</Story>

<Story name="Disabled" asChild>
  <ComposerFormattingToolbar
    id="formatting-disabled"
    {formattingState}
    {editorApi}
    indentState={{ canIndent: false, canOutdent: false }}
    inputDisabled
  />
</Story>
