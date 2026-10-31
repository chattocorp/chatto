<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import RenderedImage from './RenderedImage.svelte';
  const { Story } = defineMeta({ title: 'UI/RenderedImage', component: RenderedImage });
</script>

<script lang="ts">
  import MarkdownHtml from './MarkdownHtml.svelte';
  import { renderMarkdown } from '$lib/markdown';
  const source = 'flowchart LR\nA[Launch checks] --> B{Ready?}\nB -->|Yes| C[Depart]\nB -->|No| A';
  const html = renderMarkdown(
    `Code example:\n\n\u0060\u0060\u0060mermaid\n${source}\n\u0060\u0060\u0060\n\nVisual message:\n\n\u0060\u0060\u0060mermaid rendered\n${source}\n\u0060\u0060\u0060`
  );
</script>

<Story name="Code and visual intent" asChild>
  <div class="prose w-full max-w-2xl">
    {#await html then value}<MarkdownHtml html={value} />{/await}
  </div>
</Story>
