<!-- @component Markdown source beside Chatto's real message rendering; no server is needed. -->
<script lang="ts">
  import { renderMarkdown } from '$lib/markdown';
  import { m } from '$lib/i18n/messages';
  import { MarkdownHtml, Hint } from '$lib/ui';
  let { source }: { source: string } = $props();
  const rendered = $derived(renderMarkdown(source));
</script>

<div class="my-6 grid min-w-0 overflow-hidden rounded-lg border border-border lg:grid-cols-2">
  <div class="min-w-0 bg-surface p-5">
    <div class="mb-3 text-sm font-semibold text-muted">Markdown</div>
    <pre dir="ltr" class="overflow-x-auto text-sm leading-relaxed"><code>{source}</code></pre>
  </div>
  <div class="min-w-0 border-t border-border p-5 lg:border-s lg:border-t-0">
    <div class="mb-3 text-sm font-semibold text-muted">{m('help.preview')}</div>
    {#await rendered then html}
      <div class="prose"><MarkdownHtml {html} /></div>
    {:catch}
      <Hint tone="danger">{m('common.error.network')}</Hint>
    {/await}
  </div>
</div>
