<script lang="ts">
  let { roomId, threadId }: { roomId: string; threadId?: string } = $props();
</script>

<div data-testid="room-layout-room" data-room-id={roomId} data-thread-id={threadId ?? ''}></div>
