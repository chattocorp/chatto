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
    canMarkAsRead = true,
    onReady
  }: {
    targetId: string;
    markAsRead: (
      targetId: string,
      upToEventId: string | undefined,
      signal: AbortSignal
    ) => Promise<ReadResult>;
    events?: UnreadMarkerEvent[];
    skipActorId?: string | null;
    canMarkAsRead?: boolean;
    onReady: (api: UnreadMarkerHarnessAPI) => void;
  } = $props();

  const unread = useUnreadMarker(() => targetId, {
    markAsRead: (target, upToEventId, signal) => markAsRead(target, upToEventId, signal),
    markerWindowFromReadResult: (result, markedAtMs): UnreadMarkerWindow | null => {
      if (!result.previousLastReadAt || !result.lastReadAt) return null;
      if (result.previousLastReadAt === result.lastReadAt) return null;
      return {
        afterTime: result.previousLastReadAt,
        beforeTime: markedAtMs
      };
    },
    getMarkerEvents: () => events,
    getMarkerSkipActorId: () => skipActorId,
    canMarkAsRead: () => canMarkAsRead
  });

  $effect(() => {
    onReady(unread);
  });
</script>

<button type="button">Interaction target</button>
