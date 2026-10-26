<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MarkdownEditorField from './MarkdownEditorField.svelte';

  const { Story } = defineMeta({
    title: 'Composer/Markdown form field',
    component: MarkdownEditorField,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let markdown = $state('**Welcome!** Please check the pinned messages before posting.');
  let visual = $state('**Welcome!** Please check the pinned messages before posting.');
</script>

<Story name="Markdown" asChild>
  <MarkdownEditorField
    bind:value={markdown}
    editorKind="markdown"
    label="Room notice"
    placeholder="Write a notice"
    description="Corrections keep existing dismissals."
    maxlength={4000}
  />
</Story>

<Story name="Visual" asChild>
  <MarkdownEditorField
    bind:value={visual}
    editorKind="visual"
    label="Room notice"
    placeholder="Write a notice"
    description="Corrections keep existing dismissals."
    maxlength={4000}
  />
</Story>

<Story name="Disabled" asChild>
  <MarkdownEditorField
    value="Saving the **room notice**."
    editorKind="visual"
    label="Room notice"
    disabled
  />
</Story>
