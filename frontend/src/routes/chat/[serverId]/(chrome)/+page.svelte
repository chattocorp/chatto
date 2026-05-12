<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getSpaceRoomsStore } from '$lib/state/space';
  import { getLastRoom } from '$lib/storage/lastRoom';

  const serverId = getActiveServer()();
  const lastRoom = $derived(getLastRoom(serverId));
  const roomsStore = getSpaceRoomsStore();
  const serverInfo = $derived(serverRegistry.tryGetStore(serverId)?.serverInfo);
  const serverInfoLoading = $derived(serverInfo?.loading ?? true);

  function redirectToRoom(roomId: string) {
    void goto(
      resolve('/chat/[serverId]/(chrome)/[roomId]', {
        serverId: serverIdToSegment(serverId),
        roomId
      }),
      { replaceState: true }
    );
  }

  $effect(() => {
    if (sessionStorage.getItem('returnUrl')) return;
    if (serverInfoLoading) return;

    if (lastRoom) {
      redirectToRoom(lastRoom);
      return;
    }
    if (!roomsStore.isInitialLoading) {
      const fallback = roomsStore.rooms[0]?.id;
      if (fallback) {
        redirectToRoom(fallback);
      }
    }
  });

  const showNoRoomMessage = $derived(
    !lastRoom && !roomsStore.isInitialLoading && roomsStore.rooms.length === 0
  );
</script>

{#if showNoRoomMessage}
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
