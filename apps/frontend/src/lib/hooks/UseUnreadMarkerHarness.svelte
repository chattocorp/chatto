<script lang="ts">
  import {
    useUnreadMarker,
    type UnreadMarkerEvent,
    type UnreadMarkerWindow
  } from './useUnreadMarker.svelte';

  type ReadResult = {
    lastReadAt: string | null;
    previousLastReadAt: string | null;
  };

  type UnreadMarkerHarnessAPI = ReturnType<typeof useUnreadMarker<ReadResult>>;

  let {
    targetId,
    markAsRead,
    events = [],
    skipActorId = null,
    onReady
  }: {
    targetId: string;
    markAsRead: (targetId: string, upToEventId?: string) => Promise<ReadResult | null>;
    events?: UnreadMarkerEvent[];
    skipActorId?: string | null;
    onReady: (api: UnreadMarkerHarnessAPI) => void;
  } = $props();

  const unread = useUnreadMarker(() => targetId, {
    markAsRead: (target, upToEventId) => markAsRead(target, upToEventId),
    markerWindowFromReadResult: (result, markedAtMs): UnreadMarkerWindow | null => {
      if (!result.previousLastReadAt || !result.lastReadAt) return null;
      if (result.previousLastReadAt === result.lastReadAt) return null;
      return {
        afterTime: result.previousLastReadAt,
        beforeTime: markedAtMs
      };
    },
    getMarkerEvents: () => events,
    getMarkerSkipActorId: () => skipActorId
  });

  $effect(() => {
    onReady(unread);
  });
</script>
