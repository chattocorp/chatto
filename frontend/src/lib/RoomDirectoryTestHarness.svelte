<!--
@component

Test-only wrapper around `RoomDirectory`. Constructs a real
`RoomDirectoryStore` with a stubbed urql client, seeds the rooms list, and
provides a stub `SpaceRoomsStore` via context — so component-level tests
can exercise the rendered view without standing up the full
SpaceEventProvider tree.
-->
<script lang="ts">
  import { untrack } from 'svelte';
  import { setSpaceRoomsStore, type SpaceRoom, type SpaceLayoutSection } from '$lib/state/space';
  import {
    RoomDirectoryStore,
    setRoomDirectoryStore,
    type DirectoryRoom
  } from '$lib/state/space/roomDirectory.svelte';
  import RoomDirectory from './RoomDirectory.svelte';

  let {
    initialRooms,
    joinedRooms = [],
    layoutSections = null
  }: {
    initialRooms: DirectoryRoom[];
    joinedRooms?: SpaceRoom[];
    layoutSections?: SpaceLayoutSection[] | null;
  } = $props();

  // urql client stub: query never resolves (we seed `allRooms` directly), so
  // the in-flight load doesn't trample the test fixture.
  const stubClient = {
    query: () => ({ toPromise: () => new Promise(() => {}) }),
    mutation: () => ({ toPromise: () => Promise.resolve({ data: null, error: null }) })
  };

  const directory = new RoomDirectoryStore(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- test stub
    stubClient as any,
    'space-test'
  );
  directory.allRooms = untrack(() => initialRooms);
  directory.isLoading = false;
  setRoomDirectoryStore(directory);

  // SpaceRoomsStore stub: only the fields RoomDirectory reads need to be
  // populated. A full constructor isn't viable here without dragging in
  // notification/roomUnread mocks; a duck-typed object is good enough.
  const spaceRoomsStub = {
    rooms: untrack(() => joinedRooms),
    layoutSections: untrack(() => layoutSections),
    unsectionedRoomIds: [] as string[]
  };
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- partial stub
  setSpaceRoomsStore(spaceRoomsStub as any);
</script>

<RoomDirectory />
