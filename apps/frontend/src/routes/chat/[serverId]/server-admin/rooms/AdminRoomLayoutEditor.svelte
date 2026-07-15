<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import type {
    AdminRoomGroup as GroupState,
    AdminRoomInfo as RoomInfo,
    AdminRoomLayoutStore,
    AdminSidebarItem,
    AdminSidebarLinkInfo,
    GroupReorderResult,
    RoomMoveFlushResult
  } from '$lib/state/server/adminRoomLayout.svelte';
  import { EmptyState, Hint, Pill, ToggleChip } from '$lib/ui';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import FormDialog from '$lib/ui/FormDialog.svelte';
  import { Button, Checkbox, TextArea, TextInput } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import { toast } from '$lib/ui/toast';
  import { UNIVERSAL_ROOM_HELP_TEXT } from '$lib/utils/roomCopy';
  import { flip } from 'svelte/animate';
  import { dndzone, type DndEvent } from 'svelte-dnd-action';
  import * as m from '$lib/i18n/messages';

  let {
    layout,
    serverSegment,
    onroomcreated
  }: {
    layout: AdminRoomLayoutStore;
    serverSegment: string;
    onroomcreated?: () => void;
  } = $props();

  type DndRoomItem = AdminSidebarItem & { id: string };
  type DndGroupItem = GroupState & { id: string };

  let renderGroups = $derived(
    layout.groups.map((group) => ({
      ...group,
      rooms: group.rooms ?? [],
      items: group.items ?? []
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
      toast.error(m['admin.rooms_admin.create_group_failed']({ error: result.error }));
      return;
    }
    newGroupName = '';
    createGroupDialogVisible = false;
    toast.success(m['admin.rooms_admin.group_created']());
  }

  async function renameGroup(groupId: string, newName: string) {
    const result = await layout.renameGroup(groupId, newName);
    if (!result.ok) {
      toast.error(m['admin.rooms_admin.rename_group_failed']({ error: result.error }));
      return;
    }
    toast.success(m['admin.rooms_admin.group_renamed']());
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
      toast.error(m['admin.rooms_admin.delete_group_failed']({ error: result.error }));
      return;
    }
    toast.success(m['admin.rooms_admin.group_deleted']());
  }

  // --- Drag-and-drop handlers ---

  function handleRoomMoveResult(result: RoomMoveFlushResult | null) {
    if (!result) return;
    if (!result.ok) {
      for (const error of result.errors) toast.error(error);
      return;
    }
    if (result.movedCount > 0) {
      toast.success(
        result.movedCount === 1
          ? m['admin.rooms_admin.item_moved_one']()
          : m['admin.rooms_admin.item_moved_many']({ count: result.movedCount })
      );
    }
  }

  function handleGroupReorderResult(result: GroupReorderResult) {
    if (!result.ok) {
      toast.error(m['admin.rooms_admin.reorder_groups_failed']({ error: result.error }));
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
  let editRoomUniversal = $state(false);
  let editRoomOriginalName = $state('');
  let editRoomOriginalDescription = $state('');
  let editRoomOriginalUniversal = $state(false);

  let editRoomNameError = $derived.by(() => {
    if (!editRoomName) return undefined;
    if (editRoomName.trim() === '') return m['admin.rooms_admin.room_name_empty']();
    if (editRoomName !== editRoomName.trim()) return m['admin.rooms_admin.room_name_trim']();
    if (!/^[a-zA-Z0-9_-]+$/.test(editRoomName.trim())) {
      return m['admin.rooms_admin.room_name_charset']();
    }
    if (editRoomName.length > 30) {
      return m['admin.rooms_admin.room_name_too_long']();
    }
    return undefined;
  });

  let editRoomMetadataChanged = $derived(
    editRoomName.trim() !== editRoomOriginalName ||
      editRoomDescription.trim() !== editRoomOriginalDescription
  );
  let editRoomUniversalChanged = $derived(editRoomUniversal !== editRoomOriginalUniversal);
  let editRoomChanged = $derived(editRoomMetadataChanged || editRoomUniversalChanged);
  let editRoomSaving = $derived(layout.updatingRoom || layout.universalRoomId === editRoomId);

  function openEditRoom(room: RoomInfo) {
    editRoomId = room.id;
    editRoomName = room.name;
    editRoomDescription = room.description ?? '';
    editRoomUniversal = room.isUniversal;
    editRoomOriginalName = room.name;
    editRoomOriginalDescription = room.description ?? '';
    editRoomOriginalUniversal = room.isUniversal;
    editRoomDialogVisible = true;
  }

  async function handleEditRoomSubmit(e: Event) {
    e.preventDefault();
    if (editRoomNameError || !editRoomName.trim()) return;

    if (editRoomMetadataChanged) {
      const result = await layout.updateRoom(
        editRoomId,
        editRoomName.trim(),
        editRoomDescription.trim() || null
      );
      if (!result.ok) {
        toast.error(m['admin.rooms_admin.update_room_failed']({ error: result.error }));
        return;
      }
    }

    if (editRoomUniversalChanged) {
      const result = await layout.updateRoomUniversal(editRoomId, editRoomUniversal);
      if (!result.ok) {
        toast.error(m['admin.rooms_admin.update_room_failed']({ error: result.error }));
        return;
      }
    }

    toast.success(m['admin.rooms_admin.room_updated']());
    editRoomDialogVisible = false;
  }

  // --- Room archiving ---

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
      toast.error(m['admin.rooms_admin.unarchive_room_failed']({ error: result.error }));
    } else {
      toast.success(m['admin.rooms_admin.room_unarchived']());
    }
    unarchiveConfirmRoom = null;
  }

  function cancelUnarchive() {
    unarchiveConfirmDialogVisible = false;
    unarchiveConfirmRoom = null;
  }

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
      toast.error(m['admin.rooms_admin.archive_room_failed']({ error: result.error }));
    } else {
      toast.success(m['admin.rooms_admin.room_archived']());
    }

    archiveConfirmRoom = null;
  }

  function cancelArchive() {
    archiveConfirmDialogVisible = false;
    archiveConfirmRoom = null;
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
    toast.success(m['admin.rooms_admin.room_created']());
    layout.handleRoomCreated();
    onroomcreated?.();
  }

  // --- Sidebar link editing ---

  let linkDialogVisible = $state(false);
  let editingLinkId = $state<string | null>(null);
  let linkGroupId = $state<string | null>(null);
  let linkLabel = $state('');
  let linkUrl = $state('');

  function openCreateLink(group: GroupState) {
    editingLinkId = null;
    linkGroupId = group.id;
    linkLabel = '';
    linkUrl = '';
    linkDialogVisible = true;
  }

  function openEditLink(link: AdminSidebarLinkInfo) {
    editingLinkId = link.id;
    linkGroupId = null;
    linkLabel = link.label;
    linkUrl = link.url;
    linkDialogVisible = true;
  }

  async function handleLinkSubmit(e: Event) {
    e.preventDefault();
    const label = linkLabel.trim();
    const url = linkUrl.trim();
    if (!label || !url) return;

    const result = editingLinkId
      ? await layout.updateSidebarLink(editingLinkId, label, url)
      : linkGroupId
        ? await layout.createSidebarLink(linkGroupId, label, url)
        : { ok: false as const, error: 'No group selected' };

    if (!result.ok) {
      toast.error(m['admin.rooms_admin.save_link_failed']({ error: result.error }));
      return;
    }

    toast.success(
      editingLinkId ? m['admin.rooms_admin.link_updated']() : m['admin.rooms_admin.link_created']()
    );
    linkDialogVisible = false;
  }

  let deleteLinkConfirmDialogVisible = $state(false);
  let deleteLinkConfirm = $state<AdminSidebarLinkInfo | null>(null);

  function confirmDeleteLink(link: AdminSidebarLinkInfo) {
    deleteLinkConfirm = link;
    deleteLinkConfirmDialogVisible = true;
  }

  async function deleteLink() {
    if (!deleteLinkConfirm) return;
    const result = await layout.deleteSidebarLink(deleteLinkConfirm.id);
    deleteLinkConfirmDialogVisible = false;
    deleteLinkConfirm = null;
    if (!result.ok) {
      toast.error(m['admin.rooms_admin.delete_link_failed']({ error: result.error }));
      return;
    }
    toast.success(m['admin.rooms_admin.link_deleted']());
  }
