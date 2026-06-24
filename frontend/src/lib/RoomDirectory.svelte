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
  import { resolve } from '$app/paths';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';
  import { Button } from '$lib/ui/form';
  import Dialog from '$lib/ui/Dialog.svelte';
  import type { RoomsStore } from '$lib/state/server/rooms.svelte';
  import type { RoomDirectoryStore, DirectoryRoom } from '$lib/state/server/roomDirectory.svelte';

  let {
    directory,
    roomsStore,
    serverSegment
  }: {
    directory: RoomDirectoryStore;
    roomsStore: RoomsStore;
    serverSegment: string;
  } = $props();

  let searchQuery = $state('');
  let leaveConfirmVisible = $state(false);
  let leaveConfirmRoom = $state<DirectoryRoom | null>(null);

  // --- Derived data ---

  const joinedRoomIds = $derived(
    new Set(roomsStore.rooms.filter((r) => r.viewerIsMember).map((r) => r.id))
  );
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
  const hasVisibleResults = $derived(hasLayout ? visibleSets.length > 0 : filteredRooms.length > 0);

  // --- Actions ---

  async function handleJoin(roomId: string) {
    const result = await directory.joinRoom(roomId);
    if (result.ok) {
      toast.success(
        result.room
          ? m['room.join.success']({ room: result.room.name })
          : m['room.join.success_generic']()
      );
    } else {
      toast.error(m['room.join.failed']());
      console.error('Error joining room:', result.error);
    }
  }

  async function handleJoinGroup(group: { id: string; name: string }) {
    const result = await directory.joinGroup(group.id);
    if (result.ok) {
      if (result.joinedRoomIds.length === 0) {
        toast.success(m['room.directory.already_in_group']({ group: group.name }));
      } else {
        toast.success(
          result.joinedRoomIds.length === 1
            ? m['room.directory.joined_group_one']({ group: group.name })
            : m['room.directory.joined_group_many']({
                count: result.joinedRoomIds.length,
                group: group.name
              })
        );
      }
    } else {
      toast.error(m['room.directory.join_group_failed']());
      console.error('Error joining group:', result.error);
    }
  }

  // A group is "join-allable" iff it has at least one not-yet-joined,
  // self-joinable room. Cheap to compute per render — no debouncing needed.
  function canJoinAllInGroup(rooms: DirectoryRoom[]): boolean {
    return rooms.some(
      (r) =>
        r.viewerCanJoinRoom &&
        !r.isUniversal &&
        !directory.isJoined(r.id, joinedRoomIds) &&
        !directory.joiningIds.has(r.id)
    );
  }

  // JS-based masonry: each card spans as many small grid rows as its
  // measured height needs, so the browser packs cards via
  // `grid-auto-flow: dense` and they end up left-to-right per row
  // (proper Pinterest layout) without depending on the still-
  // experimental `grid-template-rows: masonry` property.
  const MASONRY_ROW = 8; // px; must match the grid container's grid-auto-rows
  const MASONRY_GAP = 16; // px; must match the container's gap-4

  function masonryItem(node: HTMLElement) {
    let raf = 0;
    function measure() {
      // Use the card's natural content height — we set align-self: start
      // on grid items, so the row span doesn't feed back into the height.
      const h = node.getBoundingClientRect().height;
      if (!h) return;
      const rows = Math.max(1, Math.ceil((h + MASONRY_GAP) / (MASONRY_ROW + MASONRY_GAP)));
      node.style.gridRow = `span ${rows}`;
    }
    function schedule() {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(measure);
    }
    schedule();
    const ro = new ResizeObserver(schedule);
    ro.observe(node);
    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
    };
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
      toast.success(
        result.room
          ? m['room.directory.left']({ room: result.room.name })
          : m['room.directory.left_generic']()
      );
    } else {
      toast.error(m['room.leave.failed']());
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
  <!--
    `transition-none` overrides the `transition-colors duration-100`
    baked into the `btn` utility — hover swaps feel snappier without
    the fade.
  -->
  {@const sizing = 'btn-sm w-28 shrink-0 justify-center border transition-none'}
  {@const primarySolid = `btn btn-accent border-transparent ${sizing}`}
  <!--
    Joined rooms get a "ghost"-style button that fades into the card
    background, so the eye is drawn to the saturated accent Join
    buttons next to rooms the viewer can act on. Hover swaps to a
    solid danger fill to telegraph the leave action.
  -->
  {@const joinedGhost = `btn border-border bg-background text-muted hover:!border-danger hover:!bg-danger hover:!text-white ${sizing}`}
  {@const restrictedSoft = `btn border-border bg-background text-muted/70 !cursor-default opacity-80 ${sizing}`}
  {@const universalSoft = `btn border-border bg-accent/10 text-accent !cursor-default ${sizing}`}
  {@const roomHref = resolve('/chat/[serverId]/[roomId]', {
    serverId: serverSegment,
    roomId: room.id
  })}
  <li
    class="flex items-center gap-3 rounded px-3 py-1.5 transition-colors {joined
      ? 'hover:bg-surface-200'
      : ''}"
  >
    {#snippet roomLabel()}
      <div class="flex min-w-0 items-start gap-2 font-medium">
        <span class="mt-0.5 shrink-0 text-muted/60">#</span>
        <div class="min-w-0 flex-1">
          <div class="flex min-w-0 items-center gap-2">
            <span class="min-w-0 truncate">{room.name}</span>
          </div>
          {#if room.description}
            <div class="truncate text-xs font-normal text-muted/80">{room.description}</div>
          {/if}
        </div>
      </div>
    {/snippet}
    {#if joined}
      <a href={roomHref} class="min-w-0 flex-1">
        {@render roomLabel()}
      </a>
    {:else}
      <div class="min-w-0 flex-1">
        {@render roomLabel()}
      </div>
    {/if}

    {#if joined && room.isUniversal}
      <span class={universalSoft} title={m['room.directory.universal_title']()}>
        <span class="iconify uil--globe"></span>
        {m['room.directory.universal']()}
      </span>
    {:else if joined}
      <button
        type="button"
        class="group {joinedGhost}"
        onclick={() => promptLeaveRoom(room)}
        disabled={leaving}
        title={m['room.directory.joined_title']({ room: room.name })}
      >
        {#if leaving}
          <span class="iconify animate-spin uil--spinner"></span>
          {m['room.directory.leaving']()}
        {:else}
          <span class="iconify uil--check group-hover:hidden"></span>
          <span class="iconify hidden uil--sign-out-alt group-hover:inline"></span>
          <span class="group-hover:hidden">{m['room.directory.joined']()}</span>
          <span class="hidden group-hover:inline">{m['room.directory.leave']()}</span>
        {/if}
      </button>
    {:else if joining}
      <button type="button" class={primarySolid} disabled>
        <span class="iconify animate-spin uil--spinner"></span>
        {m['room.directory.joining']()}
      </button>
    {:else if room.viewerCanJoinRoom}
      <button type="button" class={primarySolid} onclick={() => handleJoin(room.id)}>
        <span class="iconify uil--plus"></span>
        {m['room.directory.join']()}
      </button>
    {:else}
      <span class={restrictedSoft} title={m['room.directory.restricted_title']()}>
        <span class="iconify uil--lock"></span>
        {m['room.directory.restricted']()}
      </span>
    {/if}
  </li>
{/snippet}

{#snippet groupCard(set: { id: string; name: string; roomIds: string[] }, rooms: DirectoryRoom[])}
  {@const joining = directory.joiningGroupIds.has(set.id)}
  {@const canJoinAll = canJoinAllInGroup(rooms)}
  <div {@attach masonryItem} class="self-start overflow-hidden panel-shell">
    <div class="flex items-center justify-between gap-4 border-b border-border p-4">
      <h2 class="truncate text-lg font-semibold">{set.name}</h2>
      {#if canJoinAll || joining}
        <!-- Matches the per-row primary buttons: w-28 so the card's
             header action lines up vertically with Join / Joined. -->
        <button
          type="button"
          class="btn-accent btn w-28 shrink-0 justify-center border border-transparent btn-sm transition-none"
          onclick={() => handleJoinGroup(set)}
          disabled={joining}
        >
          {#if joining}
            <span class="iconify animate-spin uil--spinner"></span>
            {m['room.directory.joining']()}
          {:else}
            <span class="iconify uil--plus-circle"></span>
            {m['room.directory.join_all']()}
          {/if}
        </button>
      {/if}
    </div>
    <!--
      Horizontal inset (`px-1` + the menu-item's own `px-3` = 16px)
      matches the header's `p-4` so the per-row buttons line up with
      the "Join all" action above.
    -->
    <ul class="flex flex-col gap-0.5 px-1 py-2">
      {#each rooms as room (room.id)}
        {@render roomRow(room)}
      {/each}
    </ul>
  </div>
{/snippet}

<div class="mb-6">
  <input
    type="text"
    placeholder={m['room.directory.search_placeholder']()}
    bind:value={searchQuery}
    class="input w-full"
  />
</div>

{#if visibleRooms.length === 0}
  <p class="text-muted">{m['room.directory.empty']()}</p>
{:else if !hasVisibleResults}
  <p class="text-muted">{m['room.directory.no_results']()}</p>
{:else if hasLayout}
  <!-- Row-major masonry via JS row-spans. Each card is measured by the
       `masonryItem` attachment, which sets `grid-row: span N` to fit
       its content. `grid-auto-flow: dense` then packs cards left-to-
       right, filling shorter columns first. Works everywhere CSS Grid
       does — no dependency on the experimental masonry track. -->
  <div
    class="grid gap-4"
    style="grid-template-columns: repeat(auto-fill, minmax(20rem, 1fr)); grid-auto-rows: 8px; grid-auto-flow: row dense;"
  >
    {#each visibleSets as set (set.id)}
      {@render groupCard(set, getSetRooms(set))}
    {/each}
  </div>
{:else}
  <div
    class="grid gap-4"
    style="grid-template-columns: repeat(auto-fill, minmax(20rem, 1fr)); grid-auto-rows: 8px; grid-auto-flow: row dense;"
  >
    {@render groupCard(
      { id: 'all', name: m['common.rooms'](), roomIds: filteredRooms.map((r) => r.id) },
      filteredRooms
    )}
  </div>
{/if}

<Dialog bind:visible={leaveConfirmVisible} title={m['room.leave.title']()} size="sm">
  <p class="mb-4">
    {m['room.directory.leave_confirm']({ room: leaveConfirmRoom?.name ?? '' })}
  </p>

  <div class="flex items-center gap-3">
    <Button variant="danger" onclick={confirmLeaveRoom}>{m['room.leave.action']()}</Button>
    <Button variant="ghost" onclick={() => (leaveConfirmVisible = false)}>
      {m['common.cancel']()}
    </Button>
  </div>
</Dialog>
