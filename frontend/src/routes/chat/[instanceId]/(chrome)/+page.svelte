<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { untrack } from 'svelte';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getSpaceRoomsStore } from '$lib/state/space';
  import { getLastRoom } from '$lib/storage/lastRoom';

  let { data } = $props();

  const instanceId = getActiveInstance()();
  // Parent remounts this component on spaceId change via {#key}, so the snapshot
  // taken here is the only value this instance will ever see.
  const spaceId = untrack(() => data.spaceId);
  const lastRoom = spaceId ? getLastRoom(instanceId, spaceId) : null;
  const roomsStore = getSpaceRoomsStore();

  function redirectToRoom(roomId: string) {
    void goto(
      resolve('/chat/[instanceId]/[spaceId]/[roomId]', {
        instanceId: instanceIdToSegment(instanceId),
        spaceId: spaceId!,
        roomId
      }),
      { replaceState: true }
    );
  }

  if (lastRoom) redirectToRoom(lastRoom);

  // No cached last room — wait for the (sidebar-shared) rooms store and
  // redirect to any room the user has joined. No extra GraphQL query.
  $effect(() => {
    if (lastRoom || !spaceId || roomsStore.isInitialLoading) return;
    const fallback = roomsStore.rooms[0]?.id;
    if (fallback) redirectToRoom(fallback);
  });
</script>

{#if !lastRoom && !roomsStore.isInitialLoading && roomsStore.rooms.length === 0}
  <div class="flex flex-1 items-center justify-center p-8">
    <div class="max-w-md text-center">
      <div class="mb-6">
        <span class="mb-4 iconify inline-block text-6xl text-muted uil--comments-alt"></span>
        <h2 class="mb-2 text-2xl font-bold">No Room Selected</h2>
        <p class="text-muted">
          Choose a room from your sidebar to get started. We promise this page will eventually do
          something more useful.
        </p>
      </div>
    </div>
  </div>
{/if}
