<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TipTapEditor from './TipTapEditor.svelte';

  const { Story } = defineMeta({
    title: 'Composer/Visual editor',
    component: TipTapEditor,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import MarkdownEditor from './MarkdownEditor.svelte';
  import ComposerFormattingToolbar from './ComposerFormattingToolbar.svelte';
  import type { ComposerEditorApi, ComposerFormattingState } from './editorTypes';
  import { emptyComposerIndentState } from './editorTypes';

  let source = $state(
    'The release is ready[^checks]. Read the deployment note[^deploy]. The checks are complete[^checks].\n\n[^checks]: **All checks passed**, including the browser tests.\n\n[^deploy]: Deploy after the backup completes.'
  );
  let visual = $state(true);
  const ActiveEditor = $derived(visual ? TipTapEditor : MarkdownEditor);
  let editorApi = $state<ComposerEditorApi | null>(null);
  let formattingState = $state<ComposerFormattingState>({
    bold: false,
    italic: false,
    underline: false,
    inlineCode: false,
    heading: false,
    bulletList: false,
    orderedList: false,
    blockquote: false,
    codeBlock: false
  });
  let indentState = $state(emptyComposerIndentState);

  const quoteExample = `Text *marked with stars* and _marked with underscores_.

> Quoted text *marked with stars* and _marked with underscores_.`;
  const quoteSource = `${quoteExample}
>
> Second paragraph with **strong**, __also strong__, \`inline code\`, and a [link](https://example.com).
>
> > Nested quote with *emphasis* and **strong** text.
>
> - Bullet with _emphasis_
> - Bullet with **strong** text
>
> 1. Ordered item with *emphasis*
> 2. Ordered item with __strong__ text`;

  function ready(api: ComposerEditorApi) {
    editorApi = api;
    api.setContent(source);
  }
</script>

<Story name="Shared formatting" asChild>
  {@const preview =
    '# Heading\n**Strong** and *emphasis*, `code`, [link](https://example.com).\n> Quote with **strong**, *emphasis*, and `code`.'}
  <div class="flex w-xl max-w-full flex-col gap-3">
    <div class="rounded-md border border-border bg-surface p-3">
      <TipTapEditor placeholder="Visual formatting" onReady={(api) => api.setContent(preview)} />
    </div>
    <div class="rounded-md border border-border bg-surface p-3">
      <MarkdownEditor placeholder="Source formatting" onReady={(api) => api.setContent(preview)} />
    </div>
  </div>
</Story>

<Story name="Blockquote emphasis" asChild>
  <div class="flex w-xl max-w-full flex-col gap-3">
    <div class="rounded-md border border-border bg-surface p-3">
      <TipTapEditor placeholder="Write a quote" onReady={(api) => api.setContent(quoteSource)} />
    </div>
    <div class="rounded-md border border-border bg-surface p-3">
      <MarkdownEditor
        placeholder="Write quote Markdown"
        onReady={(api) => api.setContent(quoteExample)}
      />
    </div>
  </div>
</Story>

<Story name="Footnotes" asChild>
  <div class="flex w-xl max-w-full flex-col gap-3">
    <ComposerFormattingToolbar
      id="footnote-formatting"
      {formattingState}
      {indentState}
      {editorApi}
      inputDisabled={false}
      animated={false}
    />
    <div class="rounded-md border border-border bg-surface p-3">
      <ActiveEditor
        placeholder="Write a message"
        onReady={ready}
        onUpdate={(value) => (source = value)}
        onFormattingStateChange={(value) => (formattingState = value)}
        onIndentStateChange={(value) => (indentState = value)}
      />
    </div>
    <button type="button" class="btn self-start" onclick={() => (visual = !visual)}>
      {visual ? 'Switch to Markdown source' : 'Switch to visual editor'}
    </button>
  </div>
</Story>
