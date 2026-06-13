<!--
@component

Test-only wrapper for `RoomSidebar`. Creates the room-member context through
the real sync hook so browser specs can exercise pagination wiring without
mounting the full chat room shell.
-->
<script lang="ts">
  import { useRoomMembersSync } from '$lib/hooks/useRoomMembersSync.svelte';
  import type { RoomData } from '$lib/hooks/useRoomData.svelte';
  import RoomSidebar from './RoomSidebar.svelte';

  let {
    roomId = 'room-1',
    roomData,
    currentUserId = 'viewer'
  }: {
    roomId?: string;
    roomData: RoomData;
    currentUserId?: string | null;
  } = $props();

  const roomMembers = useRoomMembersSync(() => ({
    roomId,
    isDM: false,
    roomData,
    dmData: null
  }));
</script>

<RoomSidebar
  {roomId}
  loading={false}
  canBanRoomMembers={false}
  {currentUserId}
  onLoadMoreMembers={roomMembers.loadMoreMembers}
/>
