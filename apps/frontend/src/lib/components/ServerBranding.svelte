<!--
@component
Shared public server identity for authentication and server information pages.
The compact form, used in sign-in windows, shows only a short banner and the
description, clamped to two lines. The window bar of a sign-in window shows the
server's icon and name (`SignInWindowHeader`).
-->
<script lang="ts">
  import { InlineMarkdown, MarkdownHtml } from '$lib/ui';

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

  const welcomeHtml = $derived.by(() => {
    const body = welcomeMessage;
    return body ? import('$lib/markdown').then(({ renderMarkdown }) => renderMarkdown(body)) : null;
  });
</script>

<div
  class={[
    'mx-auto flex w-full max-w-md min-w-0 flex-col items-center',
    compact ? 'gap-2' : 'gap-5'
  ]}
>
  {#if compact}
    {#if bannerUrl}
      <!-- Sign-in windows come in many sizes: the banner follows the window
           height and gives its space to the form in short windows. -->
      <img
        src={bannerUrl}
        alt=""
        class="h-[clamp(4rem,18vh,8rem)] w-full rounded-lg object-cover [@media(max-height:540px)]:hidden"
      />
    {/if}
  {:else}
    <div class="flex items-center gap-4">
      {#if iconUrl}
        <img src={iconUrl} alt="" class="h-12 w-12 rounded-xl" />
      {/if}

      <svelte:element this={`h${headingLevel}`} class="text-2xl font-bold">
        <bdi>{name}</bdi>
      </svelte:element>
    </div>
  {/if}

  {#if bannerUrl && !compact}
    <img src={bannerUrl} alt="" class="aspect-[1200/630] w-full rounded-lg object-cover" />
  {/if}

  {#if description}
    <div class={['prose prose-center max-w-none text-muted', compact && 'line-clamp-2']} dir="auto">
      <InlineMarkdown text={description} multiline />
    </div>
  {/if}

  {#if welcomeMessage}
    <div class="prose w-full max-w-none rounded-lg border border-border bg-surface p-4 text-muted">
      {#await welcomeHtml}
        <p>{welcomeMessage}</p>
      {:then html}
        <MarkdownHtml html={html ?? ''} />
      {:catch}
        <p>{welcomeMessage}</p>
      {/await}
    </div>
  {/if}
</div>
