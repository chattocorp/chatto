<script lang="ts">
  /**
   * Test-only wrapper that exposes `useRoomData` to a test through a callback.
   * Used by `useRoomData.svelte.spec.ts`.
   */
  import { useRoomData } from './useRoomData.svelte';

  type Snapshot = {
    roomData: ReturnType<typeof useRoomData>['roomData'];
    isRoomLoading: boolean;
  };

  let {
    spaceId,
    roomId,
    onSnapshot
  }: {
    spaceId: string;
    roomId: string;
    onSnapshot: (snapshot: Snapshot) => void;
  } = $props();

  const room = useRoomData(() => ({ spaceId, roomId }));

  $effect(() => {
    onSnapshot({ roomData: room.roomData, isRoomLoading: room.isRoomLoading });
  });
</script>
