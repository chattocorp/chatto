<script lang="ts">
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { getChromePermissions } from '$lib/state/space';

  // Get space permissions from context (set by parent layout)
  // Access .current in $derived to maintain reactivity when permissions load async
  const chromePermissions = getChromePermissions();
  const permissionsLoaded = $derived(chromePermissions.current.loaded);
  const canBrowseRooms = $derived(chromePermissions.current.canBrowseRooms);

  // The active server's stores. Both substores self-manage refresh and
  // live-event ingestion from inside `ServerStateStore`, so this page just
  // reads them.
  const stores = $derived(serverRegistry.getStore(getActiveServer()));
  const directory = $derived(stores.roomDirectory);
  const roomsStore = $derived(stores.rooms);
</script>

<PageTitle title="Browse Rooms" />

{#if !permissionsLoaded}
  <!-- Render the page shell while server permissions are still loading.
       Without this, we'd flash "Access Denied" during the brief window
       between layout mount and validateSpace returning. -->
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader title="Browse Rooms" showMobileNav />
  </div>
{:else if !canBrowseRooms}
  <div class="flex h-full w-full flex-col items-center justify-center gap-4">
    <div class="text-2xl font-semibold text-danger">Access Denied</div>
    <div class="text-lg text-muted">You do not have permission to browse rooms in this space.</div>
    <a href={resolve('/chat/[serverId]', { serverId: serverIdToSegment(getActiveServer()) })} class="text-primary hover:underline"
      >Return to Space</a
    >
  </div>
{:else}
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader title="Browse Rooms" showMobileNav />

    <div class="flex-1 overflow-auto p-6">
      <div class="max-w-2xl">
        <RoomDirectory {directory} {roomsStore} />
      </div>
    </div>
  </div>
{/if}
