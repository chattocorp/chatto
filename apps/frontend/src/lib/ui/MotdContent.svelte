<script lang="ts">
  import MarkdownHtml from './MarkdownHtml.svelte';

  let { motd }: { motd: string } = $props();

  let markdownModule: Promise<typeof import('$lib/markdown')> | null = null;

  function loadMarkdown() {
    markdownModule ??= import('$lib/markdown');
    return markdownModule;
  }
</script>

<span
  data-testid="motd-content"
  class="prose prose-compact max-w-none flex-1 truncate text-center text-sm"
>
  {#await loadMarkdown()}
    {motd}
  {:then { renderMarkdown }}
    {#await renderMarkdown(motd)}
      {motd}
    {:then html}
      <MarkdownHtml {html} />
    {/await}
  {:catch}
    {motd}
  {/await}
</span>
