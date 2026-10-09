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
    inlineCode: false,
    heading: false,
    bulletList: false,
    orderedList: false,
    blockquote: false,
    codeBlock: false
  });
  let indentState = $state(emptyComposerIndentState);

  function ready(api: ComposerEditorApi) {
    editorApi = api;
    api.setContent(source);
  }
</script>

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
