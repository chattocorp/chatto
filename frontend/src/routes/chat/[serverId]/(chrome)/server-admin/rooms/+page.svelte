<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation, useActiveRoomLayoutUpdated } from '$lib/hooks';
  import { Panel } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import FormDialog from '$lib/ui/FormDialog.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import { Button, TextInput, TextArea } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { untrack } from 'svelte';
  import { dndzone, type DndEvent } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';

  // --- Queries & Mutations ---

  const RoomSetsQuery = graphql(`
    query AdminRoomSets {
      server {
        rooms(type: CHANNEL) {
          id
          name
          description
          archived
          autoJoin
        }
        roomSets {
          id
          name
          rooms {
            id
          }
        }
      }
    }
  `);

  const UpdateRoomSetsMutation = graphql(`
    mutation UpdateRoomSets($input: UpdateRoomSetsInput!) {
      updateRoomSets(input: $input) {
        id
        name
        rooms {
          id
        }
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

  const SetRoomAutoJoinMutation = graphql(`
    mutation SetRoomAutoJoin($input: SetRoomAutoJoinInput!) {
      setRoomAutoJoin(input: $input) {
        id
        autoJoin
      }
    }
  `);

  const layoutQuery = useQuery(RoomSetsQuery, () => ({}));
  const updateLayoutMutation = useMutation(UpdateRoomSetsMutation);
  const updateRoomMutation = useMutation(UpdateRoomMutation);
  const archiveMutation = useMutation(ArchiveRoomMutation);
  const unarchiveMutation = useMutation(UnarchiveRoomMutation);
  const setAutoJoinMutation = useMutation(SetRoomAutoJoinMutation);

  // --- Types ---

  type RoomInfo = { id: string; name: string; description?: string | null; autoJoin: boolean };
  type DndRoomItem = RoomInfo & { id: string };
  type SetState = {
    id: string;
    name: string;
    rooms: DndRoomItem[];
  };

  // --- Local state ---

  let sets = $state<SetState[]>([]);
  let unsorted = $state<DndRoomItem[]>([]);
  let archivedItems = $state<DndRoomItem[]>([]);
  let initialized = $state(false);
  let isDragging = $state(false);
  let lastMutationTimestamp = 0;

  let loading = $derived(layoutQuery.loading);
  let error = $derived(
    layoutQuery.error ??
      (!layoutQuery.loading && !layoutQuery.data?.server ? 'Server not found' : null)
  );

  // Build lookup maps for active and archived rooms. The query asks the
  // server for channels only — `Server.rooms(type: CHANNEL)` — so DM rooms
  // (which the server merges into `Server.rooms` by default for the
  // unified sidebar) are not in the result.
  let allRooms = $derived(layoutQuery.data?.server?.rooms ?? []);
  let activeRoomsMap = $derived(
    new Map<string, RoomInfo>(
      allRooms
        .filter((r) => !r.archived)
        .map((r) => [
          r.id,
          { id: r.id, name: r.name, description: r.description, autoJoin: r.autoJoin }
        ])
    )
  );
  // Server-side archived room IDs (used to detect DnD boundary crossings)
  let archivedRoomIds = $derived(new Set(allRooms.filter((r) => r.archived).map((r) => r.id)));

  // Initialize local state from query data.
  // Only re-runs when layoutQuery.data changes (on refetch).
  // During DnD, no refetch happens, so local state is preserved.
  // Real-time events are debounced by lastMutationTimestamp in the
  // useRoomLayoutUpdated handler, preventing unwanted refetches.
  $effect(() => {
    const space = layoutQuery.data?.server;
    if (!space) return;

    const remoteSets = space.roomSets;

    if (remoteSets && remoteSets.length > 0) {
      sets = remoteSets.map((s) => ({
        id: s.id,
        name: s.name,
        rooms: s.rooms.map((r) => activeRoomsMap.get(r.id)).filter((r): r is RoomInfo => r != null)
      }));

      // Any active rooms not yet placed in a set. Once the room-sets
      // feature is fully wired and the migration runs, this list should
      // always be empty — but during the transition (and for any rooms
      // created before migration) we surface them here so the operator
      // can drag them into a set.
      const idsInSets = new Set(remoteSets.flatMap((s) => s.rooms.map((r) => r.id)));
      unsorted = [...activeRoomsMap.values()]
        .filter((r) => !idsInSets.has(r.id))
        .sort((a, b) => a.name.localeCompare(b.name));
    } else {
      sets = [];
      unsorted = [...activeRoomsMap.values()].sort((a, b) => a.name.localeCompare(b.name));
    }

    // Archived rooms (DnD-compatible)
    archivedItems = allRooms
      .filter((r) => r.archived)
      .map((r) => ({ id: r.id, name: r.name, description: r.description, autoJoin: r.autoJoin }))
      .sort((a, b) => a.name.localeCompare(b.name));

    // Set lastSavedSnapshot from the just-computed local state so it
    // matches layoutSnapshot exactly (avoids a false save on first load).
    // Use untrack to avoid creating dependencies on sets/unsorted
    // (which this effect also writes to — reading them would cause an infinite loop).
    lastSavedSnapshot = untrack(() => layoutSnapshot);

    initialized = true;
  });

  // --- Real-time sync ---

  useActiveRoomLayoutUpdated(() => {
    // Skip refetch during drag or if we just performed a mutation (our own event bouncing back)
    if (isDragging || Date.now() - lastMutationTimestamp < 2000) return;
    layoutQuery.refetch();
  });

  // --- Set creation modal ---

  let createSetDialogVisible = $state(false);
  let newSetName = $state('');

  function openCreateSet() {
    newSetName = '';
    createSetDialogVisible = true;
  }

  function handleCreateSetSubmit(e: Event) {
    e.preventDefault();
    const name = newSetName.trim();
    if (!name) return;

    sets = [
      ...sets,
      {
        id: crypto.randomUUID(),
        name,
        rooms: []
      }
    ];
    newSetName = '';
    createSetDialogVisible = false;
  }

  function renameSet(setId: string, newName: string) {
    const idx = sets.findIndex((s) => s.id === setId);
    if (idx !== -1) {
      sets[idx] = { ...sets[idx], name: newName };
    }
  }

  let deleteSetConfirmDialogVisible = $state(false);
  let deleteSetConfirm = $state<SetState | null>(null);

  function confirmDeleteSet(set: SetState) {
    deleteSetConfirm = set;
    deleteSetConfirmDialogVisible = true;
  }

  function deleteSet() {
    if (!deleteSetConfirm) return;
    const idx = sets.findIndex((s) => s.id === deleteSetConfirm!.id);
    if (idx === -1) return;

    // Move rooms back to unsorted (append at end to preserve existing order)
    const removedRooms = sets[idx].rooms;
    unsorted = [...unsorted, ...removedRooms];
    sets = sets.filter((s) => s.id !== deleteSetConfirm!.id);
    deleteSetConfirmDialogVisible = false;
    deleteSetConfirm = null;
  }

  // --- Drag-and-drop handlers ---

  function handleSetConsider(setId: string, e: CustomEvent<DndEvent<DndRoomItem>>) {
    isDragging = true;
    const idx = sets.findIndex((s) => s.id === setId);
    if (idx !== -1) {
      sets[idx] = { ...sets[idx], rooms: e.detail.items };
    }
  }

  function handleSetFinalize(setId: string, e: CustomEvent<DndEvent<DndRoomItem>>) {
    const idx = sets.findIndex((s) => s.id === setId);
    if (idx !== -1) {
      sets[idx] = { ...sets[idx], rooms: e.detail.items };
    }
    if (!checkBoundaryCrossing()) {
      isDragging = false;
    }
  }

  function handleUnsortedConsider(e: CustomEvent<DndEvent<DndRoomItem>>) {
    isDragging = true;
    unsorted = e.detail.items;
  }

  function handleUnsortedFinalize(e: CustomEvent<DndEvent<DndRoomItem>>) {
    unsorted = e.detail.items;
    if (!checkBoundaryCrossing()) {
      isDragging = false;
    }
  }

  // Drag-and-drop for reordering sets themselves
  type DndSetItem = SetState & { id: string };

  let draggingSetId = $state<string | null>(null);

  function handleSetsConsider(e: CustomEvent<DndEvent<DndSetItem>>) {
    isDragging = true;
    draggingSetId = e.detail.info?.id ?? null;
    sets = e.detail.items;
  }

  function handleSetsFinalize(e: CustomEvent<DndEvent<DndSetItem>>) {
    draggingSetId = null;
    sets = e.detail.items;
    if (!checkBoundaryCrossing()) {
      isDragging = false;
    }
  }

  // Drag-and-drop for the archived zone
  function handleArchivedConsider(e: CustomEvent<DndEvent<DndRoomItem>>) {
    isDragging = true;
    archivedItems = e.detail.items;
  }

  function handleArchivedFinalize(e: CustomEvent<DndEvent<DndRoomItem>>) {
    archivedItems = e.detail.items;
    if (!checkBoundaryCrossing()) {
      // Re-sort alphabetically — reordering within archived is meaningless
      archivedItems = [...archivedItems].sort((a, b) => a.name.localeCompare(b.name));
      isDragging = false;
    }
  }

  /**
   * After any finalize, check if a room crossed the archive boundary.
   * Returns true if a boundary crossing was detected (modal shown, isDragging stays true).
   */
  function checkBoundaryCrossing(): boolean {
    // Skip if already showing a confirmation
    if (archiveConfirmDialogVisible || unarchiveConfirmDialogVisible) return true;

    // Check if a non-archived room was dragged into the archived zone
    const newlyArchived = archivedItems.find((r) => !archivedRoomIds.has(r.id));
    if (newlyArchived) {
      confirmArchiveRoom(newlyArchived, 'dnd');
      return true;
    }

    // Check if an archived room was dragged out of the archived zone
    const currentArchivedIdSet = new Set(archivedItems.map((r) => r.id));
    for (const id of archivedRoomIds) {
      if (!currentArchivedIdSet.has(id)) {
        const room =
          unsorted.find((r) => r.id === id) ??
          sets.flatMap((s) => s.rooms).find((r) => r.id === id);
        if (room) {
          pendingUnarchiveRoom = room;
          unarchiveConfirmDialogVisible = true;
          return true;
        }
      }
    }

    return false;
  }

  // --- Auto-save layout ---

  let layoutSnapshot = $derived(
    JSON.stringify({
      sets: sets.map((s) => ({
        id: s.id,
        name: s.name,
        roomIds: s.rooms.map((r) => r.id)
      }))
    })
  );

  let lastSavedSnapshot = $state<string | null>(null);
  let saveTimer: ReturnType<typeof setTimeout> | undefined;

  $effect(() => {
    void layoutSnapshot; // track changes

    if (!initialized || isDragging) return;
    if (layoutSnapshot === lastSavedSnapshot) return;

    clearTimeout(saveTimer);
    saveTimer = setTimeout(async () => {
      const snapshot = layoutSnapshot;
      const result = await updateLayoutMutation.execute({
        input: {
          sets: sets.map((s) => ({
            id: s.id,
            name: s.name,
            roomIds: s.rooms.map((r) => r.id)
          }))
        }
      });

      if (result.error) {
        toast.error(`Failed to save layout: ${result.error}`);
      } else {
        toast.success('Layout saved');
        lastSavedSnapshot = snapshot;
        lastMutationTimestamp = Date.now();
      }
    }, 500);

    return () => clearTimeout(saveTimer);
  });

  // --- Set rename modal ---

  let editSetDialogVisible = $state(false);
  let editSetId = $state('');
  let editSetName = $state('');

  function openEditSet(set: SetState) {
    editSetId = set.id;
    editSetName = set.name;
    editSetDialogVisible = true;
  }

  function handleEditSetSubmit(e: Event) {
    e.preventDefault();
    if (editSetId && editSetName.trim()) {
      renameSet(editSetId, editSetName.trim());
    }
    editSetDialogVisible = false;
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
  let archiveTrigger = $state<'button' | 'dnd'>('button');

  function confirmArchiveRoom(
    room: { id: string; name: string },
    trigger: 'button' | 'dnd' = 'button'
  ) {
    archiveConfirmRoom = room;
    archiveTrigger = trigger;
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
    isDragging = false;
    lastMutationTimestamp = Date.now();
    layoutQuery.refetch();
  }

  function cancelArchive() {
    archiveConfirmDialogVisible = false;
    archiveConfirmRoom = null;
    if (archiveTrigger === 'dnd') {
      isDragging = false;
      lastMutationTimestamp = Date.now();
      layoutQuery.refetch();
    }
  }

  async function unarchiveRoom(roomId: string) {
    archivingRoomId = roomId;
    const result = await unarchiveMutation.execute({ input: { roomId } });
    archivingRoomId = null;

    if (result.error) {
      toast.error(`Failed to unarchive room: ${result.error}`);
    } else {
      toast.success('Room unarchived');
      lastMutationTimestamp = Date.now();
      layoutQuery.refetch();
    }
  }

  // --- Unarchive confirmation (DnD) ---

  let unarchiveConfirmDialogVisible = $state(false);
  let pendingUnarchiveRoom = $state<{ id: string; name: string } | null>(null);

  async function confirmDndUnarchive() {
    if (!pendingUnarchiveRoom) return;
    const roomId = pendingUnarchiveRoom.id;
    unarchiveConfirmDialogVisible = false;

    const result = await unarchiveMutation.execute({ input: { roomId } });

    if (result.error) {
      toast.error(`Failed to unarchive room: ${result.error}`);
    } else {
      toast.success('Room unarchived');
    }

    pendingUnarchiveRoom = null;
    isDragging = false;
    lastMutationTimestamp = Date.now();
    layoutQuery.refetch();
  }

  function cancelDndUnarchive() {
    unarchiveConfirmDialogVisible = false;
    pendingUnarchiveRoom = null;
    isDragging = false;
    lastMutationTimestamp = Date.now();
    layoutQuery.refetch();
  }

  // --- Auto-join toggle ---

  async function toggleAutoJoin(roomId: string, currentValue: boolean) {
    const result = await setAutoJoinMutation.execute({
      input: { roomId, autoJoin: !currentValue }
    });

    if (result.error) {
      toast.error(`Failed to update auto-join: ${result.error}`);
    } else {
      toast.success(!currentValue ? 'Auto-join enabled' : 'Auto-join disabled');
      lastMutationTimestamp = Date.now();
      layoutQuery.refetch();
    }
  }

  // --- Room creation modal ---

  let createRoomDialogVisible = $state(false);

  function handleRoomCreated() {
    createRoomDialogVisible = false;
    toast.success('Room created');
    lastMutationTimestamp = Date.now();
    layoutQuery.refetch();
  }
</script>

{#snippet roomActions(room: DndRoomItem)}
  <button
    type="button"
    class={[
      'inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs',
      room.autoJoin
        ? 'bg-green-500/10 text-green-600 hover:bg-green-500/20 dark:text-green-400'
        : 'text-muted hover:bg-surface-200 hover:text-text'
    ]}
    title={room.autoJoin
      ? 'New members auto-join this room'
      : 'New members do not auto-join this room'}
    onclick={(e) => {
      e.stopPropagation();
      toggleAutoJoin(room.id, room.autoJoin);
    }}
  >
    <span class={['iconify', room.autoJoin ? 'uil--check-circle' : 'uil--circle']}></span>
    Auto-join
  </button>
  <button
    type="button"
    class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted hover:bg-surface-200 hover:text-text"
    title="Edit room"
    onclick={(e) => {
      e.stopPropagation();
      openEditRoom(room);
    }}
  >
    <span class="iconify uil--pen"></span>
    Edit
  </button>
  <button
    type="button"
    class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted hover:bg-surface-200 hover:text-warning"
    title="Archive room"
    disabled={archivingRoomId === room.id}
    onclick={(e) => {
      e.stopPropagation();
      confirmArchiveRoom(room);
    }}
  >
    <span class="iconify uil--archive"></span>
    Archive
  </button>
{/snippet}

{#snippet archivedRoomActions(room: DndRoomItem)}
  <button
    type="button"
    class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted hover:bg-surface-200 hover:text-text"
    title="Edit room"
    onclick={(e) => {
      e.stopPropagation();
      openEditRoom(room);
    }}
  >
    <span class="iconify uil--pen"></span>
    Edit
  </button>
  <button
    type="button"
    class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted hover:bg-surface-200 hover:text-text"
    title="Unarchive room"
    disabled={archivingRoomId === room.id}
    onclick={(e) => {
      e.stopPropagation();
      unarchiveRoom(room.id);
    }}
  >
    <span class="iconify uil--redo"></span>
    Unarchive
  </button>
{/snippet}

<PageTitle title="Rooms | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Rooms" subtitle="Create, edit, organize, and archive rooms" showMobileNav />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading rooms...</div>
    {:else if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      <!-- Sets & Rooms -->
      <Panel title="Rooms" icon="iconify uil--layers">
        <!-- Action buttons -->
        <div class="mb-4 flex gap-3">
          <Button variant="secondary" onclick={() => (createRoomDialogVisible = true)}>
            <span class="iconify uil--plus"></span>
            New Room
          </Button>
          <Button variant="secondary" onclick={openCreateSet}>
            <span class="iconify uil--layer-group"></span>
            New Set
          </Button>
        </div>

        <p class="mb-4 text-muted">
          Drag rooms between sets to organize them. Drag set headers to reorder sets.
          Drop rooms into Archived to archive them.
        </p>

        <div class="flex flex-col gap-6">
          {#if sets.length > 0}
            <div
              class="flex flex-col gap-6"
              use:dndzone={{
                items: sets,
                flipDurationMs: 200,
                dropTargetStyle: {},
                type: 'sets'
              }}
              onconsider={handleSetsConsider}
              onfinalize={handleSetsFinalize}
            >
              {#each sets as set (set.id)}
                <div
                  animate:flip={{ duration: 200 }}
                  class={[
                    'rounded-lg transition-colors [&:has(>.set-header:hover)]:bg-surface-100',
                    draggingSetId === set.id && 'bg-surface-100'
                  ]}
                >
                  <!-- Set header -->
                  <div class="set-header flex items-center gap-2 px-2 py-2">
                    <span
                      role="button"
                      tabindex="0"
                      class="hover:text-foreground iconify cursor-grab text-muted uil--draggabledots"
                      title="Drag to reorder set"
                      aria-label="Drag to reorder set"
                    >
                    </span>

                    <span class="flex-1 font-semibold">
                      {set.name}
                    </span>

                    <button
                      type="button"
                      class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted hover:bg-surface-200 hover:text-text"
                      title="Rename set"
                      onclick={() => openEditSet(set)}
                    >
                      <span class="iconify uil--pen"></span>
                      Rename
                    </button>
                    <button
                      type="button"
                      class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted hover:bg-surface-200 hover:text-danger"
                      title="Delete set (rooms move to Unsorted)"
                      onclick={() => confirmDeleteSet(set)}
                    >
                      <span class="iconify uil--trash-alt"></span>
                      Delete
                    </button>
                  </div>

                  <!-- Room drop zone -->
                  <div
                    class="min-h-10 pl-8"
                    use:dndzone={{
                      items: set.rooms,
                      flipDurationMs: 200,
                      dropTargetStyle: {
                        outline: '2px dashed var(--color-muted)',
                        'outline-offset': '-2px',
                        'border-radius': '0.5rem',
                        'background-color': 'color-mix(in srgb, var(--color-muted) 5%, transparent)'
                      },
                      type: 'rooms'
                    }}
                    onconsider={(e) => handleSetConsider(set.id, e)}
                    onfinalize={(e) => handleSetFinalize(set.id, e)}
                  >
                    {#each set.rooms as room (room.id)}
                      <div
                        animate:flip={{ duration: 200 }}
                        class="group flex cursor-grab items-start gap-2 rounded px-2 py-1.5 hover:bg-surface-100"
                      >
                        <span class="text-sm text-muted">#</span>
                        <div class="min-w-0 flex-1">
                          <span class="block truncate text-sm">{room.name}</span>
                          {#if room.description}
                            <span class="block truncate text-xs text-muted">{room.description}</span
                            >
                          {/if}
                        </div>
                        {@render roomActions(room)}
                      </div>
                    {:else}
                      <div class="px-2 py-3 text-center text-muted">Drop rooms here</div>
                    {/each}
                  </div>
                </div>
              {/each}
            </div>
          {/if}

          <!-- Unsorted rooms -->
          <div>
            <div class="flex items-center gap-2 px-2 py-2">
              <span class="iconify text-muted uil--inbox"></span>
              <span class="flex-1 font-semibold text-muted">Unsorted</span>
            </div>

            <div
              class="min-h-10 pl-8"
              use:dndzone={{
                items: unsorted,
                flipDurationMs: 200,
                dropTargetStyle: {
                  outline: '2px dashed var(--color-muted)',
                  'outline-offset': '-2px',
                  'border-radius': '0.5rem',
                  'background-color': 'color-mix(in srgb, var(--color-muted) 5%, transparent)'
                },
                type: 'rooms'
              }}
              onconsider={handleUnsortedConsider}
              onfinalize={handleUnsortedFinalize}
            >
              {#each unsorted as room (room.id)}
                <div
                  animate:flip={{ duration: 200 }}
                  class="group flex cursor-grab items-start gap-2 rounded px-2 py-1.5 hover:bg-surface-100"
                >
                  <span class="text-sm text-muted">#</span>
                  <div class="min-w-0 flex-1">
                    <span class="block truncate text-sm">{room.name}</span>
                    {#if room.description}
                      <span class="block truncate text-xs text-muted">{room.description}</span>
                    {/if}
                  </div>
                  {@render roomActions(room)}
                </div>
              {:else}
                <div class="px-2 py-3 text-center text-muted">All rooms are organized</div>
              {/each}
            </div>
          </div>

          <!-- Archived rooms -->
          <div>
            <div class="flex items-center gap-2 px-2 py-2">
              <span class="iconify text-muted uil--archive"></span>
              <span class="flex-1 font-semibold text-muted">Archived</span>
            </div>

            <div
              class="min-h-10 pl-8"
              use:dndzone={{
                items: archivedItems,
                flipDurationMs: 200,
                dropTargetStyle: {
                  outline: '2px dashed var(--color-muted)',
                  'outline-offset': '-2px',
                  'border-radius': '0.5rem',
                  'background-color': 'color-mix(in srgb, var(--color-muted) 5%, transparent)'
                },
                type: 'rooms'
              }}
              onconsider={handleArchivedConsider}
              onfinalize={handleArchivedFinalize}
            >
              {#each archivedItems as room (room.id)}
                <div
                  animate:flip={{ duration: 200 }}
                  class="group flex cursor-grab items-start gap-2 rounded px-2 py-1.5 hover:bg-surface-100"
                >
                  <span class="text-sm text-muted/50">#</span>
                  <div class="min-w-0 flex-1">
                    <span class="block truncate text-sm text-muted">{room.name}</span>
                    {#if room.description}
                      <span class="block truncate text-xs text-muted/50">{room.description}</span>
                    {/if}
                  </div>
                  {@render archivedRoomActions(room)}
                </div>
              {:else}
                <div class="px-2 py-3 text-center text-muted">Drop rooms here to archive them</div>
              {/each}
            </div>
          </div>
        </div>
      </Panel>
    {/if}
  </div>
</div>

<!-- Create Room Dialog -->
<Dialog bind:visible={createRoomDialogVisible} title="Create Room" size="sm">
  {#if createRoomDialogVisible}
    <CreateRoom onroomcreated={handleRoomCreated} />
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

<!-- Create Set Dialog -->
<FormDialog
  bind:visible={createSetDialogVisible}
  title="Create Set"
  size="sm"
  submitLabel="Create Set"
  submitIcon="iconify uil--plus"
  disabled={!newSetName.trim()}
  onsubmit={handleCreateSetSubmit}
  onclose={() => (createSetDialogVisible = false)}
>
  <TextInput
    id="new-set-name"
    label="Set name"
    bind:value={newSetName}
    placeholder="e.g., General, Projects, Teams"
  />
</FormDialog>

<!-- Edit Set Dialog -->
<FormDialog
  bind:visible={editSetDialogVisible}
  title="Rename Set"
  size="sm"
  submitLabel="Save"
  disabled={!editSetName.trim()}
  onsubmit={handleEditSetSubmit}
  onclose={() => (editSetDialogVisible = false)}
>
  <TextInput id="edit-set-name" label="Set name" bind:value={editSetName} />
</FormDialog>

<!-- Delete Set Confirmation Dialog -->
{#if deleteSetConfirmDialogVisible && deleteSetConfirm}
  <ConfirmDialog
    title="Delete Set"
    actionLabel="Delete Set"
    actionIcon="iconify uil--trash-alt"
    onconfirm={deleteSet}
    onclose={() => {
      deleteSetConfirmDialogVisible = false;
      deleteSetConfirm = null;
    }}
  >
    Are you sure you want to delete the set <strong>{deleteSetConfirm.name}</strong>?
    {#if deleteSetConfirm.rooms.length > 0}
      Its {deleteSetConfirm.rooms.length} room{deleteSetConfirm.rooms.length === 1
        ? ''
        : 's'} will be moved to Unsorted.
    {/if}
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

<!-- Unarchive Room Confirmation Dialog (DnD) -->
{#if unarchiveConfirmDialogVisible && pendingUnarchiveRoom}
  <ConfirmDialog
    title="Unarchive Room"
    tone="info"
    actionLabel="Unarchive Room"
    actionIcon="iconify uil--redo"
    onconfirm={confirmDndUnarchive}
    onclose={cancelDndUnarchive}
  >
    Are you sure you want to unarchive <strong>#{pendingUnarchiveRoom.name}</strong>? It will
    become accessible to space members again.
  </ConfirmDialog>
{/if}
