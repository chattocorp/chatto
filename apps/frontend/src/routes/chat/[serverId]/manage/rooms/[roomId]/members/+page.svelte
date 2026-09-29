<!--
@component

Members section of a room: the room members, and adding or removing members
for viewers who can manage the room.
-->
<script lang="ts">
  import RoomMembersPanel from '../RoomMembersPanel.svelte';
  import { useRoomManagement } from '../roomManagementContext';

  const detail = useRoomManagement();
</script>

{#key detail.roomId}
  <RoomMembersPanel
    roomId={detail.roomId}
    roomName={detail.room.name}
    isUniversal={detail.room.isUniversal}
    archived={detail.room.archived}
    canManageMembers={detail.canManageRoom}
    scrollRoot={detail.scrollContainer}
  />
{/key}
