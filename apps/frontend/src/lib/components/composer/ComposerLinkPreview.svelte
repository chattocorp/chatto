<script lang="ts">
  import { parseMessageLink } from '$lib/messageLinks';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
  import MessagePreviewCard from '$lib/components/MessagePreviewCard.svelte';
  import type { LinkPreviewState } from './linkPreviews.svelte';

  let { state }: { state: LinkPreviewState } = $props();
</script>

{#if state.activeURL}
  {@const url = state.activeURL}
  {@const messageLink = parseMessageLink(url)}
  {#if messageLink}
    <MessagePreviewCard link={messageLink} onDismiss={() => state.dismissPreview(url)} />
  {:else if state.fetchingURLs.has(url)}
    <LoadingFog class="h-24 w-full" />
  {:else if state.previews.get(url)}
    <LinkPreviewCard
      preview={state.previews.get(url)!}
      onDismiss={() => state.dismissPreview(url)}
    />
  {/if}
{/if}
