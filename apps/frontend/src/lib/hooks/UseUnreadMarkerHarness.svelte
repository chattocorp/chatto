<script lang="ts">
  import { useUnreadMarker, type UnreadMarkerWindow } from './useUnreadMarker.svelte';

  type ReadResult = {
    lastReadAt: string | null;
    previousLastReadAt: string | null;
  };

  type UnreadMarkerHarnessAPI = ReturnType<typeof useUnreadMarker<ReadResult>>;

  let {
    targetId,
    markAsRead,
    onReady
  }: {
    targetId: string;
    markAsRead: (targetId: string, upToEventId?: string) => Promise<ReadResult | null>;
    onReady: (api: UnreadMarkerHarnessAPI) => void;
  } = $props();

  const unread = useUnreadMarker(() => targetId, {
    markAsRead: (target, upToEventId) => markAsRead(target, upToEventId),
    markerWindowFromReadResult: (result): UnreadMarkerWindow | null => {
      if (!result.previousLastReadAt || !result.lastReadAt) return null;
      return {
        afterTime: result.previousLastReadAt,
        beforeTime: result.lastReadAt
      };
    }
  });

  $effect(() => {
    onReady(unread);
  });
</script>
