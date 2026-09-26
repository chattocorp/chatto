<script lang="ts">
  import type { MessagesStore } from '$lib/state/room';

  let {
    messageStore,
    unreadAfterEventId = null,
    showStartMarker = true,
    emptyMessage = '',
    onReachedBottom
  }: {
    messageStore?: Pick<MessagesStore, 'isInitialLoading'>;
    unreadAfterEventId?: string | null;
    showStartMarker?: boolean;
    emptyMessage?: string;
    onReachedBottom?: () => void;
  } = $props();

  const isLoading = $derived(messageStore?.isInitialLoading ?? false);
</script>

<output data-testid="event-list-start-marker">{showStartMarker}</output>
{#if !isLoading}
  <output data-testid="event-list-empty-message">{emptyMessage}</output>
{/if}
<output data-testid="event-list-unread-after">{unreadAfterEventId ?? ''}</output>
<button type="button" data-testid="event-list-reached-bottom" onclick={onReachedBottom}>
  reached bottom
</button>