</script>

{#snippet iconButton(opts: {
  icon: string;
  title: string;
  onclick: () => void;
  disabled?: boolean;
  tone?: 'neutral' | 'warning' | 'danger';
  pressed?: boolean;
})}
  <ToggleChip
    tone={opts.tone ?? 'neutral'}
    square
    pressed={opts.pressed}
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
  {#if room.kind === 'room'}
    {@const roomInfo = room.room}
    {@render iconButton({
      icon: 'uil--pen',
      title: m['admin.rooms_admin.edit_room_action'](),
      onclick: () => openEditRoom(roomInfo)
    })}
    {@render iconButton({
      icon: 'uil--shield',
      title: m['admin.rooms_admin.room_permissions_title_fallback'](),
      onclick: () => openRoomPermissions(roomInfo)
    })}
    {#if roomInfo.archived}
      {@render iconButton({
        icon: 'uil--redo',
        title: m['admin.rooms_admin.unarchive_room'](),
        disabled: layout.archivingRoomId === roomInfo.id,
        onclick: () => confirmUnarchiveRoom(roomInfo)
      })}
    {:else}
      {@render iconButton({
        icon: 'uil--archive',
        title: m['admin.rooms_admin.archive_room'](),
        tone: 'warning',
        disabled: layout.archivingRoomId === roomInfo.id,
        onclick: () => confirmArchiveRoom(roomInfo)
      })}
    {/if}
  {:else}
    {@render iconButton({
      icon: 'uil--pen',
      title: m['admin.rooms_admin.edit_link'](),
      onclick: () => openEditLink(room.link)
    })}
    {@render iconButton({
      icon: 'uil--trash-alt',
      title: m['admin.rooms_admin.delete_link'](),
      tone: 'danger',
      onclick: () => confirmDeleteLink(room.link)
    })}
  {/if}
{/snippet}

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.rooms_admin.title']()}
    subtitle={m['admin.rooms_admin.subtitle']()}
    showMobileNav
  />

  <div class="flex flex-col gap-4 overflow-y-auto p-6">
    {#if layout.loading}
      <div class="text-muted">{m['admin.rooms_admin.loading']()}</div>
    {:else if layout.error}
      <Hint tone="danger">{layout.error}</Hint>
    {:else}
      {#if renderGroups.length === 0}
        <EmptyState icon="uil--layer-group" title={m['admin.rooms_admin.empty_groups']()}>
          {m['admin.rooms_admin.empty_groups_body']()}
        </EmptyState>
      {:else}
        <Hint>{m['admin.rooms_admin.drag_hint']()}</Hint>
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
              'overflow-hidden panel-shell panel-shell-raised transition-shadow',
              layout.draggingGroupId === group.id && 'shadow-lg ring-1 ring-action/30'
            ]}
          >
            <header class="group-header flex items-center gap-3 panel-header px-4 py-3">
              <span
                role="button"
                tabindex="0"
                class="iconify shrink-0 cursor-grab text-lg text-muted uil--draggabledots hover:text-text"
                title={m['admin.rooms_admin.drag_group']()}
                aria-label={m['admin.rooms_admin.drag_group']()}
              ></span>

              <div class="flex min-w-0 flex-1 items-center gap-2">
                <h2 class="truncate text-lg font-semibold">{group.name}</h2>
                <Pill tone="muted">{group.items.length}</Pill>
              </div>

              <div class="flex items-center gap-2">
                {#if group.canCreateRoom}
                  <Button variant="secondary" size="sm" onclick={() => openCreateRoom(group)}>
                    <span class="iconify uil--plus"></span>
                    {m['admin.rooms_admin.new_room']()}
                  </Button>
                {/if}
                <Button variant="secondary" size="sm" onclick={() => openCreateLink(group)}>
                  <span class="iconify uil--external-link-alt"></span>
                  {m['admin.rooms_admin.new_link']()}
                </Button>
                <div class="flex items-center gap-1.5">
                  {@render iconButton({
                    icon: 'uil--pen',
                    title: m['admin.rooms_admin.rename_group_action'](),
                    onclick: () => openEditGroup(group)
                  })}
                  {@render iconButton({
                    icon: 'uil--shield',
                    title: m['admin.rooms_admin.group_permissions'](),
                    onclick: () => openGroupPermissions(group)
                  })}
                  {@render iconButton({
                    icon: 'uil--trash-alt',
                    title:
                      group.items.length === 0
                        ? m['admin.rooms_admin.delete_group']()
                        : m['admin.rooms_admin.delete_group_blocked'](),
                    tone: 'danger',
                    disabled: group.items.length > 0,
                    onclick: () => confirmDeleteGroup(group)
                  })}
                </div>
              </div>
            </header>

            <div
              class="min-h-12 p-2"
              use:dndzone={{
                items: group.items,
                flipDurationMs: 200,
                dropTargetStyle: {
                  outline: '2px dashed var(--color-action)',
                  'outline-offset': '-2px',
                  'border-radius': '0.5rem',
                  'background-color': 'color-mix(in srgb, var(--color-action) 5%, transparent)'
                },
                type: 'rooms'
              }}
              onconsider={(e) => handleGroupConsider(group.id, e)}
              onfinalize={(e) => handleGroupFinalize(group.id, e)}
            >
              {#each group.items as room (room.id)}
                <div
                  animate:flip={{ duration: 200 }}
                  class={[
                    'group flex cursor-grab items-center gap-3 rounded-lg py-2 pr-2 pl-3 hover:bg-surface',
                    room.kind === 'room' && room.room.archived && 'opacity-60'
                  ]}
                >
                  <div class="min-w-0 flex-1">
                    {#if room.kind === 'room'}
                      <div class="flex min-w-0 items-start gap-2">
                        <span class="mt-0.5 shrink-0 text-muted">#</span>
                        <div class="min-w-0 flex-1">
                          <div class="flex min-w-0 items-center gap-2">
                            <span class="min-w-0 truncate font-medium">{room.room.name}</span>
                            {#if room.room.isUniversal}
                              <Pill
                                tone="action"
                                title={m['admin.rooms_admin.universal_room']()}
                                class="inline-flex shrink-0 items-center gap-1 rounded-md px-1.5"
                              >
                                <span class="iconify text-xs uil--globe" aria-hidden="true"></span>
                                {m['admin.rooms_admin.universal']()}
                              </Pill>
                            {/if}
                            {#if room.room.archived}
                              <Pill tone="muted" class="shrink-0 rounded-md px-1.5"
                                >{m['admin.rooms_admin.archived']()}</Pill
                              >
                            {/if}
                          </div>
                          {#if room.room.description}
                            <p class="truncate text-sm text-muted">{room.room.description}</p>
                          {/if}
                        </div>
                      </div>
                    {:else}
                      <div class="flex min-w-0 items-baseline gap-1.5">
                        <span class="iconify text-muted uil--external-link-alt"></span>
                        <span class="truncate font-medium">{room.link.label}</span>
                      </div>
                      <p class="truncate text-sm text-muted">{room.link.url}</p>
                    {/if}
                  </div>
                  <div class="flex items-center gap-1.5">
                    {@render roomActions(room)}
                  </div>
                </div>
              {:else}
                <div class="px-3 py-4 text-center text-sm text-muted">
                  {m['admin.rooms_admin.drop_rooms']()}
                </div>
              {/each}
            </div>
          </section>
        {/each}
      </div>

      <div class="flex justify-center">
        <Button variant="secondary" onclick={openCreateGroup}>
          <span class="iconify uil--plus"></span>
          {m['admin.rooms_admin.new_group']()}
        </Button>
      </div>
    {/if}
  </div>
</div>

<Dialog
  bind:visible={createRoomDialogVisible}
  title={m['admin.rooms_admin.create_room']()}
  size="sm"
>
  {#if createRoomDialogVisible && createRoomGroupId}
    <CreateRoom groupId={createRoomGroupId} onroomcreated={handleRoomCreated} />
  {/if}
</Dialog>

<FormDialog
  bind:visible={editRoomDialogVisible}
  title={m['admin.rooms_admin.edit_room']()}
  size="sm"
  submitLabel={m['admin.permissions.save_changes']()}
  submitLoadingText={m['rbac.role_form.saving']()}
  loading={editRoomSaving}
  disabled={!editRoomName.trim() || !!editRoomNameError || !editRoomChanged}
  onsubmit={handleEditRoomSubmit}
  onclose={() => (editRoomDialogVisible = false)}
>
  <TextInput
    id="edit-room-name"
    label={m['rbac.role_form.name']()}
    bind:value={editRoomName}
    required
    disabled={editRoomSaving}
    error={editRoomNameError}
  />

  <TextArea
    id="edit-room-description"
    label={m['rbac.role_form.description']()}
    bind:value={editRoomDescription}
    rows={3}
    disabled={editRoomSaving}
    placeholder={m['admin.rooms_admin.room_description_placeholder']()}
  />

  <Checkbox
    id="edit-room-universal"
    bind:checked={editRoomUniversal}
    disabled={editRoomSaving}
    label={m['admin.rooms_admin.universal_room']()}
    description={UNIVERSAL_ROOM_HELP_TEXT}
  />
</FormDialog>

<FormDialog
  bind:visible={createGroupDialogVisible}
  title={m['admin.rooms_admin.create_group']()}
  size="sm"
  submitLabel={m['admin.rooms_admin.create_group']()}
  submitIcon="iconify uil--plus"
  disabled={!newGroupName.trim()}
  onsubmit={handleCreateGroupSubmit}
  onclose={() => (createGroupDialogVisible = false)}
>
  <TextInput
    id="new-group-name"
    label={m['admin.rooms_admin.group_name']()}
    bind:value={newGroupName}
    placeholder={m['admin.rooms_admin.group_name_placeholder']()}
  />
</FormDialog>

<FormDialog
  bind:visible={editGroupDialogVisible}
  title={m['admin.rooms_admin.rename_group']()}
  size="sm"
  submitLabel={m['rbac.role_form.save']()}
  disabled={!editGroupName.trim()}
  onsubmit={handleEditGroupSubmit}
  onclose={() => (editGroupDialogVisible = false)}
>
  <TextInput
    id="edit-group-name"
    label={m['admin.rooms_admin.group_name']()}
    bind:value={editGroupName}
  />
</FormDialog>

<FormDialog
  bind:visible={linkDialogVisible}
  title={editingLinkId ? m['admin.rooms_admin.edit_link']() : m['admin.rooms_admin.create_link']()}
  size="sm"
  submitLabel={editingLinkId ? m['rbac.role_form.save']() : m['admin.rooms_admin.create_link']()}
  submitIcon={editingLinkId ? undefined : 'iconify uil--plus'}
  disabled={!linkLabel.trim() || !linkUrl.trim()}
  onsubmit={handleLinkSubmit}
  onclose={() => (linkDialogVisible = false)}
>
  <TextInput
    id="sidebar-link-label"
    label={m['admin.rooms_admin.label']()}
    bind:value={linkLabel}
  />
  <TextInput
    id="sidebar-link-url"
    label={m['admin.rooms_admin.url']()}
    bind:value={linkUrl}
    placeholder={m['admin.rooms_admin.link_url_placeholder']()}
  />
</FormDialog>

{#if deleteGroupConfirmDialogVisible && deleteGroupConfirm}
  <ConfirmDialog
    title={m['admin.rooms_admin.delete_group']()}
    actionLabel={m['admin.rooms_admin.delete_group']()}
    actionIcon="iconify uil--trash-alt"
    onconfirm={deleteGroup}
    onclose={() => {
      deleteGroupConfirmDialogVisible = false;
      deleteGroupConfirm = null;
    }}
  >
    {m['admin.rooms_admin.delete_group_prompt']({ name: deleteGroupConfirm.name })}
  </ConfirmDialog>
{/if}

{#if deleteLinkConfirmDialogVisible && deleteLinkConfirm}
  <ConfirmDialog
    title={m['admin.rooms_admin.delete_link']()}
    actionLabel={m['admin.rooms_admin.delete_link']()}
    actionIcon="iconify uil--trash-alt"
    tone="danger"
    onconfirm={deleteLink}
    onclose={() => {
      deleteLinkConfirmDialogVisible = false;
      deleteLinkConfirm = null;
    }}
  >
    {m['admin.rooms_admin.delete_link_prompt']({ label: deleteLinkConfirm.label })}
  </ConfirmDialog>
{/if}

{#if archiveConfirmDialogVisible && archiveConfirmRoom}
  <ConfirmDialog
    title={m['admin.rooms_admin.archive_room']()}
    tone="warning"
    actionLabel={m['admin.rooms_admin.archive_room']()}
    actionIcon="iconify uil--archive"
    loading={!!layout.archivingRoomId}
    onconfirm={archiveRoom}
    onclose={cancelArchive}
  >
    {m['admin.rooms_admin.archive_room_prompt']({ room: archiveConfirmRoom.name })}
  </ConfirmDialog>
{/if}

{#if unarchiveConfirmDialogVisible && unarchiveConfirmRoom}
  <ConfirmDialog
    title={m['admin.rooms_admin.unarchive_room']()}
    tone="warning"
    actionLabel={m['admin.rooms_admin.unarchive_room']()}
    actionIcon="iconify uil--redo"
    loading={!!layout.archivingRoomId}
    onconfirm={unarchiveRoom}
    onclose={cancelUnarchive}
  >
    {m['admin.rooms_admin.unarchive_room_prompt']({ room: unarchiveConfirmRoom.name })}
  </ConfirmDialog>
{/if}
