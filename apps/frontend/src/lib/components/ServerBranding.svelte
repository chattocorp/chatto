<!-- @component Shared public server identity for authentication and server information pages. -->
<script lang="ts">
  import { MarkdownHtml } from '$lib/ui';

  let {
    name,
    iconUrl = null,
    bannerUrl = null,
    description = null,
    welcomeMessage = null,
    compact = false,
    headingLevel = 3
  }: {
    name: string;
    iconUrl?: string | null;
    bannerUrl?: string | null;
    description?: string | null;
    welcomeMessage?: string | null;
    compact?: boolean;
    /** Match the containing page's heading hierarchy without changing the visual style. */
    headingLevel?: 2 | 3;
  } = $props();

  let markdownModule: Promise<typeof import('$lib/markdown')> | null = null;

  function loadMarkdown() {
    markdownModule ??= import('$lib/markdown');
    return markdownModule;
  }
</script>

<div class={['flex flex-col items-center', compact ? 'gap-3' : 'gap-5']}>
  <div class={['flex items-center', compact ? 'gap-3' : 'gap-4']}>
    {#if iconUrl}
      <img src={iconUrl} alt="" class={compact ? 'h-10 w-10 rounded-lg' : 'h-12 w-12 rounded-xl'} />
    {/if}

    <svelte:element
      this={`h${headingLevel}`}
      class={compact ? 'text-lg font-bold' : 'text-2xl font-bold'}
    >
      <bdi>{name}</bdi>
    </svelte:element>
  </div>

  {#if bannerUrl}
    <img src={bannerUrl} alt="" class="aspect-[1200/630] w-full rounded-lg object-cover" />
  {/if}

  {#if description}
    <p class="text-center text-muted">{description}</p>
  {/if}

  {#if welcomeMessage}
    <div class="prose w-full max-w-none rounded-lg border border-border bg-surface p-4 text-muted">
      {#await loadMarkdown()}
        <p>{welcomeMessage}</p>
      {:then { renderMarkdown }}
        {#await renderMarkdown(welcomeMessage)}
          <p>{welcomeMessage}</p>
        {:then html}
          <MarkdownHtml {html} />
        {/await}
      {:catch}
        <p>{welcomeMessage}</p>
      {/await}
    </div>
  {/if}
</div>
