<script lang="ts">
  let {
    unreadAfterEventId = null,
    onReachedBottom
  }: {
    unreadAfterEventId?: string | null;
    onReachedBottom?: () => void;
  } = $props();
</script>

<output data-testid="event-list-unread-after">{unreadAfterEventId ?? ''}</output>
<button type="button" data-testid="event-list-reached-bottom" onclick={onReachedBottom}>
  reached bottom
</button>
