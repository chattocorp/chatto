<script lang="ts">
  import { resolve } from '$app/paths';
  import { getActiveSpace } from '$lib/state/activeSpace.svelte';
  import { page } from '$app/state';
  import { untrack } from 'svelte';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { useSpaceEvent, useRoomLayoutUpdated } from '$lib/hooks';
  import RoomDirectory from '$lib/RoomDirectory.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { getSpacePermissions } from '$lib/state/space';
  import {
    RoomDirectoryStore,
    setRoomDirectoryStore
  } from '$lib/state/space/roomDirectory.svelte';

  const getInstanceId = getActiveInstance();
  const spaceId = $derived(getActiveSpace()());

  // Get space permissions from context (set by parent layout)
  // Access .current in $derived to maintain reactivity when permissions load async
  const spacePermissions = getSpacePermissions();
  const permissionsLoaded = $derived(spacePermissions.current.loaded);
  const canBrowseRooms = $derived(spacePermissions.current.canBrowseRooms);

  // The parent layout keys on spaceId, so this page (and its store) remount
  // when the primary space changes — `spaceId` is stable for the page's lifetime.
  // Capture it once for the store and the layout-event filter.
  const stableSpaceId = untrack(() => spaceId);

  const connection = useConnection();
  const directory = new RoomDirectoryStore(connection().client, stableSpaceId);
  setRoomDirectoryStore(directory);

  useSpaceEvent((event) => directory.ingestSpaceEvent(event));
  useRoomLayoutUpdated(({ spaceId: eventSpaceId }) => {
    if (eventSpaceId === stableSpaceId) directory.ingestRoomLayoutUpdated();
  });
</script>

<PageTitle title="Browse Rooms" />

{#if !permissionsLoaded}
  <!-- Render the page shell while space permissions are still loading.
       Without this, we'd flash "Access Denied" during the brief window
       between layout mount and validateSpace returning (especially on
       direct URL navigation, where spaceId arrives async). -->
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader title="Browse Rooms" showMobileNav />
  </div>
{:else if !canBrowseRooms}
  <div class="flex h-full w-full flex-col items-center justify-center gap-4">
    <div class="text-2xl font-semibold text-danger">Access Denied</div>
    <div class="text-lg text-muted">You do not have permission to browse rooms in this space.</div>
    <a href={resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(getInstanceId()) })} class="text-primary hover:underline"
      >Return to Space</a
    >
  </div>
{:else}
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader title="Browse Rooms" showMobileNav />

    <div class="flex-1 overflow-auto p-6">
      <div class="max-w-2xl">
        <RoomDirectory />
      </div>
    </div>
  </div>
{/if}
