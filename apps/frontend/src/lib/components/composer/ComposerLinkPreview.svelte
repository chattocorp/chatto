<script lang="ts">
  import { parseMessageLink } from '$lib/messageLinks';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
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
    <div aria-busy="true"></div>
  {:else if state.previews.get(url)}
    <LinkPreviewCard
      preview={state.previews.get(url)!}
      onDismiss={() => state.dismissPreview(url)}
    />
  {/if}
{/if}
