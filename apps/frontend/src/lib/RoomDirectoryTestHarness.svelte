<!--
@component

Test-only wrapper around `RoomDirectory`. Constructs a real
`RoomDirectoryStore` with a stubbed urql client, seeds the rooms list, and
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

  // urql client stub: query never resolves (we seed `allRooms` directly), so
  // the in-flight load doesn't trample the test fixture.
  const stubClient = {
    query: () => ({ toPromise: () => new Promise(() => {}) }),
    mutation: () => ({ toPromise: () => Promise.resolve({ data: null, error: null }) })
  };

  const stubRoomAPI = {
    joinRoom: async () => null,
    leaveRoom: async () => true,
    joinGroup: async () => []
  };

  const directory = new RoomDirectoryStore(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test stub
    stubClient as any,
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
