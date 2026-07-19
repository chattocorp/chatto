<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MarkdownHtml from './MarkdownHtml.svelte';

  const { Story } = defineMeta({
    title: 'UI/MarkdownHtml',
    component: MarkdownHtml,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  import { renderMarkdown } from '$lib/markdown';

  const tableHtml = renderMarkdown(`| Release | Status | Owner | Compatibility | Notes |
| :--- | :---: | ---: | :--- | :--- |
| 0.4.8 | **Stable** | Core team | Server and bundled client | Current production release |
| 0.5.0-alpha.1 | Testing | Contributors | Mixed-version clients | Previewing the next release |`);
</script>

<Story name="GFM table" asChild>
  <div class="w-80 rounded-md border border-border bg-surface p-3">
    <div class="prose max-w-none min-w-0">
      {#await tableHtml}
        <span class="text-muted">Rendering table…</span>
      {:then html}
        <MarkdownHtml {html} />
      {/await}
    </div>
  </div>
</Story>
