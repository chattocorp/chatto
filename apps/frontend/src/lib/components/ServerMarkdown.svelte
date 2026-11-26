<!--
@component

Server-authored Markdown, such as the welcome message or the message of the
day, rendered as prose. The plain text shows while the renderer loads and if
it fails.
-->
<script lang="ts">
  import { MarkdownHtml } from '$lib/ui';

  let { source }: { source: string } = $props();

  const html = $derived.by(() => {
    const body = source;
    return import('$lib/markdown').then(({ renderMarkdown }) => renderMarkdown(body));
  });
</script>

<div class="prose max-w-none wrap-anywhere" dir="auto">
  {#await html}
    <p class="whitespace-pre-wrap">{source}</p>
  {:then html}
    <MarkdownHtml {html} />
  {:catch}
    <p class="whitespace-pre-wrap">{source}</p>
  {/await}
</div>
