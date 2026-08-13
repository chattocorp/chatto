<!--
@component

Test-only wrapper around `RoomDirectory`. Constructs a real command store over
a fixture navigation view so component tests do not need a realtime transport.
-->
<script lang="ts">
  import { RoomKind } from '$lib/api-client/roomDirectory';
  import type { RoomsListItem, RoomsListGroup } from '$lib/state/server/rooms.svelte';
  import { RoomDirectoryStore, type DirectoryRoom } from '$lib/state/server/roomDirectory.svelte';
  import RoomDirectory from './RoomDirectory.svelte';

  let {
    initialRooms,
    joinedRooms = [],
    roomGroups = null
  }: {
    initialRooms: DirectoryRoom[];
    joinedRooms?: RoomsListItem[];
    roomGroups?: RoomsListGroup[] | null;
  } = $props();

  const stubRoomAPI = {
    joinRoom: async () => null,
    leaveRoom: async () => true,
    joinGroup: async () => []
  };
  const stubMemberDirectoryAPI = {
    listRoomMembers: async () => ({ members: [], totalCount: 0, hasMore: false })
  };

  const navigation = {
    get rooms() {
      const joinedById = new Map(joinedRooms.map((room) => [room.id, room]));
      return initialRooms
        .filter((room) => !room.archived)
        .map((room): RoomsListItem => {
          const listed = joinedById.get(room.id);
          return {
            ...room,
            type: RoomKind.CHANNEL,
            viewerIsMember: listed?.viewerIsMember ?? false,
            viewerCanManageRoom: listed?.viewerCanManageRoom ?? false,
            viewerNotificationCount: 0,
            viewerImportantNotificationCount: 0,
            hasMessageHistory: null,
            members: []
          };
        });
    },
    get roomGroups() {
      return roomGroups ?? [];
    },
    isInitialLoading: false,
    isRoomMember(roomId: string) {
      return this.rooms.some((room) => room.id === roomId && room.viewerIsMember);
    }
  };
  const directory = new RoomDirectoryStore(
    navigation,
    stubMemberDirectoryAPI,
    stubRoomAPI
  );
</script>

<RoomDirectory {directory} serverSegment="-" />
