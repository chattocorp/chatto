<script lang="ts">
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation, useActiveRoomLayoutUpdated } from '$lib/hooks';
  import { EmptyState, Hint, Pill, ToggleChip } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import FormDialog from '$lib/ui/FormDialog.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
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
          isGlobal
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

  const SetRoomGlobalMutation = graphql(`
    mutation SetRoomGlobal($input: SetRoomGlobalInput!) {
      setRoomGlobal(input: $input) {
        id
        isGlobal
      }
    }
  `);

  const layoutQuery = useQuery(RoomSetsQuery, () => ({}));
  const updateLayoutMutation = useMutation(UpdateRoomSetsMutation);
  const updateRoomMutation = useMutation(UpdateRoomMutation);
  const archiveMutation = useMutation(ArchiveRoomMutation);
  const unarchiveMutation = useMutation(UnarchiveRoomMutation);
  const setGlobalMutation = useMutation(SetRoomGlobalMutation);

  // --- Types ---

  type RoomInfo = {
    id: string;
    name: string;
    description?: string | null;
    isGlobal: boolean;
    archived: boolean;
  };
  type DndRoomItem = RoomInfo & { id: string };
  type SetState = {
    id: string;
    name: string;
    rooms: DndRoomItem[];
  };

  // --- Local state ---

  let sets = $state<SetState[]>([]);
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
          isGlobal: r.isGlobal,
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

    sets = space.roomSets.map((s) => ({
      id: s.id,
      name: s.name,
      rooms: s.rooms.map((r) => roomsMap.get(r.id)).filter((r): r is RoomInfo => r != null)
    }));

    // Set lastSavedSnapshot from the just-computed local state so it
    // matches layoutSnapshot exactly (avoids a false save on first load).
    // Use untrack to avoid creating dependencies on sets (which this
    // effect also writes to — reading would cause an infinite loop).
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
    isDragging = false;
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
    isDragging = false;
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

  // --- Global toggle ---

  async function toggleGlobal(roomId: string, currentValue: boolean) {
    const result = await setGlobalMutation.execute({
      input: { roomId, isGlobal: !currentValue }
    });

    if (result.error) {
      toast.error(`Failed to update global flag: ${result.error}`);
    } else {
      toast.success(!currentValue ? 'Room marked as global' : 'Room is no longer global');
      lastMutationTimestamp = Date.now();
      layoutQuery.refetch();
    }
  }

  // --- Permissions dialog (set or room scope) ---

  let permissionsDialogVisible = $state(false);
  let permissionsScope = $state<
    { kind: 'set'; set: SetState } | { kind: 'room'; room: RoomInfo } | null
  >(null);

  function openSetPermissions(set: SetState) {
    permissionsScope = { kind: 'set', set };
    permissionsDialogVisible = true;
  }

  function openRoomPermissions(room: RoomInfo) {
    permissionsScope = { kind: 'room', room };
    permissionsDialogVisible = true;
  }

  // --- Room creation modal ---

  let createRoomDialogVisible = $state(false);
  let createRoomSetId = $state<string | null>(null);

  function openCreateRoom(set: SetState) {
    createRoomSetId = set.id;
    createRoomDialogVisible = true;
  }

  function handleRoomCreated() {
    createRoomDialogVisible = false;
    createRoomSetId = null;
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
  tone?: 'primary' | 'warning' | 'danger';
})}
  <ToggleChip
    tone={opts.tone ?? 'primary'}
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
      {#if sets.length === 0}
        <EmptyState icon="uil--layer-group" title="No room sets yet">
          Create a set to start organizing rooms.
        </EmptyState>
      {:else}
        <p class="text-sm text-muted">
          Drag rooms between sets to organize them. Drag set headers to reorder sets.
          Archived rooms stay in their set but are hidden from members.
        </p>
      {/if}

      <div
        class="flex flex-col gap-4"
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
          <section
            animate:flip={{ duration: 200 }}
            class={[
              'overflow-hidden rounded-xl border border-border bg-background transition-shadow',
              draggingSetId === set.id && 'shadow-lg ring-1 ring-accent/30'
            ]}
          >
            <!-- Set header -->
            <header
              class="set-header flex items-center gap-3 border-b border-border px-4 py-3"
            >
              <span
                role="button"
                tabindex="0"
                class="iconify shrink-0 cursor-grab text-lg text-muted hover:text-text uil--draggabledots"
                title="Drag to reorder set"
                aria-label="Drag to reorder set"
              ></span>

              <div class="flex min-w-0 flex-1 items-center gap-2">
                <h2 class="truncate text-lg font-semibold">{set.name}</h2>
                <Pill tone="muted">{set.rooms.length}</Pill>
              </div>

              <div class="flex items-center gap-2">
                <Button variant="secondary" size="sm" onclick={() => openCreateRoom(set)}>
                  <span class="iconify uil--plus"></span>
                  New Room
                </Button>
                <div class="flex items-center gap-1.5">
                  {@render iconButton({
                    icon: 'uil--pen',
                    title: 'Rename set',
                    onclick: () => openEditSet(set)
                  })}
                  {@render iconButton({
                    icon: 'uil--shield',
                    title: 'Set permissions',
                    onclick: () => openSetPermissions(set)
                  })}
                  {@render iconButton({
                    icon: 'uil--trash-alt',
                    title:
                      set.rooms.length === 0
                        ? 'Delete set'
                        : 'Move all rooms out of this set before deleting',
                    tone: 'danger',
                    disabled: set.rooms.length > 0,
                    onclick: () => confirmDeleteSet(set)
                  })}
                </div>
              </div>
            </header>

            <!-- Room drop zone -->
            <div
              class="min-h-12 p-2"
              use:dndzone={{
                items: set.rooms,
                flipDurationMs: 200,
                dropTargetStyle: {
                  outline: '2px dashed var(--color-accent)',
                  'outline-offset': '-2px',
                  'border-radius': '0.5rem',
                  'background-color': 'color-mix(in srgb, var(--color-accent) 5%, transparent)'
                },
                type: 'rooms'
              }}
              onconsider={(e) => handleSetConsider(set.id, e)}
              onfinalize={(e) => handleSetFinalize(set.id, e)}
            >
              {#each set.rooms as room (room.id)}
                <div
                  animate:flip={{ duration: 200 }}
                  class={[
                    'group flex cursor-grab items-center gap-3 rounded-lg px-3 py-2 hover:bg-surface-100',
                    room.archived && 'opacity-60'
                  ]}
                >
                  <div class="min-w-0 flex-1">
                    <div class="flex min-w-0 items-baseline gap-2">
                      {#if room.isGlobal}
                        <span
                          class="iconify text-base text-muted uil--globe"
                          title="Global room"
                          aria-label="Global room"
                        ></span>
                      {:else}
                        <span class="text-lg text-muted">#</span>
                      {/if}
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
                    {#if !room.archived}
                      <ToggleChip
                        pressed={room.isGlobal}
                        tone="success"
                        square
                        title={room.isGlobal
                          ? 'Global room — all server members are members'
                          : 'Make this room global (all server members get implicit membership)'}
                        onclick={() => toggleGlobal(room.id, room.isGlobal)}
                      >
                        <span class="iconify text-base uil--globe" aria-label="Global"></span>
                      </ToggleChip>
                    {/if}
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
        <Button variant="secondary" onclick={openCreateSet}>
          <span class="iconify uil--plus"></span>
          New Set
        </Button>
      </div>
    {/if}
  </div>
</div>

<!-- Create Room Dialog -->
<Dialog bind:visible={createRoomDialogVisible} title="Create Room" size="sm">
  {#if createRoomDialogVisible && createRoomSetId}
    <CreateRoom setId={createRoomSetId} onroomcreated={handleRoomCreated} />
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

<!-- Permissions Dialog (set or room scope) -->
<Dialog
  bind:visible={permissionsDialogVisible}
  title={permissionsScope
    ? permissionsScope.kind === 'set'
      ? `Permissions — ${permissionsScope.set.name}`
      : `Permissions — #${permissionsScope.room.name}`
    : 'Permissions'}
  size="lg"
>
  {#if permissionsDialogVisible && permissionsScope}
    {#if permissionsScope.kind === 'set'}
      <PermissionMatrix setId={permissionsScope.set.id} />
    {:else}
      <PermissionMatrix roomId={permissionsScope.room.id} />
    {/if}
  {/if}
</Dialog>

