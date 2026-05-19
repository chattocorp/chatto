<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation, useActiveRoomLayoutUpdated } from '$lib/hooks';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { EmptyState, Hint, Pill, ToggleChip } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import FormDialog from '$lib/ui/FormDialog.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import { Button, TextInput, TextArea } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { dndzone, type DndEvent } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));

  // --- Queries & Mutations ---

  const RoomGroupsQuery = graphql(`
    query AdminRoomGroups {
      server {
        rooms(type: CHANNEL) {
          id
          name
          description
          archived
        }
        roomGroups {
          id
          name
          rooms {
            id
          }
        }
      }
    }
  `);

  const CreateRoomGroupMutation = graphql(`
    mutation AdminCreateRoomGroup($input: CreateRoomGroupInput!) {
      createRoomGroup(input: $input) {
        id
        name
      }
    }
  `);

  const UpdateRoomGroupMutation = graphql(`
    mutation AdminUpdateRoomGroup($input: UpdateRoomGroupInput!) {
      updateRoomGroup(input: $input) {
        id
        name
      }
    }
  `);

  const DeleteRoomGroupMutation = graphql(`
    mutation AdminDeleteRoomGroup($input: DeleteRoomGroupInput!) {
      deleteRoomGroup(input: $input)
    }
  `);

  const ReorderRoomGroupsMutation = graphql(`
    mutation AdminReorderRoomGroups($input: ReorderRoomGroupsInput!) {
      reorderRoomGroups(input: $input) {
        id
      }
    }
  `);

  const MoveRoomToSetMutation = graphql(`
    mutation AdminMoveRoomToSet($input: MoveRoomToSetInput!) {
      moveRoomToSet(input: $input) {
        id
      }
    }
  `);

  const ReorderRoomsInGroupMutation = graphql(`
    mutation AdminReorderRoomsInGroup($input: ReorderRoomsInGroupInput!) {
      reorderRoomsInGroup(input: $input) {
        id
      }
    }
  `);

  const UpdateRoomMutation = graphql(`
    mutation AdminUpdateRoom($input: UpdateRoomInput!) {
      updateRoom(input: $input) {
        id
        name
        description
      }
    }
  `);

  const ArchiveRoomMutation = graphql(`
    mutation ArchiveRoom($input: ArchiveRoomInput!) {
      archiveRoom(input: $input) {
        id
        archived
      }
    }
  `);

  const UnarchiveRoomMutation = graphql(`
    mutation UnarchiveRoom($input: UnarchiveRoomInput!) {
      unarchiveRoom(input: $input) {
        id
        archived
      }
    }
  `);

  const layoutQuery = useQuery(RoomGroupsQuery, () => ({}));
  const createGroupMutation = useMutation(CreateRoomGroupMutation);
  const updateGroupMutation = useMutation(UpdateRoomGroupMutation);
  const deleteGroupMutation = useMutation(DeleteRoomGroupMutation);
  const reorderGroupsMutation = useMutation(ReorderRoomGroupsMutation);
  const moveRoomMutation = useMutation(MoveRoomToSetMutation);
  const reorderRoomsMutation = useMutation(ReorderRoomsInGroupMutation);
  const updateRoomMutation = useMutation(UpdateRoomMutation);
  const archiveMutation = useMutation(ArchiveRoomMutation);
  const unarchiveMutation = useMutation(UnarchiveRoomMutation);

  // --- Types ---

  type RoomInfo = {
    id: string;
    name: string;
    description?: string | null;
    archived: boolean;
  };
  type DndRoomItem = RoomInfo & { id: string };
  type GroupState = {
    id: string;
    name: string;
    rooms: DndRoomItem[];
  };

  // --- Local state ---

  let groups = $state<GroupState[]>([]);
  let initialized = $state(false);
  let isDragging = $state(false);
  let lastMutationTimestamp = 0;

  // Only show the spinner on the very first load — subsequent refetches
  // (triggered by mutations and live events) shouldn't flash the page tree
  // away. Local state already reflects the optimistic update; the refetch
  // just reconciles with the server.
  let loading = $derived(layoutQuery.loading && !initialized);
  let error = $derived(
    layoutQuery.error ??
      (!layoutQuery.loading && !layoutQuery.data?.server ? 'Server not found' : null)
  );

  // Build a lookup of every channel room (active and archived). Archived
  // rooms keep their set position in the admin so the operator can find
  // and unarchive them; the frontend's regular sidebar filters them out.
  let allRooms = $derived(layoutQuery.data?.server?.rooms ?? []);
  let roomsMap = $derived(
    new Map<string, RoomInfo>(
      allRooms.map((r) => [
        r.id,
        {
          id: r.id,
          name: r.name,
          description: r.description,
          archived: r.archived
        }
      ])
    )
  );

  // Initialize local state from query data. Only re-runs when layoutQuery
  // data changes (on refetch). Real-time events are debounced via
  // lastMutationTimestamp to avoid clobbering in-flight DnD edits.
  $effect(() => {
    const space = layoutQuery.data?.server;
    if (!space) return;

    groups = space.roomGroups.map((s) => ({
      id: s.id,
      name: s.name,
      rooms: s.rooms.map((r) => roomsMap.get(r.id)).filter((r): r is RoomInfo => r != null)
    }));

    initialized = true;
  });

  // --- Real-time sync ---

  useActiveRoomLayoutUpdated(() => {
    // Skip refetch during drag or if we just performed a mutation (our own event bouncing back)
    if (isDragging || Date.now() - lastMutationTimestamp < 2000) return;
    layoutQuery.refetch();
  });

  // --- Set creation modal ---

  let createGroupDialogVisible = $state(false);
  let newGroupName = $state('');

  function openCreateGroup() {
    newGroupName = '';
    createGroupDialogVisible = true;
  }

  async function handleCreateGroupSubmit(e: Event) {
    e.preventDefault();
    const name = newGroupName.trim();
    if (!name) return;

    const result = await createGroupMutation.execute({ input: { name } });
    if (result.error || !result.data?.createRoomGroup) {
      toast.error(`Failed to create group: ${result.error ?? 'unknown error'}`);
      return;
    }
    const created = result.data.createRoomGroup;
    groups = [...groups, { id: created.id, name: created.name, rooms: [] }];
    newGroupName = '';
    createGroupDialogVisible = false;
    lastMutationTimestamp = Date.now();
    toast.success('Group created');
  }

  async function renameGroup(groupId: string, newName: string) {
    const idx = groups.findIndex((s) => s.id === groupId);
    if (idx === -1) return;
    const result = await updateGroupMutation.execute({
      input: { id: groupId, name: newName }
    });
    if (result.error) {
      toast.error(`Failed to rename group: ${result.error}`);
      return;
    }
    groups[idx] = { ...groups[idx], name: newName };
    lastMutationTimestamp = Date.now();
    toast.success('Group renamed');
  }

  let deleteGroupConfirmDialogVisible = $state(false);
  let deleteGroupConfirm = $state<GroupState | null>(null);

  function confirmDeleteGroup(group: GroupState) {
    deleteGroupConfirm = group;
    deleteGroupConfirmDialogVisible = true;
  }

  async function deleteGroup() {
    if (!deleteGroupConfirm) return;
    const target = deleteGroupConfirm;
    const result = await deleteGroupMutation.execute({ input: { id: target.id } });
    deleteGroupConfirmDialogVisible = false;
    deleteGroupConfirm = null;
    if (result.error) {
      toast.error(`Failed to delete group: ${result.error}`);
      return;
    }
    groups = groups.filter((s) => s.id !== target.id);
    lastMutationTimestamp = Date.now();
    toast.success('Group deleted');
  }

  // --- Drag-and-drop handlers ---
  //
  // Two distinct mutations cover room drags:
  //   - cross-group → `moveRoomToSet` per room that changed groups
  //   - within-group reorder → `reorderRoomsInGroup` per group whose
  //     room order changed
  // svelte-dnd-action fires onfinalize on both source and target zones
  // for cross-zone moves; we capture a pre-drag snapshot of every
  // group's room order so the diff after the drag yields both kinds of
  // change in one pass.

  type GroupRoomOrder = Map<string, string[]>;

  function buildGroupRoomOrder(state: GroupState[]): GroupRoomOrder {
    const map = new Map<string, string[]>();
    for (const g of state) map.set(g.id, g.rooms.map((r) => r.id));
    return map;
  }

  function buildRoomToGroup(snapshot: GroupRoomOrder): Map<string, string> {
    const map = new Map<string, string>();
    for (const [groupId, roomIds] of snapshot) {
      for (const roomId of roomIds) map.set(roomId, groupId);
    }
    return map;
  }

  function sameOrder(a: string[], b: string[] | undefined): boolean {
    if (!b || a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
    return true;
  }

  let preDragSnapshot: GroupRoomOrder | null = null;
  let pendingMoveDiff = false;

  function captureSnapshotIfNeeded() {
    if (!preDragSnapshot) preDragSnapshot = buildGroupRoomOrder(groups);
  }

  async function flushRoomMoves() {
    if (!preDragSnapshot) return;
    const before = preDragSnapshot;
    preDragSnapshot = null;

    const after = buildGroupRoomOrder(groups);
    const beforeRoomGroup = buildRoomToGroup(before);
    const afterRoomGroup = buildRoomToGroup(after);

    // Cross-group moves: room is now in a different group than before.
    // The server's MoveRoomToGroup appends to the target's end; the
    // post-move reorder pass below restores the user's intended position.
    const moves: Array<{ roomId: string; groupId: string }> = [];
    for (const [roomId, groupId] of afterRoomGroup) {
      if (beforeRoomGroup.get(roomId) !== groupId) moves.push({ roomId, groupId });
    }

    // Reorder pass: any group whose room sequence changed needs a
    // `reorderRoomsInGroup` call. This covers both pure intra-group
    // reorder and the "drop into the middle of another group" case
    // (where the move appends, then this reorder lifts the room into
    // its dropped position). Reorder always runs AFTER moves so the
    // server's membership set already matches `after` at that point.
    const reorders: Array<{ groupId: string; orderedRoomIds: string[] }> = [];
    for (const [groupId, orderedRoomIds] of after) {
      if (!sameOrder(orderedRoomIds, before.get(groupId))) {
        reorders.push({ groupId, orderedRoomIds });
      }
    }

    if (moves.length === 0 && reorders.length === 0) return;

    let anyFailed = false;
    for (const move of moves) {
      const result = await moveRoomMutation.execute({ input: move });
      if (result.error) {
        anyFailed = true;
        toast.error(`Failed to move room: ${result.error}`);
      }
    }
    for (const r of reorders) {
      const result = await reorderRoomsMutation.execute({ input: r });
      if (result.error) {
        anyFailed = true;
        toast.error(`Failed to reorder rooms: ${result.error}`);
      }
    }
    lastMutationTimestamp = Date.now();
    if (anyFailed) {
      layoutQuery.refetch();
    } else if (moves.length > 0) {
      toast.success(moves.length === 1 ? 'Room moved' : `${moves.length} rooms moved`);
    }
  }

  function handleGroupConsider(groupId: string, e: CustomEvent<DndEvent<DndRoomItem>>) {
    isDragging = true;
    captureSnapshotIfNeeded();
    const idx = groups.findIndex((s) => s.id === groupId);
    if (idx !== -1) {
      groups[idx] = { ...groups[idx], rooms: e.detail.items };
    }
  }

  function handleGroupFinalize(groupId: string, e: CustomEvent<DndEvent<DndRoomItem>>) {
    const idx = groups.findIndex((s) => s.id === groupId);
    if (idx !== -1) {
      groups[idx] = { ...groups[idx], rooms: e.detail.items };
    }
    isDragging = false;
    // svelte-dnd-action fires finalize on BOTH source and target zones
    // for cross-zone moves. Batch the diff into the next microtask so we
    // only emit mutations once both zones have updated.
    if (!pendingMoveDiff) {
      pendingMoveDiff = true;
      queueMicrotask(() => {
        pendingMoveDiff = false;
        void flushRoomMoves();
      });
    }
  }

  // Drag-and-drop for reordering groups themselves
  type DndGroupItem = GroupState & { id: string };

  let draggingGroupId = $state<string | null>(null);
  let preReorderIds: string[] | null = null;

  function handleGroupsConsider(e: CustomEvent<DndEvent<DndGroupItem>>) {
    isDragging = true;
    draggingGroupId = e.detail.info?.id ?? null;
    if (!preReorderIds) preReorderIds = groups.map((g) => g.id);
    groups = e.detail.items;
  }

  async function handleGroupsFinalize(e: CustomEvent<DndEvent<DndGroupItem>>) {
    draggingGroupId = null;
    groups = e.detail.items;
    isDragging = false;

    const before = preReorderIds;
    preReorderIds = null;
    const after = groups.map((g) => g.id);
    if (!before || (before.length === after.length && before.every((id, i) => id === after[i]))) {
      return;
    }

    const result = await reorderGroupsMutation.execute({ input: { orderedIds: after } });
    if (result.error) {
      toast.error(`Failed to reorder groups: ${result.error}`);
      layoutQuery.refetch();
      return;
    }
    lastMutationTimestamp = Date.now();
  }

  // --- Set rename modal ---

  let editGroupDialogVisible = $state(false);
  let editGroupId = $state('');
  let editGroupName = $state('');

  function openEditGroup(group: GroupState) {
    editGroupId = group.id;
    editGroupName = group.name;
    editGroupDialogVisible = true;
  }

  function handleEditGroupSubmit(e: Event) {
    e.preventDefault();
    if (editGroupId && editGroupName.trim()) {
      renameGroup(editGroupId, editGroupName.trim());
    }
    editGroupDialogVisible = false;
  }

  // --- Room editing ---

  let editRoomDialogVisible = $state(false);
  let editRoomId = $state('');
  let editRoomName = $state('');
  let editRoomDescription = $state('');

  let editRoomNameError = $derived.by(() => {
    if (!editRoomName) return undefined;
    if (editRoomName.trim() === '') return 'Room name cannot be empty';
    if (editRoomName !== editRoomName.trim())
      return 'Room name cannot have leading or trailing whitespace';
    if (!/^[a-zA-Z0-9_-]+$/.test(editRoomName.trim())) {
      return 'Room name can only contain letters, numbers, hyphens, and underscores';
    }
    if (editRoomName.length > 30) {
      return 'Room name cannot exceed 30 characters';
    }
    return undefined;
  });

  function openEditRoom(room: { id: string; name: string; description?: string | null }) {
    editRoomId = room.id;
    editRoomName = room.name;
    editRoomDescription = room.description ?? '';
    editRoomDialogVisible = true;
  }

  async function handleEditRoomSubmit(e: Event) {
    e.preventDefault();
    if (editRoomNameError || !editRoomName.trim()) return;

    const result = await updateRoomMutation.execute({
      input: {
        roomId: editRoomId,
        name: editRoomName.trim(),
        description: editRoomDescription.trim() || null
      }
    });

    if (result.error) {
      toast.error(`Failed to update room: ${result.error}`);
    } else {
      toast.success('Room updated');
      editRoomDialogVisible = false;
      lastMutationTimestamp = Date.now();
      layoutQuery.refetch();
    }
  }

  // --- Room archiving ---

  let archivingRoomId = $state<string | null>(null);
  let archiveConfirmDialogVisible = $state(false);
  let archiveConfirmRoom = $state<{ id: string; name: string } | null>(null);

  function confirmArchiveRoom(room: { id: string; name: string }) {
    archiveConfirmRoom = room;
    archiveConfirmDialogVisible = true;
  }

  async function archiveRoom() {
    if (!archiveConfirmRoom) return;
    const roomId = archiveConfirmRoom.id;
    archivingRoomId = roomId;
    archiveConfirmDialogVisible = false;
    const result = await archiveMutation.execute({ input: { roomId } });
    archivingRoomId = null;

    if (result.error) {
      toast.error(`Failed to archive room: ${result.error}`);
    } else {
      toast.success('Room archived');
    }

    archiveConfirmRoom = null;
    lastMutationTimestamp = Date.now();
    layoutQuery.refetch();
  }

  function cancelArchive() {
    archiveConfirmDialogVisible = false;
    archiveConfirmRoom = null;
  }

  let unarchiveConfirmDialogVisible = $state(false);
  let unarchiveConfirmRoom = $state<{ id: string; name: string } | null>(null);

  function confirmUnarchiveRoom(room: { id: string; name: string }) {
    unarchiveConfirmRoom = room;
    unarchiveConfirmDialogVisible = true;
  }

  async function unarchiveRoom() {
    if (!unarchiveConfirmRoom) return;
    const roomId = unarchiveConfirmRoom.id;
    archivingRoomId = roomId;
    unarchiveConfirmDialogVisible = false;
    const result = await unarchiveMutation.execute({ input: { roomId } });
    archivingRoomId = null;

    if (result.error) {
      toast.error(`Failed to unarchive room: ${result.error}`);
    } else {
      toast.success('Room unarchived');
      lastMutationTimestamp = Date.now();
      layoutQuery.refetch();
    }
    unarchiveConfirmRoom = null;
  }

  function cancelUnarchive() {
    unarchiveConfirmDialogVisible = false;
    unarchiveConfirmRoom = null;
  }

  // --- Permissions navigation ---
  //
  // Group / room permissions live on dedicated routes
  // (`/server-admin/rooms/group/[groupId]` and `.../room/[roomId]`).
  // Clicking the shield chip navigates there; the destination page has
  // its own back arrow.

  function openGroupPermissions(group: GroupState) {
    goto(
      resolve('/chat/[serverId]/(chrome)/server-admin/rooms/group/[groupId]', {
        serverId: serverSegment,
        groupId: group.id
      })
    );
  }

  function openRoomPermissions(room: RoomInfo) {
    goto(
      resolve('/chat/[serverId]/(chrome)/server-admin/rooms/room/[roomId]', {
        serverId: serverSegment,
        roomId: room.id
      })
    );
  }

  // --- Room creation modal ---

  let createRoomDialogVisible = $state(false);
  let createRoomGroupId = $state<string | null>(null);

  function openCreateRoom(group: GroupState) {
    createRoomGroupId = group.id;
    createRoomDialogVisible = true;
  }

  function handleRoomCreated() {
    createRoomDialogVisible = false;
    createRoomGroupId = null;
    toast.success('Room created');
    lastMutationTimestamp = Date.now();
    layoutQuery.refetch();
  }
</script>

{#snippet iconButton(opts: {
  icon: string;
  title: string;
  onclick: () => void;
  disabled?: boolean;
  tone?: 'neutral' | 'warning' | 'danger';
})}
  <ToggleChip
    tone={opts.tone ?? 'neutral'}
    square
    title={opts.title}
    disabled={opts.disabled}
    onclick={(e) => {
      e.stopPropagation();
      opts.onclick();
    }}
  >
    <span class={['iconify text-base', opts.icon]} aria-label={opts.title}></span>
  </ToggleChip>
{/snippet}

