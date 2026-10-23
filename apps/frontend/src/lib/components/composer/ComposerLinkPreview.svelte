<script lang="ts">
  import { parseMessageLink } from '$lib/chatLinks';
  import { LoadingFog } from '$lib/ui';
  import LazyMessagePreviewCard from '$lib/components/LazyMessagePreviewCard.svelte';
  import ComposerLinkPreviewCard from './ComposerLinkPreviewCard.svelte';
  import type { LinkPreviewState } from './linkPreviews.svelte';

  let { state }: { state: LinkPreviewState } = $props();
</script>

{#if state.activeURL}
  {@const url = state.activeURL}
  {@const messageLink = parseMessageLink(url)}
  {#if messageLink}
    <LazyMessagePreviewCard
      link={messageLink}
      compact
      onDismiss={() => state.dismissPreview(url)}
    />
  {:else if state.fetchingURLs.has(url)}
    <LoadingFog class="h-16 w-full max-w-md rounded-md" />
  {:else if state.previews.get(url)}
    <ComposerLinkPreviewCard
      preview={state.previews.get(url)!}
      onDismiss={() => state.dismissPreview(url)}
    />
  {/if}
{/if}
