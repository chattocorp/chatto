<!--
@component

Room directory rendered as a responsive grid of group cards. Each card
represents one room group from the admin-defined layout; rooms inside
are compact rows with a join / joined / restricted indicator. The
header carries a "Join all" affordance when there's at least one
joinable, non-joined room left in the group.

Both stores are passed in as props — the active server's `directory`
(`RoomDirectoryStore`) owns the all-rooms listing and optimistic
join/leave state, and the active server's `roomsStore` (`RoomsStore`)
supplies the joined-membership set. Explicit props keep the component
testable without context stubs and decoupled from the multi-server
registry.
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

  const joinedRoomIds = $derived(new Set(roomsStore.rooms.map((r) => r.id)));
  const roomGroups = $derived(roomsStore.roomGroups);
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
    if (!roomGroups) return [];
    return roomGroups.filter((s) => getSetRooms(s).length > 0);
  });

  const hasLayout = $derived(roomGroups !== null && roomGroups.length > 0);
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

  async function handleJoinGroup(group: { id: string; name: string }) {
    const result = await directory.joinGroup(group.id);
    if (result.ok) {
      if (result.joinedRoomIds.length === 0) {
        toast.success(`Already in every room in ${group.name}`);
      } else {
        toast.success(
          `Joined ${result.joinedRoomIds.length} room${result.joinedRoomIds.length === 1 ? '' : 's'} in ${group.name}`
        );
      }
    } else {
      toast.error('Failed to join group');
      console.error('Error joining group:', result.error);
    }
  }

  // A group is "join-allable" iff it has at least one not-yet-joined,
  // self-joinable room. Cheap to compute per render — no debouncing needed.
  function canJoinAllInGroup(rooms: DirectoryRoom[]): boolean {
    return rooms.some(
      (r) =>
        r.viewerCanJoinRoom &&
        !directory.isJoined(r.id, joinedRoomIds) &&
        !directory.joiningIds.has(r.id)
    );
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

{#snippet roomRow(room: DirectoryRoom)}
  {@const joined = directory.isJoined(room.id, joinedRoomIds)}
  {@const joining = directory.joiningIds.has(room.id)}
  {@const leaving = directory.leavingIds.has(room.id)}
  <li class="group flex items-center gap-3 rounded-md px-2 py-1.5 hover:bg-surface-100">
    <div class="min-w-0 flex-1">
      <div class={['truncate font-medium', joined ? 'text-text' : 'text-muted']}>
        <span class="text-muted/70">#</span>{room.name}
      </div>
      {#if room.description}
        <div class="truncate text-xs text-muted/80">{room.description}</div>
      {/if}
    </div>

    {#if joined}
      <button
        type="button"
        class="shrink-0 cursor-pointer rounded-full border border-success/30 bg-success/10 px-2 py-0.5 text-xs font-medium text-success hover:border-danger/40 hover:bg-danger/10 hover:text-danger disabled:cursor-wait disabled:opacity-50"
        onclick={() => promptLeaveRoom(room)}
        disabled={leaving}
        title={leaving ? 'Leaving…' : 'Joined — click to leave'}
      >
        <span class="iconify uil--check inline-block align-[-2px] text-sm group-hover:hidden"
        ></span>
        <span class="hidden group-hover:inline">Leave</span>
        <span class="group-hover:hidden">Joined</span>
      </button>
    {:else if joining}
      <span
        class="shrink-0 rounded-full bg-primary/40 px-2 py-0.5 text-xs font-medium text-white"
      >
        Joining…
      </span>
    {:else if room.viewerCanJoinRoom}
      <button
        type="button"
        class="shrink-0 cursor-pointer rounded-full bg-primary px-3 py-0.5 text-xs font-medium text-white hover:bg-primary-hover"
        onclick={() => handleJoin(room.id)}
      >
        Join
      </button>
    {:else}
      <span
        class="shrink-0 rounded-full border border-border bg-surface px-2 py-0.5 text-xs text-muted"
        title="You don't have permission to join this room"
      >
        Restricted
      </span>
    {/if}
  </li>
{/snippet}

{#snippet groupCard(set: { id: string; name: string; roomIds: string[] }, rooms: DirectoryRoom[])}
  {@const joining = directory.joiningGroupIds.has(set.id)}
  {@const canJoinAll = canJoinAllInGroup(rooms)}
  <article
    class="flex flex-col rounded-xl border border-border bg-surface/60 shadow-sm transition hover:border-border-strong hover:shadow"
  >
    <header
      class="flex items-baseline justify-between gap-3 border-b border-border/60 px-4 py-3"
    >
      <div class="min-w-0">
        <h3 class="truncate text-base font-semibold text-text">{set.name}</h3>
        <p class="text-xs text-muted">
          {rooms.length}
          {rooms.length === 1 ? 'room' : 'rooms'}
        </p>
      </div>
      {#if canJoinAll || joining}
        <button
          type="button"
          class="shrink-0 cursor-pointer rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-white hover:bg-primary-hover disabled:cursor-wait disabled:opacity-50"
          onclick={() => handleJoinGroup(set)}
          disabled={joining}
        >
          {joining ? 'Joining…' : 'Join all'}
        </button>
      {/if}
    </header>
    <ul class="flex flex-col gap-0.5 p-2">
      {#each rooms as room (room.id)}
        {@render roomRow(room)}
      {/each}
    </ul>
  </article>
{/snippet}

<div class="mb-6">
  <input
    type="text"
    placeholder="Search rooms…"
    bind:value={searchQuery}
    class="w-full rounded-md border border-border bg-surface px-3 py-2 text-text placeholder:text-muted focus:border-primary focus:outline-none"
  />
</div>

{#if visibleRooms.length === 0}
  <p class="text-muted">No rooms in this server yet.</p>
{:else if !hasVisibleResults}
  <p class="text-muted">No rooms match your filter.</p>
{:else if hasLayout}
  <div
    class="grid gap-4"
    style="grid-template-columns: repeat(auto-fill, minmax(min(20rem, 100%), 1fr));"
  >
    {#each visibleSets as set (set.id)}
      {@render groupCard(set, getSetRooms(set))}
    {/each}
  </div>
{:else}
  <!-- No groups configured — render as one synthetic card. -->
  <div
    class="grid gap-4"
    style="grid-template-columns: repeat(auto-fill, minmax(min(20rem, 100%), 1fr));"
  >
    {@render groupCard(
      { id: 'all', name: 'Rooms', roomIds: filteredRooms.map((r) => r.id) },
      filteredRooms
    )}
  </div>
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
