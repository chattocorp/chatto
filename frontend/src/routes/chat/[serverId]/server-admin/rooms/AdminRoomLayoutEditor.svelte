<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import type {
    AdminRoomGroup as GroupState,
    AdminRoomInfo as RoomInfo,
    AdminRoomLayoutStore,
    GroupReorderResult,
    RoomMoveFlushResult
  } from '$lib/state/server/adminRoomLayout.svelte';
  import { EmptyState, Hint, Pill, ToggleChip } from '$lib/ui';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import FormDialog from '$lib/ui/FormDialog.svelte';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import { toast } from '$lib/ui/toast';
  import { flip } from 'svelte/animate';
  import { dndzone, type DndEvent } from 'svelte-dnd-action';

  let {
    layout,
    serverSegment,
    onroomcreated
  }: {
    layout: AdminRoomLayoutStore;
    serverSegment: string;
    onroomcreated?: () => void;
  } = $props();

  type DndRoomItem = RoomInfo & { id: string };
  type DndGroupItem = GroupState & { id: string };

  let renderGroups = $derived(
    layout.groups.map((group) => ({
      ...group,
      rooms: group.rooms ?? []
    }))
  );

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

    const result = await layout.createGroup(name);
    if (!result.ok) {
      toast.error(`Failed to create group: ${result.error}`);
      return;
    }
    newGroupName = '';
    createGroupDialogVisible = false;
    toast.success('Group created');
  }

  async function renameGroup(groupId: string, newName: string) {
    const result = await layout.renameGroup(groupId, newName);
    if (!result.ok) {
      toast.error(`Failed to rename group: ${result.error}`);
      return;
    }
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
    const result = await layout.deleteGroup(target.id);
    deleteGroupConfirmDialogVisible = false;
    deleteGroupConfirm = null;
    if (!result.ok) {
      toast.error(`Failed to delete group: ${result.error}`);
      return;
    }
    toast.success('Group deleted');
  }

  // --- Drag-and-drop handlers ---

  function handleRoomMoveResult(result: RoomMoveFlushResult | null) {
    if (!result) return;
    if (!result.ok) {
      for (const error of result.errors) toast.error(error);
      return;
    }
    if (result.movedCount > 0) {
      toast.success(result.movedCount === 1 ? 'Room moved' : `${result.movedCount} rooms moved`);
    }
  }

  function handleGroupReorderResult(result: GroupReorderResult) {
    if (!result.ok) {
      toast.error(`Failed to reorder groups: ${result.error}`);
    }
  }

  function handleGroupConsider(groupId: string, e: CustomEvent<DndEvent<DndRoomItem>>) {
    layout.handleRoomDragConsider(groupId, e.detail.items);
  }

  async function handleGroupFinalize(groupId: string, e: CustomEvent<DndEvent<DndRoomItem>>) {
    const result = await layout.handleRoomDragFinalize(groupId, e.detail.items);
    handleRoomMoveResult(result);
  }

  function handleGroupsConsider(e: CustomEvent<DndEvent<DndGroupItem>>) {
    layout.handleGroupsConsider(e.detail.items, e.detail.info?.id ?? null);
  }

  async function handleGroupsFinalize(e: CustomEvent<DndEvent<DndGroupItem>>) {
    const result = await layout.handleGroupsFinalize(e.detail.items);
    handleGroupReorderResult(result);
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
      void renameGroup(editGroupId, editGroupName.trim());
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

    const result = await layout.updateRoom(
      editRoomId,
      editRoomName.trim(),
      editRoomDescription.trim() || null
    );

    if (!result.ok) {
      toast.error(`Failed to update room: ${result.error}`);
    } else {
      toast.success('Room updated');
      editRoomDialogVisible = false;
    }
  }

  // --- Room archiving ---

  let archiveConfirmDialogVisible = $state(false);
  let archiveConfirmRoom = $state<{ id: string; name: string } | null>(null);

  function confirmArchiveRoom(room: { id: string; name: string }) {
    archiveConfirmRoom = room;
    archiveConfirmDialogVisible = true;
  }

  async function archiveRoom() {
    if (!archiveConfirmRoom) return;
    const roomId = archiveConfirmRoom.id;
    archiveConfirmDialogVisible = false;
    const result = await layout.archiveRoom(roomId);

    if (!result.ok) {
      toast.error(`Failed to archive room: ${result.error}`);
    } else {
      toast.success('Room archived');
    }

    archiveConfirmRoom = null;
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
    unarchiveConfirmDialogVisible = false;
    const result = await layout.unarchiveRoom(roomId);

    if (!result.ok) {
      toast.error(`Failed to unarchive room: ${result.error}`);
    } else {
      toast.success('Room unarchived');
    }
    unarchiveConfirmRoom = null;
  }

  function cancelUnarchive() {
    unarchiveConfirmDialogVisible = false;
    unarchiveConfirmRoom = null;
  }

  // --- Permissions navigation ---

  function openGroupPermissions(group: GroupState) {
    goto(
      resolve('/chat/[serverId]/server-admin/rooms/group/[groupId]', {
        serverId: serverSegment,
        groupId: group.id
      })
    );
  }

  function openRoomPermissions(room: RoomInfo) {
    goto(
      resolve('/chat/[serverId]/server-admin/rooms/room/[roomId]', {
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
    layout.handleRoomCreated();
    onroomcreated?.();
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
      disabled: layout.archivingRoomId === room.id,
      onclick: () => confirmUnarchiveRoom(room)
    })}
  {:else}
    {@render iconButton({
      icon: 'uil--archive',
      title: 'Archive room',
      tone: 'warning',
      disabled: layout.archivingRoomId === room.id,
      onclick: () => confirmArchiveRoom(room)
    })}
  {/if}
{/snippet}

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Rooms" subtitle="Create, edit, organize, and archive rooms" showMobileNav />

  <div class="flex flex-col gap-4 overflow-y-auto p-6">
    {#if layout.loading}
      <div class="text-muted">Loading rooms...</div>
    {:else if layout.error}
      <Hint tone="danger">{layout.error}</Hint>
    {:else}
      {#if renderGroups.length === 0}
        <EmptyState icon="uil--layer-group" title="No room groups yet">
          Create a set to start organizing rooms.
        </EmptyState>
      {:else}
        <Hint>
          Drag rooms between groups to organize them. Drag group headers to reorder groups. Archived
          rooms stay in their set but are hidden from members.
        </Hint>
      {/if}

      <div
        class="flex flex-col gap-4"
        use:dndzone={{
          items: renderGroups,
          flipDurationMs: 200,
          dropTargetStyle: {},
          type: 'groups'
        }}
        onconsider={handleGroupsConsider}
        onfinalize={handleGroupsFinalize}
      >
        {#each renderGroups as group (group.id)}
          <section
            animate:flip={{ duration: 200 }}
            class={[
              'overflow-hidden rounded-xl border border-border bg-background shadow-md transition-shadow',
              layout.draggingGroupId === group.id && 'shadow-lg ring-1 ring-accent/30'
            ]}
          >
            <header class="group-header flex items-center gap-3 panel-header px-4 py-3">
              <span
                role="button"
                tabindex="0"
                class="iconify shrink-0 cursor-grab text-lg text-muted uil--draggabledots hover:text-text"
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
                    'group flex cursor-grab items-center gap-3 rounded-lg py-2 pr-2 pl-3 hover:bg-surface-100',
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

<Dialog bind:visible={createRoomDialogVisible} title="Create Room" size="sm">
  {#if createRoomDialogVisible && createRoomGroupId}
    <CreateRoom groupId={createRoomGroupId} onroomcreated={handleRoomCreated} />
  {/if}
</Dialog>

<FormDialog
  bind:visible={editRoomDialogVisible}
  title="Edit Room"
  size="sm"
  submitLabel="Save Changes"
  submitLoadingText="Saving..."
  loading={layout.updatingRoom}
  disabled={!editRoomName.trim() || !!editRoomNameError}
  onsubmit={handleEditRoomSubmit}
  onclose={() => (editRoomDialogVisible = false)}
>
  <TextInput
    id="edit-room-name"
    label="Name"
    bind:value={editRoomName}
    required
    disabled={layout.updatingRoom}
    error={editRoomNameError}
  />

  <TextArea
    id="edit-room-description"
    label="Description"
    bind:value={editRoomDescription}
    rows={3}
    disabled={layout.updatingRoom}
    placeholder="Optional description for this room"
  />
</FormDialog>

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

{#if archiveConfirmDialogVisible && archiveConfirmRoom}
  <ConfirmDialog
    title="Archive Room"
    tone="warning"
    actionLabel="Archive Room"
    actionIcon="iconify uil--archive"
    loading={!!layout.archivingRoomId}
    onconfirm={archiveRoom}
    onclose={cancelArchive}
  >
    Are you sure you want to archive <strong>#{archiveConfirmRoom.name}</strong>? Members will no
    longer be able to access this room.
  </ConfirmDialog>
{/if}

{#if unarchiveConfirmDialogVisible && unarchiveConfirmRoom}
  <ConfirmDialog
    title="Unarchive Room"
    tone="warning"
    actionLabel="Unarchive Room"
    actionIcon="iconify uil--redo"
    loading={!!layout.archivingRoomId}
    onconfirm={unarchiveRoom}
    onclose={cancelUnarchive}
  >
    Are you sure you want to unarchive <strong>#{unarchiveConfirmRoom.name}</strong>? Members will
    be able to access it again.
  </ConfirmDialog>
{/if}
