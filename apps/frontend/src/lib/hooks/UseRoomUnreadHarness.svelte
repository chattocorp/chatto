<script lang="ts">
  import { useRoomUnread } from './useRoomUnread.svelte';

  let {
    roomId,
    canReadMessages = true,
    onReady
  }: {
    roomId: string;
    canReadMessages?: boolean;
    onReady: (api: ReturnType<typeof useRoomUnread>) => void;
  } = $props();

  const unread = useRoomUnread(() => ({ roomId, events: [], canReadMessages }));

  $effect(() => {
    onReady(unread);
  });
</script>