{#snippet roomActions(room: DndRoomItem)}
  {@render iconButton({
    icon: 'uil--pen',
    title: 'Edit room',
    onclick: () => openEditRoom(room)
  })}
  {@render iconButton({
    icon: 'uil--shield',
    title: 'Per-room permission overrides',
    onclick: () => openRoomPermissions(room)
  })}
  {#if room.archived}
    {@render iconButton({
      icon: 'uil--redo',
      title: 'Unarchive room',
      disabled: archivingRoomId === room.id,
      onclick: () => confirmUnarchiveRoom(room)
    })}
  {:else}
    {@render iconButton({
      icon: 'uil--archive',
      title: 'Archive room',
      tone: 'warning',
      disabled: archivingRoomId === room.id,
      onclick: () => confirmArchiveRoom(room)
    })}
  {/if}
{/snippet}

<PageTitle title="Rooms | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Rooms"
    subtitle="Create, edit, organize, and archive rooms"
    showMobileNav
  />

  <div class="flex flex-col gap-4 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading rooms...</div>
    {:else if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      {#if groups.length === 0}
        <EmptyState icon="uil--layer-group" title="No room groups yet">
          Create a set to start organizing rooms.
        </EmptyState>
      {:else}
        <Hint>
          Drag rooms between groups to organize them. Drag group headers to reorder groups.
          Archived rooms stay in their set but are hidden from members.
        </Hint>
      {/if}

      <div
        class="flex flex-col gap-4"
        use:dndzone={{
          items: groups,
          flipDurationMs: 200,
          dropTargetStyle: {},
          type: 'groups'
        }}
        onconsider={handleGroupsConsider}
        onfinalize={handleGroupsFinalize}
      >
        {#each groups as group (group.id)}
          <section
            animate:flip={{ duration: 200 }}
            class={[
              'overflow-hidden rounded-xl border border-border bg-background shadow-md transition-shadow',
              draggingGroupId === group.id && 'shadow-lg ring-1 ring-accent/30'
            ]}
          >
            <!-- Set header -->
            <header
              class="group-header panel-header flex items-center gap-3 px-4 py-3"
            >
              <span
                role="button"
                tabindex="0"
                class="iconify shrink-0 cursor-grab text-lg text-muted hover:text-text uil--draggabledots"
                title="Drag to reorder group"
                aria-label="Drag to reorder group"
              ></span>

              <div class="flex min-w-0 flex-1 items-center gap-2">
                <h2 class="truncate text-lg font-semibold">{group.name}</h2>
                <Pill tone="muted">{group.rooms.length}</Pill>
              </div>

              <div class="flex items-center gap-2">
                <Button variant="secondary" size="sm" onclick={() => openCreateRoom(group)}>
                  <span class="iconify uil--plus"></span>
                  New Room
                </Button>
                <div class="flex items-center gap-1.5">
                  {@render iconButton({
                    icon: 'uil--pen',
                    title: 'Rename group',
                    onclick: () => openEditGroup(group)
                  })}
                  {@render iconButton({
                    icon: 'uil--shield',
                    title: 'Group permissions',
                    onclick: () => openGroupPermissions(group)
                  })}
                  {@render iconButton({
                    icon: 'uil--trash-alt',
                    title:
                      group.rooms.length === 0
                        ? 'Delete group'
                        : 'Move all rooms out of this group before deleting',
                    tone: 'danger',
                    disabled: group.rooms.length > 0,
                    onclick: () => confirmDeleteGroup(group)
                  })}
                </div>
              </div>
            </header>

            <!-- Room drop zone -->
            <div
              class="min-h-12 p-2"
              use:dndzone={{
                items: group.rooms,
                flipDurationMs: 200,
                dropTargetStyle: {
                  outline: '2px dashed var(--color-accent)',
                  'outline-offset': '-2px',
                  'border-radius': '0.5rem',
                  'background-color': 'color-mix(in srgb, var(--color-accent) 5%, transparent)'
                },
                type: 'rooms'
              }}
              onconsider={(e) => handleGroupConsider(group.id, e)}
              onfinalize={(e) => handleGroupFinalize(group.id, e)}
            >
              {#each group.rooms as room (room.id)}
                <div
                  animate:flip={{ duration: 200 }}
                  class={[
                    'group flex cursor-grab items-center gap-3 rounded-lg py-2 pl-3 pr-2 hover:bg-surface-100',
                    room.archived && 'opacity-60'
                  ]}
                >
                  <div class="min-w-0 flex-1">
                    <div class="flex min-w-0 items-baseline gap-1">
                      <span class="text-muted">#</span>
                      <span class="truncate font-medium">{room.name}</span>
                      {#if room.archived}
                        <Pill tone="muted">Archived</Pill>
                      {/if}
                    </div>
                    {#if room.description}
                      <p class="truncate text-sm text-muted">{room.description}</p>
                    {/if}
                  </div>
                  <div class="flex items-center gap-1.5">
                    {@render roomActions(room)}
                  </div>
                </div>
              {:else}
                <div class="px-3 py-4 text-center text-sm text-muted">Drop rooms here</div>
              {/each}
            </div>
          </section>
        {/each}
      </div>

      <div class="flex justify-center">
        <Button variant="secondary" onclick={openCreateGroup}>
          <span class="iconify uil--plus"></span>
          New Group
        </Button>
      </div>
    {/if}
  </div>
</div>

<!-- Create Room Dialog -->
<Dialog bind:visible={createRoomDialogVisible} title="Create Room" size="sm">
  {#if createRoomDialogVisible && createRoomGroupId}
    <CreateRoom groupId={createRoomGroupId} onroomcreated={handleRoomCreated} />
  {/if}
</Dialog>

<!-- Edit Room Dialog -->
<FormDialog
  bind:visible={editRoomDialogVisible}
  title="Edit Room"
  size="sm"
  submitLabel="Save Changes"
  submitLoadingText="Saving..."
  loading={updateRoomMutation.loading}
  disabled={!editRoomName.trim() || !!editRoomNameError}
  onsubmit={handleEditRoomSubmit}
  onclose={() => (editRoomDialogVisible = false)}
>
  <TextInput
    id="edit-room-name"
    label="Name"
    bind:value={editRoomName}
    required
    disabled={updateRoomMutation.loading}
    error={editRoomNameError}
  />

  <TextArea
    id="edit-room-description"
    label="Description"
    bind:value={editRoomDescription}
    rows={3}
    disabled={updateRoomMutation.loading}
    placeholder="Optional description for this room"
  />
</FormDialog>

<!-- Create Group Dialog -->
<FormDialog
  bind:visible={createGroupDialogVisible}
  title="Create Group"
  size="sm"
  submitLabel="Create Group"
  submitIcon="iconify uil--plus"
  disabled={!newGroupName.trim()}
  onsubmit={handleCreateGroupSubmit}
  onclose={() => (createGroupDialogVisible = false)}
>
  <TextInput
    id="new-group-name"
    label="Group name"
    bind:value={newGroupName}
    placeholder="e.g., General, Projects, Teams"
  />
</FormDialog>

<!-- Edit Set Dialog -->
<FormDialog
  bind:visible={editGroupDialogVisible}
  title="Rename Group"
  size="sm"
  submitLabel="Save"
  disabled={!editGroupName.trim()}
  onsubmit={handleEditGroupSubmit}
  onclose={() => (editGroupDialogVisible = false)}
>
  <TextInput id="edit-group-name" label="Group name" bind:value={editGroupName} />
</FormDialog>

<!-- Delete Group Confirmation Dialog -->
{#if deleteGroupConfirmDialogVisible && deleteGroupConfirm}
  <ConfirmDialog
    title="Delete Group"
    actionLabel="Delete Group"
    actionIcon="iconify uil--trash-alt"
    onconfirm={deleteGroup}
    onclose={() => {
      deleteGroupConfirmDialogVisible = false;
      deleteGroupConfirm = null;
    }}
  >
    Are you sure you want to delete the set <strong>{deleteGroupConfirm.name}</strong>?
  </ConfirmDialog>
{/if}

<!-- Archive Room Confirmation Dialog -->
{#if archiveConfirmDialogVisible && archiveConfirmRoom}
  <ConfirmDialog
    title="Archive Room"
    tone="warning"
    actionLabel="Archive Room"
    actionIcon="iconify uil--archive"
    loading={!!archivingRoomId}
    onconfirm={archiveRoom}
    onclose={cancelArchive}
  >
    Are you sure you want to archive <strong>#{archiveConfirmRoom.name}</strong>? Members will no
    longer be able to access this room.
  </ConfirmDialog>
{/if}

<!-- Unarchive Room Confirmation Dialog -->
{#if unarchiveConfirmDialogVisible && unarchiveConfirmRoom}
  <ConfirmDialog
    title="Unarchive Room"
    tone="warning"
    actionLabel="Unarchive Room"
    actionIcon="iconify uil--redo"
    loading={!!archivingRoomId}
    onconfirm={unarchiveRoom}
    onclose={cancelUnarchive}
  >
    Are you sure you want to unarchive <strong>#{unarchiveConfirmRoom.name}</strong>? Members will
    be able to access it again.
  </ConfirmDialog>
{/if}


