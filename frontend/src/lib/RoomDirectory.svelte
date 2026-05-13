<!--
@component

Room directory for browsing and joining rooms. Shows all non-archived
rooms organized by the admin-defined layout (or alphabetically if none).
Rooms can be joined without leaving the page.

Both stores are passed in as props — the active server's `directory` (a
`RoomDirectoryStore`) owns the all-rooms listing and optimistic join/leave
state, and the active server's `roomsStore` (a `RoomsStore`) supplies
the joined-membership set. Explicit props keep the component testable
without context stubs and decoupled from the multi-server registry.
-->
<script lang="ts">
  import { toast } from '$lib/ui/toast';
  import { Button } from '$lib/ui/form';
  import Dialog from '$lib/ui/Dialog.svelte';
  import type { RoomsStore } from '$lib/state/space';
  import type {
    RoomDirectoryStore,
    DirectoryRoom
  } from '$lib/state/space/roomDirectory.svelte';

  let {
    directory,
    roomsStore
  }: { directory: RoomDirectoryStore; roomsStore: RoomsStore } = $props();

  let searchQuery = $state('');
  let leaveConfirmVisible = $state(false);
  let leaveConfirmRoom = $state<DirectoryRoom | null>(null);

  // --- Derived data ---

  // Joined membership comes from the active server's rooms store —
  // RoomsSync keeps it current via per-server event handlers.
  const joinedRoomIds = $derived(new Set(roomsStore.rooms.map((r) => r.id)));

  // Room sets come from the rooms store — same source the sidebar uses,
  // so the directory shows the admin-configured layout consistently.
  const roomSets = $derived(roomsStore.roomSets);

  const visibleRooms = $derived(directory.allRooms.filter((room) => !room.archived));

  function matchesSearch(room: DirectoryRoom): boolean {
    if (!searchQuery.trim()) return true;
    const query = searchQuery.toLowerCase();
    return (
      room.name.toLowerCase().includes(query) ||
      (room.description?.toLowerCase().includes(query) ?? false)
    );
  }

  const filteredRooms = $derived(
    visibleRooms.filter(matchesSearch).sort((a, b) => a.name.localeCompare(b.name))
  );

  const visibleRoomMap = $derived(new Map(visibleRooms.map((r) => [r.id, r])));

  function getSetRooms(set: { roomIds: string[] }): DirectoryRoom[] {
    return set.roomIds
      .map((id) => visibleRoomMap.get(id))
      .filter((r): r is DirectoryRoom => r != null && matchesSearch(r));
  }

  const visibleSets = $derived.by(() => {
    if (!roomSets) return [];
    return roomSets.filter((s) => getSetRooms(s).length > 0);
  });

  const hasLayout = $derived(roomSets !== null && roomSets.length > 0);
  const hasVisibleResults = $derived(
    hasLayout ? visibleSets.length > 0 : filteredRooms.length > 0
  );

  // --- Actions ---

  async function handleJoin(roomId: string) {
    const result = await directory.joinRoom(roomId);
    if (result.ok) {
      toast.success(result.room ? `Joined #${result.room.name}` : 'Joined room');
    } else {
      toast.error('Failed to join room');
      console.error('Error joining room:', result.error);
    }
  }

  function promptLeaveRoom(room: DirectoryRoom) {
    leaveConfirmRoom = room;
    leaveConfirmVisible = true;
  }

  async function confirmLeaveRoom() {
    if (!leaveConfirmRoom) return;
    const roomId = leaveConfirmRoom.id;
    leaveConfirmVisible = false;
    leaveConfirmRoom = null;

    const result = await directory.leaveRoom(roomId);
    if (result.ok) {
      toast.success(result.room ? `Left #${result.room.name}` : 'Left room');
    } else {
      toast.error('Failed to leave room');
      console.error('Error leaving room:', result.error);
    }
  }
</script>

{#snippet roomItem(room: DirectoryRoom)}
  {@const joined = directory.isJoined(room.id, joinedRoomIds)}
  {@const joining = directory.joiningIds.has(room.id)}
  {@const leaving = directory.leavingIds.has(room.id)}
  <li class="flex w-full items-center justify-between gap-4 px-4 py-3">
    <div class="min-w-0 flex-1">
      <div class={['font-medium', joined ? '' : 'text-muted']}># {room.name}</div>
      {#if room.description}
        <div class="truncate text-sm text-muted">{room.description}</div>
      {/if}
    </div>

    {#if joined}
      <button
        type="button"
        class="w-22 shrink-0 cursor-pointer rounded-md border border-success/30 bg-success/10 px-3 py-1.5 text-center text-sm font-medium text-success hover:border-danger/30 hover:bg-danger/10 hover:text-danger disabled:cursor-wait disabled:opacity-50"
        onclick={() => promptLeaveRoom(room)}
        disabled={leaving}
      >
        {leaving ? 'Leaving...' : 'Joined'}
      </button>
    {:else if joining}
      <span
        class="w-22 shrink-0 rounded-md bg-primary px-3 py-1.5 text-center text-sm font-medium text-white opacity-50"
      >
        Joining...
      </span>
    {:else if room.viewerCanJoinRoom}
      <button
        type="button"
        class="w-22 shrink-0 cursor-pointer rounded-md bg-primary px-3 py-1.5 text-center text-sm font-medium text-white hover:bg-primary-hover"
        onclick={() => handleJoin(room.id)}
      >
        Join
      </button>
    {:else}
      <span class="w-22 shrink-0 text-center text-sm text-muted">No permission</span>
    {/if}
  </li>
{/snippet}

{#snippet roomList(rooms: DirectoryRoom[])}
  <ul class="divide-y divide-border overflow-hidden rounded-md border border-border">
    {#each rooms as room (room.id)}
      {@render roomItem(room)}
    {/each}
  </ul>
{/snippet}

<div class="mb-4">
  <input
    type="text"
    placeholder="Filter rooms..."
    bind:value={searchQuery}
    class="w-full rounded-md border border-border bg-surface px-3 py-2 text-text placeholder:text-muted focus:border-primary focus:outline-none"
  />
</div>

{#if visibleRooms.length === 0}
  <p class="text-muted">No rooms in this space yet.</p>
{:else if !hasVisibleResults}
  <p class="text-muted">No rooms match your filter.</p>
{:else if hasLayout}
  <!-- Room-set layout -->
  <div class="flex flex-col gap-6">
    {#each visibleSets as set (set.id)}
      {@const setRooms = getSetRooms(set)}
      <div>
        <h3 class="mb-2 text-xs font-semibold tracking-wider text-muted uppercase">
          {set.name}
        </h3>
        {@render roomList(setRooms)}
      </div>
    {/each}
  </div>
{:else}
  <!-- No layout configured — flat list sorted alphabetically -->
  {@render roomList(filteredRooms)}
{/if}

<Dialog bind:visible={leaveConfirmVisible} title="Leave Room" size="sm">
  <p class="mb-4">
    Are you sure you want to leave <strong>#{leaveConfirmRoom?.name}</strong>?
  </p>

  <div class="flex items-center gap-3">
    <Button variant="danger" onclick={confirmLeaveRoom}>Leave Room</Button>
    <Button variant="ghost" onclick={() => (leaveConfirmVisible = false)}>Cancel</Button>
  </div>
</Dialog>
