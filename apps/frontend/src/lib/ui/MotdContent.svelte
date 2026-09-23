<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import MarkdownHtml from './MarkdownHtml.svelte';

  let { motd, onclick }: { motd: string; onclick: () => void } = $props();
  const html = $derived.by(() => {
    const body = motd;
    return import('$lib/markdown').then(({ renderInlineMarkdown }) => renderInlineMarkdown(body));
  });
</script>

<!-- @component Single-line MOTD preview. The owner opens the full message. -->
<div class="flex min-w-0 flex-1 justify-center">
  <div data-testid="motd-content" class="app-header-text-action max-w-full text-sm" dir="auto">
    <!-- Keep the modal trigger and Markdown links as separate controls. -->
    <button
      type="button"
      class="absolute inset-0 cursor-pointer rounded-lg focus-visible:outline-2 focus-visible:outline-action"
      aria-label={m('server_settings.motd_label')}
      aria-haspopup="dialog"
      {onclick}
    ></button>
    <div
      data-testid="motd-preview"
      class="prose prose-compact pointer-events-none min-w-0 max-w-none truncate [&_a]:pointer-events-auto [&_a]:relative"
    >
      {#await html}
        {motd}
      {:then html}
        <MarkdownHtml {html} />
      {:catch}
        {motd}
      {/await}
    </div>
  </div>
</div>
