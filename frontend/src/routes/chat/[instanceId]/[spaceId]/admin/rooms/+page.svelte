<script lang="ts">
  import { page } from '$app/state';
  import { graphql } from '$lib/gql';
  import { useQuery, useMutation, useActiveRoomLayoutUpdated } from '$lib/hooks';
  import { Panel } from '$lib/components/admin';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import { Button, TextInput, TextArea } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { untrack } from 'svelte';
  import { dndzone, type DndEvent } from 'svelte-dnd-action';
  import { flip } from 'svelte/animate';

  const spaceId = $derived(page.params.spaceId!);

  // --- Queries & Mutations ---

  const RoomLayoutQuery = graphql(`
    query AdminRoomLayout($spaceId: ID!) {
      space(id: $spaceId) {
        id
        rooms {
          id
          name
          description
          archived
          autoJoin
        }
        roomLayout {
          sections {
            id
            name
            rooms {
              id
            }
          }
          unsectionedRoomIds
        }
      }
    }
  `);

  const UpdateRoomLayoutMutation = graphql(`
    mutation UpdateRoomLayout($input: UpdateRoomLayoutInput!) {
      updateRoomLayout(input: $input) {
        sections {
          id
          name
          rooms {
            id
          }
        }
        unsectionedRoomIds
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

  const layoutQuery = useQuery(RoomLayoutQuery, () => ({ spaceId }));
  const updateLayoutMutation = useMutation(UpdateRoomLayoutMutation);
  const updateRoomMutation = useMutation(UpdateRoomMutation);
  const archiveMutation = useMutation(ArchiveRoomMutation);
  const unarchiveMutation = useMutation(UnarchiveRoomMutation);
  const setAutoJoinMutation = useMutation(SetRoomAutoJoinMutation);

  // --- Types ---

  type RoomInfo = { id: string; name: string; description?: string | null; autoJoin: boolean };
  type DndRoomItem = RoomInfo & { id: string };
  type SectionState = {
    id: string;
    name: string;
    rooms: DndRoomItem[];
  };

  // --- Local state ---

  let sections = $state<SectionState[]>([]);
  let unsorted = $state<DndRoomItem[]>([]);
  let archivedItems = $state<DndRoomItem[]>([]);
  let initialized = $state(false);
  let isDragging = $state(false);
  let lastMutationTimestamp = 0;

  let loading = $derived(layoutQuery.loading);
  let error = $derived(
    layoutQuery.error ??
      (!layoutQuery.loading && !layoutQuery.data?.space ? 'Space not found' : null)
  );

  // Build lookup maps for active and archived rooms
  let allRooms = $derived(layoutQuery.data?.space?.rooms ?? []);
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
    const space = layoutQuery.data?.space;
    if (!space) return;

    const layout = space.roomLayout;

    if (layout) {
      sections = layout.sections.map((s) => ({
        id: s.id,
        name: s.name,
        rooms: s.rooms.map((r) => activeRoomsMap.get(r.id)).filter((r): r is RoomInfo => r != null)
      }));

      // Unsorted = active rooms not in any section, respecting stored order
      const sectionedIds = new Set(layout.sections.flatMap((s) => s.rooms.map((r) => r.id)));
      const unsortedActiveRooms = new Map(
        [...activeRoomsMap.entries()].filter(([id]) => !sectionedIds.has(id))
      );

      if (layout.unsectionedRoomIds.length > 0) {
        // Use stored order, then append new rooms alphabetically
        const ordered: DndRoomItem[] = [];
        // eslint-disable-next-line svelte/prefer-svelte-reactivity -- local computation, not reactive state
        const seen = new Set<string>();
        for (const id of layout.unsectionedRoomIds) {
          const room = unsortedActiveRooms.get(id);
          if (room) {
            ordered.push(room);
            seen.add(id);
          }
        }
        const extra = [...unsortedActiveRooms.values()]
          .filter((r) => !seen.has(r.id))
          .sort((a, b) => a.name.localeCompare(b.name));
        unsorted = [...ordered, ...extra];
      } else {
        unsorted = [...unsortedActiveRooms.values()].sort((a, b) => a.name.localeCompare(b.name));
      }
    } else {
      sections = [];
      unsorted = [...activeRoomsMap.values()].sort((a, b) => a.name.localeCompare(b.name));
    }

    // Archived rooms (DnD-compatible)
    archivedItems = allRooms
      .filter((r) => r.archived)
      .map((r) => ({ id: r.id, name: r.name, description: r.description, autoJoin: r.autoJoin }))
      .sort((a, b) => a.name.localeCompare(b.name));

    // Set lastSavedSnapshot from the just-computed local state so it
    // matches layoutSnapshot exactly (avoids false save on first load
    // when the server has no stored unsectionedRoomIds yet).
    // Use untrack to avoid creating dependencies on sections/unsorted
    // (which this effect also writes to — reading them would cause an infinite loop).
    lastSavedSnapshot = untrack(() => layoutSnapshot);

    initialized = true;
  });

  // --- Real-time sync ---

  useActiveRoomLayoutUpdated(({ spaceId: eventSpaceId }) => {
    if (eventSpaceId !== spaceId) return;
    // Skip refetch during drag or if we just performed a mutation (our own event bouncing back)
    if (isDragging || Date.now() - lastMutationTimestamp < 2000) return;
    layoutQuery.refetch();
  });

  // --- Section creation modal ---

  let createSectionDialogVisible = $state(false);
  let newSectionName = $state('');

  function openCreateSection() {
    newSectionName = '';
    createSectionDialogVisible = true;
  }

  function handleCreateSectionSubmit(e: Event) {
    e.preventDefault();
    const name = newSectionName.trim();
    if (!name) return;

    sections = [
      ...sections,
      {
        id: crypto.randomUUID(),
        name,
        rooms: []
      }
    ];
    newSectionName = '';
    createSectionDialogVisible = false;
  }

  function renameSection(sectionId: string, newName: string) {
    const idx = sections.findIndex((s) => s.id === sectionId);
    if (idx !== -1) {
      sections[idx] = { ...sections[idx], name: newName };
    }
  }

  let deleteSectionConfirmDialogVisible = $state(false);
  let deleteSectionConfirm = $state<SectionState | null>(null);

  function confirmDeleteSection(section: SectionState) {
    deleteSectionConfirm = section;
    deleteSectionConfirmDialogVisible = true;
  }

  function deleteSection() {
    if (!deleteSectionConfirm) return;
    const idx = sections.findIndex((s) => s.id === deleteSectionConfirm!.id);
    if (idx === -1) return;

    // Move rooms back to unsorted (append at end to preserve existing order)
    const removedRooms = sections[idx].rooms;
    unsorted = [...unsorted, ...removedRooms];
    sections = sections.filter((s) => s.id !== deleteSectionConfirm!.id);
    deleteSectionConfirmDialogVisible = false;
    deleteSectionConfirm = null;
  }

  // --- Drag-and-drop handlers ---

  function handleSectionConsider(sectionId: string, e: CustomEvent<DndEvent<DndRoomItem>>) {
    isDragging = true;
    const idx = sections.findIndex((s) => s.id === sectionId);
    if (idx !== -1) {
      sections[idx] = { ...sections[idx], rooms: e.detail.items };
    }
  }

  function handleSectionFinalize(sectionId: string, e: CustomEvent<DndEvent<DndRoomItem>>) {
    const idx = sections.findIndex((s) => s.id === sectionId);
    if (idx !== -1) {
      sections[idx] = { ...sections[idx], rooms: e.detail.items };
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

  // Drag-and-drop for reordering sections themselves
  type DndSectionItem = SectionState & { id: string };

  let draggingSectionId = $state<string | null>(null);

  function handleSectionsConsider(e: CustomEvent<DndEvent<DndSectionItem>>) {
    isDragging = true;
    draggingSectionId = e.detail.info?.id ?? null;
    sections = e.detail.items;
  }

  function handleSectionsFinalize(e: CustomEvent<DndEvent<DndSectionItem>>) {
    draggingSectionId = null;
    sections = e.detail.items;
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
          sections.flatMap((s) => s.rooms).find((r) => r.id === id);
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
      sections: sections.map((s) => ({
        id: s.id,
        name: s.name,
        roomIds: s.rooms.map((r) => r.id)
      })),
      unsectionedRoomIds: unsorted.map((r) => r.id)
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
          spaceId,
          sections: sections.map((s) => ({
            id: s.id,
            name: s.name,
            roomIds: s.rooms.map((r) => r.id)
          })),
          unsectionedRoomIds: unsorted.map((r) => r.id)
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

  // --- Section rename modal ---

  let editSectionDialogVisible = $state(false);
  let editSectionId = $state('');
  let editSectionName = $state('');

  function openEditSection(section: SectionState) {
    editSectionId = section.id;
    editSectionName = section.name;
    editSectionDialogVisible = true;
  }

  function handleEditSectionSubmit(e: Event) {
    e.preventDefault();
    if (editSectionId && editSectionName.trim()) {
      renameSection(editSectionId, editSectionName.trim());
    }
    editSectionDialogVisible = false;
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
        spaceId,
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
    const result = await archiveMutation.execute({ input: { spaceId, roomId } });
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
    const result = await unarchiveMutation.execute({ input: { spaceId, roomId } });
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

    const result = await unarchiveMutation.execute({ input: { spaceId, roomId } });

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
      input: { spaceId, roomId, autoJoin: !currentValue }
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
      <Hint variant="danger">{error}</Hint>
    {:else}
      <!-- Sections & Rooms -->
      <Panel title="Rooms" icon="iconify uil--layers">
        <!-- Action buttons -->
        <div class="mb-4 flex gap-3">
          <Button variant="secondary" onclick={() => (createRoomDialogVisible = true)}>
            <span class="iconify uil--plus"></span>
            New Room
          </Button>
          <Button variant="secondary" onclick={openCreateSection}>
            <span class="iconify uil--layer-group"></span>
            New Section
          </Button>
        </div>

        <p class="mb-4 text-muted">
          Drag rooms between sections to organize them. Drag section headers to reorder sections.
          Drop rooms into Archived to archive them.
        </p>

        <div class="flex flex-col gap-6">
          {#if sections.length > 0}
            <div
              class="flex flex-col gap-6"
              use:dndzone={{
                items: sections,
                flipDurationMs: 200,
                dropTargetStyle: {},
                type: 'sections'
              }}
              onconsider={handleSectionsConsider}
              onfinalize={handleSectionsFinalize}
            >
              {#each sections as section (section.id)}
                <div
                  animate:flip={{ duration: 200 }}
                  class={[
                    'rounded-lg transition-colors [&:has(>.section-header:hover)]:bg-surface-100',
                    draggingSectionId === section.id && 'bg-surface-100'
                  ]}
                >
                  <!-- Section header -->
                  <div class="section-header flex items-center gap-2 px-2 py-2">
                    <span
                      role="button"
                      tabindex="0"
                      class="hover:text-foreground iconify cursor-grab text-muted uil--draggabledots"
                      title="Drag to reorder section"
                      aria-label="Drag to reorder section"
                    >
                    </span>

                    <span class="flex-1 font-semibold">
                      {section.name}
                    </span>

                    <button
                      type="button"
                      class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted hover:bg-surface-200 hover:text-text"
                      title="Rename section"
                      onclick={() => openEditSection(section)}
                    >
                      <span class="iconify uil--pen"></span>
                      Rename
                    </button>
                    <button
                      type="button"
                      class="inline-flex cursor-pointer items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted hover:bg-surface-200 hover:text-danger"
                      title="Delete section (rooms move to Unsorted)"
                      onclick={() => confirmDeleteSection(section)}
                    >
                      <span class="iconify uil--trash-alt"></span>
                      Delete
                    </button>
                  </div>

                  <!-- Room drop zone -->
                  <div
                    class="min-h-10 pl-8"
                    use:dndzone={{
                      items: section.rooms,
                      flipDurationMs: 200,
                      dropTargetStyle: {
                        outline: '2px dashed var(--color-muted)',
                        'outline-offset': '-2px',
                        'border-radius': '0.5rem',
                        'background-color': 'color-mix(in srgb, var(--color-muted) 5%, transparent)'
                      },
                      type: 'rooms'
                    }}
                    onconsider={(e) => handleSectionConsider(section.id, e)}
                    onfinalize={(e) => handleSectionFinalize(section.id, e)}
                  >
                    {#each section.rooms as room (room.id)}
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
  <CreateRoom {spaceId} onroomcreated={handleRoomCreated} />
</Dialog>

<!-- Edit Room Dialog -->
<Dialog bind:visible={editRoomDialogVisible} title="Edit Room" size="sm">
  <form onsubmit={handleEditRoomSubmit} class="flex flex-col gap-4">
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

    <div class="flex items-center gap-3">
      <Button
        type="submit"
        loading={updateRoomMutation.loading}
        disabled={!editRoomName.trim() || !!editRoomNameError}
        loadingText="Saving..."
      >
        <span class="iconify uil--check"></span>
        Save Changes
      </Button>
      <Button variant="ghost" onclick={() => (editRoomDialogVisible = false)}>
        <span class="iconify uil--times"></span>
        Cancel
      </Button>
    </div>
  </form>
</Dialog>

<!-- Create Section Dialog -->
<Dialog bind:visible={createSectionDialogVisible} title="Create Section" size="sm">
  <form onsubmit={handleCreateSectionSubmit} class="flex flex-col gap-4">
    <TextInput
      id="new-section-name"
      label="Section name"
      bind:value={newSectionName}
      placeholder="e.g., General, Projects, Teams"
    />

    <div class="flex items-center gap-3">
      <Button type="submit" disabled={!newSectionName.trim()}>
        <span class="iconify uil--plus"></span>
        Create Section
      </Button>
      <Button variant="ghost" onclick={() => (createSectionDialogVisible = false)}>
        <span class="iconify uil--times"></span>
        Cancel
      </Button>
    </div>
  </form>
</Dialog>

<!-- Edit Section Dialog -->
<Dialog bind:visible={editSectionDialogVisible} title="Rename Section" size="sm">
  <form onsubmit={handleEditSectionSubmit} class="flex flex-col gap-4">
    <TextInput id="edit-section-name" label="Section name" bind:value={editSectionName} />

    <div class="flex items-center gap-3">
      <Button type="submit" disabled={!editSectionName.trim()}>
        <span class="iconify uil--check"></span>
        Save
      </Button>
      <Button variant="ghost" onclick={() => (editSectionDialogVisible = false)}>
        <span class="iconify uil--times"></span>
        Cancel
      </Button>
    </div>
  </form>
</Dialog>

<!-- Delete Section Confirmation Dialog -->
<Dialog bind:visible={deleteSectionConfirmDialogVisible} title="Delete Section" size="sm">
  <p class="mb-4">
    Are you sure you want to delete the section <strong>{deleteSectionConfirm?.name}</strong>?
    {#if deleteSectionConfirm && deleteSectionConfirm.rooms.length > 0}
      Its {deleteSectionConfirm.rooms.length} room{deleteSectionConfirm.rooms.length === 1
        ? ''
        : 's'} will be moved to Unsorted.
    {/if}
  </p>

  <div class="flex items-center gap-3">
    <Button variant="danger" onclick={deleteSection}>
      <span class="iconify uil--trash-alt"></span>
      Delete Section
    </Button>
    <Button variant="ghost" onclick={() => (deleteSectionConfirmDialogVisible = false)}>
      <span class="iconify uil--times"></span>
      Cancel
    </Button>
  </div>
</Dialog>

<!-- Archive Room Confirmation Dialog -->
<Dialog bind:visible={archiveConfirmDialogVisible} title="Archive Room" size="sm">
  <p class="mb-4">
    Are you sure you want to archive <strong>#{archiveConfirmRoom?.name}</strong>? Members will no
    longer be able to access this room.
  </p>

  <div class="flex items-center gap-3">
    <Button
      variant="danger"
      onclick={archiveRoom}
      loading={!!archivingRoomId}
      loadingText="Archiving..."
    >
      <span class="iconify uil--archive"></span>
      Archive Room
    </Button>
    <Button variant="ghost" onclick={cancelArchive}>
      <span class="iconify uil--times"></span>
      Cancel
    </Button>
  </div>
</Dialog>

<!-- Unarchive Room Confirmation Dialog (DnD) -->
<Dialog bind:visible={unarchiveConfirmDialogVisible} title="Unarchive Room" size="sm">
  <p class="mb-4">
    Are you sure you want to unarchive <strong>#{pendingUnarchiveRoom?.name}</strong>? It will
    become accessible to space members again.
  </p>

  <div class="flex items-center gap-3">
    <Button onclick={confirmDndUnarchive}>
      <span class="iconify uil--redo"></span>
      Unarchive Room
    </Button>
    <Button variant="ghost" onclick={cancelDndUnarchive}>
      <span class="iconify uil--times"></span>
      Cancel
    </Button>
  </div>
</Dialog>
