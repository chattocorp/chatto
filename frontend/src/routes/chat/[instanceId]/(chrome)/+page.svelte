<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance, getActiveInstanceSpaceId } from '$lib/state/activeInstance.svelte';
  import { getInstancePermissions } from '$lib/state/instance/permissions.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { getSpaceRoomsStore } from '$lib/state/space';
  import { getLastRoom } from '$lib/storage/lastRoom';

  const instanceId = getActiveInstance()();
  const getSpaceId = getActiveInstanceSpaceId();
  const spaceId = $derived(getSpaceId());
  const lastRoom = $derived(getLastRoom(instanceId));
  // The SpaceRoomsStore is provided by `SpaceEventProvider`, which is only
  // mounted in the (chrome) layout's `{:else}` branch (when spaceId is set).
  // When this page renders in the no-spaceId branch, the store isn't
  // available — fall back to undefined so the welcome / Browse-Spaces
  // redirect logic can still run without throwing on context lookup.
  const roomsStore = $derived(spaceId ? getSpaceRoomsStore() : undefined);
  const instancePerms = getInstancePermissions();
  const instanceState = $derived(instanceRegistry.tryGetStore(instanceId)?.instance);
  const instanceInfoLoading = $derived(instanceState?.loading ?? true);

  function redirectToRoom(roomId: string) {
    void goto(
      resolve('/chat/[instanceId]/(chrome)/[roomId]', {
        instanceId: instanceIdToSegment(instanceId),
        roomId
      }),
      { replaceState: true }
    );
  }

  $effect(() => {
    if (sessionStorage.getItem('returnUrl')) return;

    // Wait for the instance info query (which loads primarySpaceId) to settle
    // before deciding whether to redirect to a room or to /chat/spaces. Without
    // this gate, a freshly-created space's primarySpaceId arrives async and
    // we'd briefly see spaceId="" and bounce the user to /chat/spaces.
    if (instanceInfoLoading) return;

    // Inside a server with at least one joined room: go to last/first room.
    if (lastRoom) {
      redirectToRoom(lastRoom);
      return;
    }
    if (spaceId && roomsStore && !roomsStore.isInitialLoading) {
      const fallback = roomsStore.rooms[0]?.id;
      if (fallback) {
        redirectToRoom(fallback);
        return;
      }
    }

    // No primary space (fresh install) or no rooms — stay here and let the
    // welcome / empty-state UI render below.
  });

  const showNoRoomMessage = $derived(
    spaceId &&
      !lastRoom &&
      roomsStore &&
      !roomsStore.isInitialLoading &&
      roomsStore.rooms.length === 0
  );
  const showWelcomeMessage = $derived(
    !spaceId && instancePerms.current.loaded
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
{:else if showWelcomeMessage}
  <div class="flex flex-1 items-center justify-center p-8">
    <div class="max-w-md text-center">
      <span class="mb-4 iconify inline-block text-6xl text-muted uil--comment-message"></span>
      <h2 class="mb-2 text-2xl font-bold">Welcome to Chatto!</h2>
      <p class="text-muted">Choose a space from the sidebar to get started.</p>
    </div>
  </div>
{/if}
