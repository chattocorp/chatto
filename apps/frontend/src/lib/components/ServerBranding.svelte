<script lang="ts">
  import MarkdownHtml from '$lib/ui/MarkdownHtml.svelte';

  let {
    name,
    iconUrl = null,
    bannerUrl = null,
    description = null,
    welcomeMessage = null
  }: {
    name: string;
    iconUrl?: string | null;
    bannerUrl?: string | null;
    description?: string | null;
    welcomeMessage?: string | null;
  } = $props();

  let markdownModule: Promise<typeof import('$lib/markdown')> | null = null;

  function loadMarkdown() {
    markdownModule ??= import('$lib/markdown');
    return markdownModule;
  }
</script>

<div class="flex flex-col items-center gap-5">
  <div class="flex items-center gap-4">
    {#if iconUrl}
      <img src={iconUrl} alt="" class="h-12 w-12 rounded-xl" />
    {/if}

    <h3 class="text-2xl font-bold">{name}</h3>
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
