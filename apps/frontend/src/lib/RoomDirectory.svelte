<!--
@component

Room directory: one section per room group from the admin-defined layout,
each with a responsive grid of room tiles. A tile shows the room's name,
description, and a join / joined / invited / restricted control. The section
heading carries a "Join all" action when at least one joinable, non-joined
room is left in the group. A search field and a membership
filter (all, joined, or not joined rooms) narrow the rooms.

The active server's command store is passed as a prop. Directory rows and
membership are read through its projection-backed navigation view, while the
store owns only optimistic join/leave state.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';
  import { ConfirmDialog, EmptyState, HelpTooltip, Pill, SegmentedControl } from '$lib/ui';
  import { Button, TextInput } from '$lib/ui/form';
  import type { RoomDirectoryStore, DirectoryRoom } from '$lib/state/server/roomDirectory';

  let {
    directory,
    serverSegment
  }: {
    directory: RoomDirectoryStore;
    serverSegment: string;
  } = $props();

  const searchInputId = $props.id();
  let searchQuery = $state('');
  /** Membership filter. Rows stay in "All" when the viewer joins or leaves them. */
  let membershipFilter = $state<'all' | 'joined' | 'not_joined'>('all');
  let leaveConfirmVisible = $state(false);
  let leaveConfirmRoom = $state<DirectoryRoom | null>(null);

  // --- Derived data ---

  const roomGroups = $derived(directory.roomGroups);
  const visibleRooms = $derived(directory.allRooms.filter((room) => !room.archived));

  function matchesFilters(room: DirectoryRoom): boolean {
    if (membershipFilter !== 'all') {
      const joined = directory.isJoined(room.id);
      if (joined !== (membershipFilter === 'joined')) return false;
    }
    if (!searchQuery.trim()) return true;
    const query = searchQuery.toLowerCase();
    return (
      room.name.toLowerCase().includes(query) ||
      (room.description?.toLowerCase().includes(query) ?? false)
    );
  }

  const filteredRooms = $derived(
    visibleRooms.filter(matchesFilters).sort((a, b) => a.name.localeCompare(b.name))
  );

  const visibleRoomMap = $derived(new Map(visibleRooms.map((r) => [r.id, r])));

  function getSetRooms(set: { roomIds: string[] }): DirectoryRoom[] {
    return set.roomIds
      .map((id) => visibleRoomMap.get(id))
      .filter((r): r is DirectoryRoom => r != null && matchesFilters(r));
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

{#snippet roomTile(room: DirectoryRoom)}
  {@const joined = directory.isJoined(room.id)}
  {@const invited = !!room.viewerInvitation}
  {@const joining = directory.joiningIds.has(room.id)}
  {@const leaving = directory.leavingIds.has(room.id)}
  {@const linked = joined || invited}
  {@const roomHref = resolve('/chat/[serverId]/[roomId]', {
    serverId: serverSegment,
    roomId: room.id
  })}
  <!--
    A tile for one room. Joined and invited rooms open from anywhere on the
    tile: the name link stretches over it, and the status control sits above
    the link. Joined rests as a quiet label; hover and focus reveal Leave.
  -->
  <li
    class={[
      'relative flex min-w-0 flex-col gap-1 rounded-xl px-3 py-2.5 transition-colors feedback-quick',
      linked ? 'bg-surface hover:bg-surface-emphasized' : 'bg-surface/50'
    ]}
  >
    <div class="flex min-h-6 min-w-0 items-center gap-2">
      {#if room.isUniversal}
        <span aria-hidden="true" class="iconify icon-[uil--globe] w-4 shrink-0 text-muted/70"
        ></span>
      {:else}
        <span aria-hidden="true" class="w-4 shrink-0 text-center text-muted/70">#</span>
      {/if}
      {#if linked}
        <a
          href={roomHref}
          class="min-w-0 flex-1 truncate font-semibold text-text-top after:absolute after:inset-0 after:rounded-xl focus-visible:outline-none focus-visible:after:outline-2 focus-visible:after:outline-action"
          ><bdi>{room.name}</bdi></a
        >
      {:else}
        <span class="min-w-0 flex-1 truncate font-semibold text-text"><bdi>{room.name}</bdi></span>
      {/if}
      <div class="relative z-10 flex shrink-0 items-center gap-1">
        {#if joined && room.isUniversal}
          <!-- A quiet label like Joined; the leading globe already marks the room. -->
          <span class="px-2 text-xs font-medium text-muted">{m('room.directory.universal')}</span>
          <HelpTooltip>{m('room.directory.universal_title')}</HelpTooltip>
        {:else if joined}
          <div class="group/leave">
            <Button
              variant="danger-ghost"
              size="compact"
              loading={leaving}
              disabled={leaving}
              title={m('room.directory.joined_title', { room: room.name })}
              onclick={() => promptLeaveRoom(room)}
            >
              {#if leaving}
                {m('room.directory.leaving')}
              {:else}
                <!-- Both states share one grid cell, so the swap does not change the width. -->
                <span class="grid justify-items-end">
                  <span
                    class="col-start-1 row-start-1 inline-flex items-center gap-1 group-focus-within/leave:invisible group-hover/leave:invisible"
                  >
                    <span aria-hidden="true" class="iconify icon-[uil--check]"></span>
                    <span>{m('room.directory.joined')}</span>
                  </span>
                  <span
                    class="invisible col-start-1 row-start-1 inline-flex items-center gap-1 group-focus-within/leave:visible group-hover/leave:visible"
                  >
                    <span
                      aria-hidden="true"
                      class="iconify icon-[uil--sign-out-alt] rtl:-scale-x-100"
                    ></span>
                    <span>{m('room.directory.leave')}</span>
                  </span>
                </span>
              {/if}
            </Button>
          </div>
        {:else if invited}
          <Pill tone="action" class="inline-flex items-center gap-1">
            <span aria-hidden="true" class="iconify icon-[uil--envelope-alt]"></span>
            {m('room.directory.invited')}
          </Pill>
        {:else if room.viewerCanJoinRoom || joining}
          <Button
            variant="secondary"
            size="compact"
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
          <Pill tone="muted" class="inline-flex items-center gap-1">
            <span aria-hidden="true" class="iconify icon-[uil--lock]"></span>
            {m('room.directory.restricted')}
          </Pill>
          <HelpTooltip>{m('room.directory.restricted_title')}</HelpTooltip>
        {/if}
      </div>
    </div>
    {#if room.description}
      <p class="line-clamp-2 ps-6 text-sm text-muted">{room.description}</p>
    {/if}
  </li>
{/snippet}

{#snippet group(set: { id: string; name: string; roomIds: string[] }, rooms: DirectoryRoom[])}
  {@const joining = directory.joiningGroupIds.has(set.id)}
  {@const canJoinAll = canJoinAllInGroup(rooms)}
  <section class="flex flex-col gap-2" aria-label={set.name}>
    <div class="flex min-h-6 items-center justify-between gap-3 px-1">
      <h3 class="truncate text-xs font-semibold tracking-wide text-muted uppercase">
        {set.name}
        <span class="font-normal">· {rooms.length}</span>
      </h3>
      {#if canJoinAll || joining}
        <Button
          variant="ghost"
          size="compact"
          loading={joining}
          disabled={joining}
          onclick={() => handleJoinGroup(set)}
        >
          {#if joining}
            {m('room.directory.joining')}
          {:else}
            <span aria-hidden="true" class="iconify icon-[uil--plus]"></span>
            {m('room.directory.join_all')}
          {/if}
        </Button>
      {/if}
    </div>
    <ul class="grid grid-cols-[repeat(auto-fill,minmax(min(100%,15rem),1fr))] gap-2">
      {#each rooms as room (room.id)}
        {@render roomTile(room)}
      {/each}
    </ul>
  </section>
{/snippet}

<div class="mb-6 flex flex-wrap items-center gap-3">
  <div class="min-w-[min(100%,16rem)] flex-1">
    <TextInput
      id={searchInputId}
      label={m('room.directory.search_placeholder')}
      labelHidden
      leadingIcon="iconify icon-[uil--search]"
      placeholder={m('room.directory.search_placeholder')}
      bind:value={searchQuery}
    />
  </div>
  <SegmentedControl
    label={m('room.directory.filter_label')}
    options={[
      { value: 'all', label: m('room.directory.filter_all') },
      { value: 'joined', label: m('room.directory.joined') },
      { value: 'not_joined', label: m('room.directory.filter_not_joined') }
    ]}
    value={membershipFilter}
    onchange={(value) => (membershipFilter = value)}
  />
</div>

{#if visibleRooms.length === 0}
  <EmptyState icon="icon-[uil--comments]" title={m('room.directory.empty')} />
{:else if !hasVisibleResults && !searchQuery.trim() && membershipFilter === 'joined'}
  <EmptyState icon="icon-[uil--comments]" title={m('room.directory.empty_joined')} />
{:else if !hasVisibleResults && !searchQuery.trim() && membershipFilter === 'not_joined'}
  <EmptyState icon="icon-[uil--check-circle]" title={m('room.directory.empty_not_joined')} />
{:else if !hasVisibleResults}
  <EmptyState icon="icon-[uil--search-minus]" title={m('room.directory.no_results')} />
{:else if hasLayout}
  <div class="flex flex-col gap-6">
    {#each visibleSets as set (set.id)}
      {@render group(set, getSetRooms(set))}
    {/each}
  </div>
{:else}
  {@render group(
    { id: 'all', name: m('common.rooms'), roomIds: filteredRooms.map((r) => r.id) },
    filteredRooms
  )}
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
