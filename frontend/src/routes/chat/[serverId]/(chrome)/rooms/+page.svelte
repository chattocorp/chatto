<script lang="ts">
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  // The active server's stores. Both substores self-manage refresh and
  // live-event ingestion from inside `ServerStateStore`, so this page just
  // reads them. The room-directory backend filters per-room by room.join,
  // so a user without join permission on any room sees an empty directory
  // (no separate "can browse" gate needed).
  const stores = $derived(serverRegistry.getStore(getActiveServer()));
  const directory = $derived(stores.roomDirectory);
  const roomsStore = $derived(stores.rooms);
</script>

<PageTitle title="Browse Rooms" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Browse Rooms" showMobileNav />

  <div class="flex-1 overflow-auto p-6">
    <div class="max-w-2xl">
      <RoomDirectory {directory} {roomsStore} />
    </div>
  </div>
</div>
