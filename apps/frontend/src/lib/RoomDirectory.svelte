<!--
@component

Room directory rendered as a responsive grid of group cards. Each card
represents one room group from the admin-defined layout; rooms inside
are compact rows with a join / joined / restricted indicator. The
header carries a "Join all" affordance when there's at least one
joinable, non-joined room left in the group.

The active server's command store is passed as a prop. Directory rows and
membership are read through its projection-backed navigation view, while the
store owns only optimistic join/leave state.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import { ConfirmDialog, EmptyState, HelpTooltip, Panel, Pill } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
  import type { RoomDirectoryStore, DirectoryRoom } from '$lib/state/server/roomDirectory.svelte';

  let {
    directory,
    serverSegment
  }: {
    directory: RoomDirectoryStore;
    serverSegment: string;
  } = $props();

  const searchInputId = $props.id();
  let searchQuery = $state('');
  let leaveConfirmVisible = $state(false);
  let leaveConfirmRoom = $state<DirectoryRoom | null>(null);

  // --- Derived data ---

  const roomGroups = $derived(directory.roomGroups);
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
          ? m('room.join.success', { room: result.room.name })
          : m('room.join.success_generic')
      );
    } else {
      toast.error(m('room.join.failed'));
      console.error('Error joining room:', result.error);
    }
  }

  async function handleJoinGroup(group: { id: string; name: string }) {
    const result = await directory.joinGroup(group.id);
    if (result.ok) {
      if (result.joinedRoomIds.length === 0) {
        toast.success(m('room.directory.already_in_group', { group: group.name }));
      } else {
        toast.success(
          result.joinedRoomIds.length === 1
            ? m('room.directory.joined_group_one', { group: group.name })
            : m('room.directory.joined_group_many', {
                count: result.joinedRoomIds.length,
                group: group.name
              })
        );
      }
    } else {
      toast.error(m('room.directory.join_group_failed'));
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
        !directory.isJoined(r.id) &&
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
          ? m('room.directory.left', { room: result.room.name })
          : m('room.directory.left_generic')
      );
    } else {
      toast.error(m('room.leave.failed'));
      console.error('Error leaving room:', result.error);
    }
  }
</script>

