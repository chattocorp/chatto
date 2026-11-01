<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, waitFor } from 'storybook/test';
  import MarkdownEditorField from './MarkdownEditorField.svelte';
  import type { MarkdownMentionOptions } from './mentionOptions';

  const mentions: MarkdownMentionOptions = {
    members: [
      { id: 'bob', login: 'bob', displayName: 'Bob Example', avatarUrl: null, presenceStatus: 0 }
    ],
    roles: [{ name: 'moderators', pingable: true }]
  };

  const { Story } = defineMeta({
    title: 'Composer/Markdown form field',
    component: MarkdownEditorField,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let markdown = $state('**Welcome!** Please check the pinned messages before posting.');
  let visual = $state('**Welcome!** Please check the pinned messages before posting.');
  let renderedMermaid = $state('```mermaid rendered\nflowchart LR\nA[Draft] --> B[Publish]\n```');
  let mermaid = $state(
    '```mermaid\nflowchart TD\n  accTitle: Spaceport departure\n  %% Cargo inspection\n  subgraph Cargo\n    A[Scan cargo] --> B{Contraband?}\n    B -->|No| C[Seal the hold]\n    B -->|Yes| D[Pay a suspicious fee]\n    D --> C\n  end\n```'
  );
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

<Story name="Mermaid source" asChild>
  <MarkdownEditorField bind:value={mermaid} editorKind="markdown" label="Flight plan" />
</Story>

<Story name="Long SVG code" asChild>
  <MarkdownEditorField
    value={'```svg rendered\n<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 900 1000">\n' +
      Array.from(
        { length: 40 },
        (_, i) =>
          `<text x="482" y="${32 + i * 20}" fill="#f0f3ff" font-size="26" font-weight="600" id="${'long-svg-attribute'.repeat(8)}">The void has a little guy. No thoughts. Just bioluminescence.</text>`
      ).join('\n') +
      '\n</svg>\n```'}
    editorKind="visual"
    label="Scene source"
    maxlength={20000}
  />
</Story>

<Story
  name="Mermaid visual editor"
  asChild
  play={async ({ canvasElement }) => {
    // Wait for lazy highlighting so the accessibility scan checks its colours.
    await waitFor(() =>
      expect(canvasElement.querySelector('.hljs-comment')).toHaveTextContent('Cargo inspection')
    );
  }}
>
  <MarkdownEditorField bind:value={mermaid} editorKind="visual" label="Flight plan" />
</Story>

<Story name="Visual block" asChild>
  <MarkdownEditorField bind:value={renderedMermaid} editorKind="visual" label="Diagram" />
</Story>

<Story name="Member mentions" asChild>
  <MarkdownEditorField
    editorKind="markdown"
    label="Room notice"
    placeholder="Type @ to choose a member"
    {mentions}
  />
</Story>

<Story name="Visual member mentions" asChild>
  <MarkdownEditorField
    editorKind="visual"
    label="Room notice"
    placeholder="Type @ to choose a member"
    {mentions}
  />
</Story>
