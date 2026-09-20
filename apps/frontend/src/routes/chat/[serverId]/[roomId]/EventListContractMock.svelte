<script lang="ts">
  let {
    unreadAfterEventId = null,
    showStartMarker = true,
    emptyMessage = '',
    isLoading = false,
    onReachedBottom
  }: {
    unreadAfterEventId?: string | null;
    showStartMarker?: boolean;
    emptyMessage?: string;
    isLoading?: boolean;
    onReachedBottom?: () => void;
  } = $props();
</script>

<output data-testid="event-list-start-marker">{showStartMarker}</output>
{#if !isLoading}
  <output data-testid="event-list-empty-message">{emptyMessage}</output>
{/if}
<output data-testid="event-list-unread-after">{unreadAfterEventId ?? ''}</output>
<button type="button" data-testid="event-list-reached-bottom" onclick={onReachedBottom}>
  reached bottom
</button>
