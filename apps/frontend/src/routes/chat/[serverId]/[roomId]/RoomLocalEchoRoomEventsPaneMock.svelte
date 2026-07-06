<script lang="ts">
  import type { MessagesStore } from '$lib/state/room';

  let {
    roomId,
    messageStore
  }: {
    roomId: string;
    messageStore: MessagesStore;
  } = $props();

  $effect(() => {
    messageStore.setRoom(roomId);
  });

  const eventIds = $derived(messageStore.rootEvents.map((event) => event.id).join(','));
</script>

<output data-testid="room-event-ids">{eventIds}</output>