{#snippet roomRow(room: DirectoryRoom)}
  {@const joined = directory.isJoined(room.id)}
  {@const joining = directory.joiningIds.has(room.id)}
  {@const leaving = directory.leavingIds.has(room.id)}
  <!--
    Every status control uses a column of at least w-28, so Join, Joined, and
    the Universal and Restricted labels line up; longer translations widen the
    column instead of overflowing it. Joined rests as a quiet
    danger-ghost button, so the eye goes to the Join buttons; hover and focus
    reveal the Leave action.
  -->
  {@const roomHref = resolve('/chat/[serverId]/[roomId]', {
    serverId: serverSegment,
    roomId: room.id
  })}
  <li class="flex items-center gap-3 selectable-list-item px-3 py-1.5">
    {#snippet roomLabel()}
      <div class="flex min-w-0 items-start gap-2 font-medium">
        <span class="mt-0.5 shrink-0 text-muted/60">#</span>
        <div class="min-w-0 flex-1">
          <div class="flex min-w-0 items-center gap-2">
            <span class="min-w-0 truncate">{room.name}</span>
          </div>
          {#if room.description}
            <div class="truncate text-xs font-normal text-muted">{room.description}</div>
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

    <div class="flex min-w-28 shrink-0 items-center justify-center gap-1">
      {#if joined && room.isUniversal}
        <Pill tone="action">
          <span aria-hidden="true" class="iconify icon-[uil--globe]"></span>
          {m('room.directory.universal')}
        </Pill>
        <HelpTooltip>{m('room.directory.universal_title')}</HelpTooltip>
      {:else if joined}
        <div class="group/leave w-full">
          <Button
            variant="danger-ghost"
            size="sm"
            fullWidth
            loading={leaving}
            disabled={leaving}
            title={m('room.directory.joined_title', { room: room.name })}
            onclick={() => promptLeaveRoom(room)}
          >
            {#if leaving}
              {m('room.directory.leaving')}
            {:else}
              <!-- Both states share one grid cell, so the swap does not change the width. -->
              <span class="grid justify-items-center">
                <span
                  class="col-start-1 row-start-1 inline-flex items-center gap-2 group-focus-within/leave:invisible group-hover/leave:invisible"
                >
                  <span aria-hidden="true" class="iconify icon-[uil--check]"></span>
                  <span>{m('room.directory.joined')}</span>
                </span>
                <span
                  class="invisible col-start-1 row-start-1 inline-flex items-center gap-2 group-focus-within/leave:visible group-hover/leave:visible"
                >
                  <span aria-hidden="true" class="iconify icon-[uil--sign-out-alt] rtl:-scale-x-100"
                  ></span>
                  <span>{m('room.directory.leave')}</span>
                </span>
              </span>
            {/if}
          </Button>
        </div>
      {:else if room.viewerCanJoinRoom || joining}
        <Button
          size="sm"
          fullWidth
          loading={joining}
          disabled={joining}
          onclick={() => handleJoin(room.id)}
        >
          {#if joining}
            {m('room.directory.joining')}
          {:else}
            <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
            {m('room.directory.join')}
          {/if}
        </Button>
      {:else}
        <Pill tone="muted">
          <span aria-hidden="true" class="iconify icon-[uil--lock]"></span>
          {m('room.directory.restricted')}
        </Pill>
        <HelpTooltip>{m('room.directory.restricted_title')}</HelpTooltip>
      {/if}
    </div>
  </li>
{/snippet}

{#snippet groupCard(set: { id: string; name: string; roomIds: string[] }, rooms: DirectoryRoom[])}
  {@const joining = directory.joiningGroupIds.has(set.id)}
  {@const canJoinAll = canJoinAllInGroup(rooms)}
  <div {@attach masonryItem} class="self-start">
    <Panel title={set.name} noPadding>
      {#snippet actions()}
        {#if canJoinAll || joining}
          <!-- Shares the row status column's minimum width, so it lines up with Join and Joined. -->
          <div class="min-w-28 shrink-0">
            <Button
              size="sm"
              fullWidth
              loading={joining}
              disabled={joining}
              onclick={() => handleJoinGroup(set)}
            >
              {#if joining}
                {m('room.directory.joining')}
              {:else}
                <span aria-hidden="true" class="iconify icon-[uil--plus-circle]"></span>
                {m('room.directory.join_all')}
              {/if}
            </Button>
          </div>
        {/if}
      {/snippet}

      <ul class="selectable-list">
        {#each rooms as room (room.id)}
          {@render roomRow(room)}
        {/each}
      </ul>
    </Panel>
  </div>
{/snippet}

<div class="mb-6">
  <TextInput
    id={searchInputId}
    label={m('room.directory.search_placeholder')}
    labelHidden
    leadingIcon="iconify icon-[uil--search]"
    placeholder={m('room.directory.search_placeholder')}
    bind:value={searchQuery}
  />
</div>

{#if visibleRooms.length === 0}
  <EmptyState icon="icon-[uil--comments]" title={m('room.directory.empty')} />
{:else if !hasVisibleResults}
  <EmptyState icon="icon-[uil--search-minus]" title={m('room.directory.no_results')} />
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
      { id: 'all', name: m('common.rooms'), roomIds: filteredRooms.map((r) => r.id) },
      filteredRooms
    )}
  </div>
{/if}

<ConfirmDialog
  bind:visible={leaveConfirmVisible}
  title={m('room.leave.title')}
  actionLabel={m('room.leave.action')}
  actionIcon="iconify icon-[uil--sign-out-alt] rtl:-scale-x-100"
  onconfirm={confirmLeaveRoom}
  onclose={() => (leaveConfirmVisible = false)}
>
  {m('room.directory.leave_confirm', { room: leaveConfirmRoom?.name ?? '' })}
</ConfirmDialog>
