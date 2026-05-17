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
  import Panel from '$lib/components/admin/Panel.svelte';
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
  <!--
    Every status indicator shares an identical outer box: btn-sm padding
    + a 1px border + `w-24 shrink-0 justify-center`. Each variant uses
    `border border-{tone}` so the inner content area stays at 22px
    regardless of fill style — without explicit borders on the primary
    variant, btn-secondary's visible border would shrink its content
    area by 2px and read as a width mismatch.
  -->
  {@const sizing = 'btn-sm w-28 shrink-0 justify-center border'}
  {@const primarySolid = `btn btn-primary border-transparent ${sizing}`}
  <!--
    Joined rooms get a "ghost"-style button that fades into the card
    background, so the eye is drawn to the saturated primary Join
    buttons next to rooms the viewer can act on. Hover swaps to a
    solid danger fill to telegraph the leave action.
  -->
  {@const joinedGhost = `btn border-border bg-background text-muted transition-colors hover:!border-danger hover:!bg-danger hover:!text-white ${sizing}`}
  {@const restrictedSoft = `btn border-border bg-background text-muted/70 !cursor-default opacity-80 ${sizing}`}
  <li class="menu-item gap-3">
    <div class="min-w-0 flex-1">
      <div class={['truncate font-medium', joined ? 'text-text' : 'text-muted']}>
        <span class="text-muted/60">#</span>{room.name}
      </div>
      {#if room.description}
        <div class="truncate text-xs text-muted/80">{room.description}</div>
      {/if}
    </div>

    {#if joined}
      <button
        type="button"
        class="group {joinedGhost}"
        onclick={() => promptLeaveRoom(room)}
        disabled={leaving}
        title={`Joined #${room.name} — click to leave`}
      >
        {#if leaving}
          <span class="iconify uil--spinner animate-spin"></span>
          Leaving
        {:else}
          <span class="iconify uil--check group-hover:hidden"></span>
          <span class="iconify uil--sign-out-alt hidden group-hover:inline"></span>
          <span class="group-hover:hidden">Joined</span>
          <span class="hidden group-hover:inline">Leave</span>
        {/if}
      </button>
    {:else if joining}
      <button type="button" class={primarySolid} disabled>
        <span class="iconify uil--spinner animate-spin"></span>
        Joining
      </button>
    {:else if room.viewerCanJoinRoom}
      <button type="button" class={primarySolid} onclick={() => handleJoin(room.id)}>
        <span class="iconify uil--plus"></span>
        Join
      </button>
    {:else}
      <span class={restrictedSoft} title="You don't have permission to join this room">
        <span class="iconify uil--lock"></span>
        Restricted
      </span>
    {/if}
  </li>
{/snippet}

{#snippet groupCard(set: { id: string; name: string; roomIds: string[] }, rooms: DirectoryRoom[])}
  {@const joining = directory.joiningGroupIds.has(set.id)}
  {@const canJoinAll = canJoinAllInGroup(rooms)}
  <!--
    Each card is wrapped in a `break-inside-avoid` to prevent the
    column-flow layout from splitting a card across columns.
  -->
  <div class="mb-4 break-inside-avoid">
    <Panel title={set.name} count={rooms.length} noPadding>
      {#snippet actions()}
        {#if canJoinAll || joining}
          <!-- Matches the per-row primary buttons: w-28 + transparent
               border so the card's header action lines up vertically
               with the Join / Joined controls in the rows below. -->
          <button
            type="button"
            class="btn btn-primary btn-sm border border-transparent w-28 shrink-0 justify-center"
            onclick={() => handleJoinGroup(set)}
            disabled={joining}
          >
            {#if joining}
              <span class="iconify uil--spinner animate-spin"></span>
              Joining
            {:else}
              <span class="iconify uil--plus-circle"></span>
              Join all
            {/if}
          </button>
        {/if}
      {/snippet}
      <!--
        Horizontal inset (`px-1` + the menu-item's own `px-3` = 16px)
        matches Panel's `p-4` header so the per-row buttons line up
        with the "Join all" action above. `menu-section`'s default
        `p-1` is overridden by `px-1 py-2` here.
      -->
      <ul class="flex flex-col gap-0.5 rounded-md bg-background px-1 py-2">
        {#each rooms as room (room.id)}
          {@render roomRow(room)}
        {/each}
      </ul>
    </Panel>
  </div>
{/snippet}

<div class="mb-6">
  <input
    type="text"
    placeholder="Search rooms…"
    bind:value={searchQuery}
    class="input w-full"
  />
</div>

{#if visibleRooms.length === 0}
  <p class="text-muted">No rooms in this server yet.</p>
{:else if !hasVisibleResults}
  <p class="text-muted">No rooms match your filter.</p>
{:else if hasLayout}
  <!-- Masonry-style multi-column layout via CSS `columns`. Cards pack
       tightly by their intrinsic height — no fixed grid rows, no gaps. -->
  <div class="gap-4 [column-width:22rem]">
    {#each visibleSets as set (set.id)}
      {@render groupCard(set, getSetRooms(set))}
    {/each}
  </div>
{:else}
  <div class="gap-4 [column-width:22rem]">
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
