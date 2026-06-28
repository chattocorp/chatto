<!--
@component

Test-only wrapper around `RoomDirectory`. Constructs a real
`RoomDirectoryStore` with stubbed APIs, seeds the rooms list, and
passes a duck-typed rooms-store stub as the prop — so component-level
tests can exercise the rendered view without standing up the full
chat-event tree or registering a server in the global registry.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import type {
    RoomsListItem,
    RoomsListGroup,
    RoomsStore
  } from '$lib/state/server/rooms.svelte';
  import {
    RoomDirectoryStore,
    type DirectoryRoom
  } from '$lib/state/server/roomDirectory.svelte';
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
  const stubRoomDirectoryAPI = {
    // The harness seeds `allRooms` directly, so this in-flight load should
    // never replace the fixture data.
    listRooms: () => new Promise<never>(() => {})
  };

  const directory = new RoomDirectoryStore(
    stubRoomDirectoryAPI,
    stubRoomAPI
  );
  directory.allRooms = untrack(() => initialRooms);
  directory.isLoading = false;

  // Rooms-store stub: only the fields RoomDirectory reads need to be
  // populated. A full constructor isn't viable here without dragging in
  // notification/roomUnread mocks; a duck-typed object is good enough.
  const roomsStoreStub = {
    rooms: untrack(() => joinedRooms),
    roomGroups: untrack(() => roomGroups)
  } as unknown as RoomsStore;
</script>

<RoomDirectory {directory} roomsStore={roomsStoreStub} serverSegment="-" />
