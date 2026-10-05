<script lang="ts">
  import { useRoomUnread } from './useRoomUnread.svelte';
  import type { UnreadMarkerEvent } from './useUnreadMarker.svelte';

  let {
    roomId,
    events = [],
    canReadMessages = true,
    lifecycleUpToEventId,
    onReady
  }: {
    roomId: string;
    events?: UnreadMarkerEvent[];
    canReadMessages?: boolean;
    /** Limits the entry read to this event when set. */
    lifecycleUpToEventId?: string;
    onReady: (api: ReturnType<typeof useRoomUnread>) => void;
  } = $props();

  const unread = useRoomUnread(() => ({
    roomId,
    events,
    canReadMessages,
    getLifecycleUpToEventId: () => lifecycleUpToEventId
  }));

  $effect(() => {
    onReady(unread);
  });
</script>
