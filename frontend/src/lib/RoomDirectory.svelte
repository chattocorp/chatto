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

{#snippet statusButton(opts: {
  label: string;
  title?: string;
  tone: 'primary' | 'success' | 'neutral';
  onclick?: () => void;
  disabled?: boolean;
})}
  {@const tones = {
    primary:
      'bg-primary text-white hover:bg-primary-hover border border-transparent',
    success:
      'bg-success/10 text-success border border-success/30 hover:bg-danger/10 hover:text-danger hover:border-danger/40',
    neutral: 'bg-surface text-muted border border-border'
  } as const}
  {#if opts.onclick}
    <button
      type="button"
      class={[
        'inline-flex h-7 w-20 shrink-0 items-center justify-center rounded-full text-xs font-medium transition-colors disabled:cursor-wait disabled:opacity-50',
        opts.disabled ? '' : 'cursor-pointer',
        tones[opts.tone]
      ]}
      title={opts.title}
      onclick={opts.onclick}
      disabled={opts.disabled}
    >
      {opts.label}
    </button>
  {:else}
    <span
      class={[
        'inline-flex h-7 w-20 shrink-0 items-center justify-center rounded-full text-xs font-medium',
        tones[opts.tone]
      ]}
      title={opts.title}
    >
      {opts.label}
    </span>
  {/if}
{/snippet}

{#snippet roomRow(room: DirectoryRoom)}
  {@const joined = directory.isJoined(room.id, joinedRoomIds)}
  {@const joining = directory.joiningIds.has(room.id)}
  {@const leaving = directory.leavingIds.has(room.id)}
  <li class="menu-item gap-3">
    <div class="min-w-0 flex-1">
      <div class={['truncate font-medium', joined ? 'text-text' : 'text-muted']}>
        <span class="text-muted/70">#</span>{room.name}
      </div>
      {#if room.description}
        <div class="truncate text-xs text-muted/80">{room.description}</div>
      {/if}
    </div>

    {#if joined}
      {@render statusButton({
        label: leaving ? 'Leaving…' : 'Joined',
        title: `Joined #${room.name} — click to leave`,
        tone: 'success',
        onclick: () => promptLeaveRoom(room),
        disabled: leaving
      })}
    {:else if joining}
      {@render statusButton({ label: 'Joining…', tone: 'primary' })}
    {:else if room.viewerCanJoinRoom}
      {@render statusButton({
        label: 'Join',
        tone: 'primary',
        onclick: () => handleJoin(room.id)
      })}
    {:else}
      {@render statusButton({
        label: 'Restricted',
        title: "You don't have permission to join this room",
        tone: 'neutral'
      })}
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
          <button
            type="button"
            class="btn btn-sm btn-primary disabled:cursor-wait disabled:opacity-50"
            onclick={() => handleJoinGroup(set)}
            disabled={joining}
          >
            {joining ? 'Joining…' : 'Join all'}
          </button>
        {/if}
      {/snippet}
      <ul class="menu-section flex flex-col gap-0.5 m-2 p-1">
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
