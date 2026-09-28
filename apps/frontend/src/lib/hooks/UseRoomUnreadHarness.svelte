<script lang="ts">
  import { useRoomUnread } from './useRoomUnread.svelte';
  import type { UnreadMarkerEvent } from './useUnreadMarker.svelte';

  let {
    roomId,
    events = [],
    canReadMessages = true,
    onReady
  }: {
    roomId: string;
    events?: UnreadMarkerEvent[];
    canReadMessages?: boolean;
    onReady: (api: ReturnType<typeof useRoomUnread>) => void;
  } = $props();

  const unread = useRoomUnread(() => ({ roomId, events, canReadMessages }));

  $effect(() => {
    onReady(unread);
  });
</script>
