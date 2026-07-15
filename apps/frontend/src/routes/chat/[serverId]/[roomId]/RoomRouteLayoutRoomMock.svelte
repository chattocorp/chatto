<script lang="ts">
  let {
    roomId,
    threadId,
    routeMessageId
  }: { roomId: string; threadId?: string; routeMessageId?: string } = $props();
</script>

<div
  data-testid="room-layout-room"
  data-room-id={roomId}
  data-thread-id={threadId ?? ''}
  data-route-message-id={routeMessageId ?? ''}
></div>
