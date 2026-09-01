<!--
@component

Renders the room list in the server sidebar. When a room layout is configured,
rooms are organized into collapsible sections. Otherwise, rooms display alphabetically.
-->
<script lang="ts">
  import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { goto, pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { m } from '$lib/i18n/messages';
  import {
    normalizeSidebarLinkURL,
    sidebarLinkAnchorAttributes,
    sidebarLinkTarget
  } from '$lib/navigation/sidebarLinkTarget';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import RoomGroupSection from '$lib/components/chat/RoomGroupSection.svelte';
  import CreateRoomGroupControl from '$lib/components/chat/CreateRoomGroupControl.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { buildDirectMessagePresentation, type UserAvatarUserView } from '$lib/render/users';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import NotificationBadge from '$lib/ui/NotificationBadge.svelte';
  import UnreadDot from '$lib/ui/UnreadDot.svelte';
  import { notificationTarget } from '$lib/state/server/notifications.svelte';
  import { prepareUiForNotificationTarget } from '$lib/notifications/notificationNavigationUi';
  import { getAppUiState, getRoomSidebarPresentation } from '$lib/state/appUi.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import {
    isNavigationVisibleRoom,
    type RoomsListItem,
    type RoomsListGroup,
    type RoomsListGroupItem
  } from '$lib/state/server/rooms.svelte';
  import type { CallRoomParticipant } from '$lib/state/server/activeCallRooms.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import MenuItem from '$lib/ui/MenuItem.svelte';
  import MenuSection from '$lib/ui/MenuSection.svelte';
  import NavigationContextMenu from '$lib/components/menus/NavigationContextMenu.svelte';
  import {
    contextMenuTrigger,
    type ContextMenuTriggerDetails
  } from '$lib/ui/contextMenuTrigger.svelte';
  import { markNavigationRoomAsRead } from '$lib/navigation/readActions';
  import { toast } from '$lib/ui/toast';
  import { createAdminRoomLayoutAPI } from '$lib/api-client/adminRoomLayout';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { fromAction, type Attachment } from 'svelte/attachments';
  import { SvelteMap } from 'svelte/reactivity';
  import {
    dragHandle,
    dragHandleZone,
    SHADOW_ITEM_MARKER_PROPERTY_NAME,
    SHADOW_PLACEHOLDER_ITEM_ID,
    type DndEvent
  } from 'svelte-dnd-action';
  import type { AdminRoomLayoutItemMutationInput } from '$lib/api-client/adminRoomLayout';

  let { canReorderGroups = false }: { canReorderGroups?: boolean } = $props();

  // RoomList reads server data from the route's server scope. Server-wide
  // room management is passed explicitly because it is shell permission state.
  // All store references go through `stores` ($derived), so when the URL
  // [serverId] param changes, every derived read in the template re-evaluates
  // against the new server's state automatically.

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const activeServer = $derived(serverRegistry.getServer(activeServerId));
  const activeServerBaseURL = $derived(activeServer?.url ?? null);
  const stores = $derived(serverScope.store);
  const notificationStore = $derived(stores.notifications);
  const activeCallRooms = $derived(stores.activeCallRooms);
  const appUi = getAppUiState();
  const roomLayoutAPI = serverScope.connection.getAPI(createAdminRoomLayoutAPI);
  const roomCommandAPI = serverScope.connection.getAPI(createRoomCommandAPI);
  const supportsRelativeSidebarMoves = $derived(
    stores.serverInfo.supportsFeature('relativeSidebarMoves')
  );

  const navigation = $derived(stores.navigation);
  const roomUnreadStore = $derived(stores.roomUnread);

  let activeRoomId = $derived(page.params.roomId);
  let roomContextMenu = $state<(ContextMenuTriggerDetails & { room: RoomsListItem }) | null>(null);
  let groupContextMenu = $state<(ContextMenuTriggerDetails & { group: RoomsListGroup }) | null>(
    null
  );
  let linkContextMenu = $state<
    (ContextMenuTriggerDetails & { group: RoomsListGroup; item: RoomsListGroupItem }) | null
  >(null);

  let createRoomDialogVisible = $state(false);
  let createRoomGroupId = $state<string | null>(null);
  let linkDialogVisible = $state(false);
  let linkGroupId = $state<string | null>(null);
  let editingLinkId = $state<string | null>(null);
  let linkLabel = $state('');
  let linkUrl = $state('');
  let deleteGroupDialogVisible = $state(false);
  let deleteGroupTarget = $state<RoomsListGroup | null>(null);
  let deleteLinkDialogVisible = $state(false);
  let deleteLinkTarget = $state<RoomsListGroupItem | null>(null);
  let archiveRoomDialogVisible = $state(false);
  let archiveRoomTarget = $state<RoomsListItem | null>(null);
  let optimisticGroupSections = $state<ManagedNavigationSection[] | null>(null);
  const optimisticGroupItems = new SvelteMap<string, RoomsListGroupItem[]>();
  let activeItemDragId = $state<string | null>(null);
  let itemFinalizeScheduled = false;
  const linkUrlIsValid = $derived(sidebarLinkTarget(linkUrl, activeServerBaseURL).valid);

  function roomMenuTrigger(room: RoomsListItem) {
    return contextMenuTrigger((details) => {
      roomContextMenu = { ...details, room };
    });
  }

  function groupMenuTrigger(group: RoomsListGroup) {
    if (!group.viewerCanManageGroup && !group.viewerCanCreateRoom) return undefined;
    return contextMenuTrigger((details) => {
      groupContextMenu = { ...details, group };
    });
  }

  const dndHandleAttachment = fromAction(dragHandle);

  const groupDragZoneAttachment = fromAction(dragHandleZone, () => ({
    items: renderManagedSections,
    flipDurationMs: 160,
    dropTargetStyle: {
      outline: '1px dashed var(--color-action)',
      'outline-offset': '-1px',
      'border-radius': '0.375rem'
    },
    type: 'sidebar-room-groups'
  }));

  const groupDragAttachment: Attachment<HTMLElement> = (node) => {
    const detachZone = groupDragZoneAttachment(node);
    const consider = (event: Event) => {
      if (event.target !== node) return;
      const detail = (event as CustomEvent<DndEvent<ManagedNavigationSection>>).detail;
      optimisticGroupSections = detail.items;
    };
    const finalize = (event: Event) => {
      if (event.target !== node) return;
      const detail = (event as CustomEvent<DndEvent<ManagedNavigationSection>>).detail;
      optimisticGroupSections = detail.items;
      const sections = detail.items.filter((section) => !isDndShadow(section));
      const movedSectionId = String(detail.info?.id ?? '');
      const moved = sections.find((section) => section.id === movedSectionId);
      if (!moved?.group) return;
      const index = sections.indexOf(moved);
      const beforeGroupId = sections[index + 1]?.group?.id;
      void persistGroupPlacement(moved.group.id, beforeGroupId);
    };
    node.addEventListener('consider', consider);
    node.addEventListener('finalize', finalize);
    return () => {
      node.removeEventListener('consider', consider);
      node.removeEventListener('finalize', finalize);
      detachZone?.();
    };
  };

  async function persistGroupPlacement(groupId: string, beforeGroupId?: string): Promise<void> {
    try {
      await roomLayoutAPI.moveRoomGroup({ groupId, beforeGroupId });
      optimisticGroupSections = null;
    } catch (error) {
      optimisticGroupSections = null;
      toast.error(
        m('admin.rooms_admin.reorder_groups_failed', {
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }

  function createItemDragAttachment(groupId: string): Attachment<HTMLDivElement> {
    const zoneAttachment = fromAction(dragHandleZone, () => ({
      items: managedSections.find((section) => section.group.id === groupId)?.items ?? [],
      flipDurationMs: 160,
      dropTargetStyle: {
        outline: '2px dashed var(--color-action)',
        'outline-offset': '-2px',
        'border-radius': '0.375rem'
      },
      type: 'sidebar-room-items'
    }));
    const attachment: Attachment<HTMLDivElement> = (node) => {
      const detachZone = zoneAttachment(node);
      const updateItems = (event: Event) => {
        if (event.target !== node) return;
        const detail = (event as CustomEvent<DndEvent<RoomsListGroupItem>>).detail;
        activeItemDragId ??= String(detail.info?.id ?? '');
        optimisticGroupItems.set(groupId, detail.items);
      };
      const finalize = (event: Event) => {
        if (event.target !== node) return;
        updateItems(event);
        if (itemFinalizeScheduled) return;
        itemFinalizeScheduled = true;
        queueMicrotask(() => {
          itemFinalizeScheduled = false;
          void persistItemPlacement();
        });
      };
      node.addEventListener('consider', updateItems);
      node.addEventListener('finalize', finalize);
      return () => {
        node.removeEventListener('consider', updateItems);
        node.removeEventListener('finalize', finalize);
        detachZone?.();
      };
    };
    return attachment;
  }

  function isDndShadow(item: { id: string }): boolean {
    const dndItem = item as typeof item & Record<string, unknown>;
    return (
      item.id === SHADOW_PLACEHOLDER_ITEM_ID || dndItem[SHADOW_ITEM_MARKER_PROPERTY_NAME] === true
    );
  }

  function itemMutationInput(item: RoomsListGroupItem): AdminRoomLayoutItemMutationInput {
    return item.type === 'room'
      ? { kind: 'room', id: item.roomId }
      : { kind: 'link', id: item.link.id };
  }

  async function persistItemPlacement(): Promise<void> {
    const draggedId = activeItemDragId;
    activeItemDragId = null;
    if (!draggedId) return;
    for (const [groupId, items] of optimisticGroupItems) {
      const index = items.findIndex((item) => item.id === draggedId);
      if (index < 0) continue;
      const item = items[index];
      if (!item) return;
      try {
        await roomLayoutAPI.moveSidebarItem({
          item: itemMutationInput(item),
          groupId,
          before: items[index + 1] ? itemMutationInput(items[index + 1]) : undefined
        });
        optimisticGroupItems.clear();
      } catch (error) {
        optimisticGroupItems.clear();
        toast.error(m('common.error.generic'));
        console.error('Failed to move sidebar item', error);
      }
      return;
    }
  }

  function linkMenuTrigger(group: RoomsListGroup, item: RoomsListGroupItem) {
    if (!group.viewerCanManageGroup || item.type !== 'link') return undefined;
    return contextMenuTrigger((details) => {
      linkContextMenu = { ...details, group, item };
    });
  }

  function handleConfigureGroup(group: RoomsListGroup): void {
    groupContextMenu = null;
    void goto(
      resolve('/chat/[serverId]/manage/room-groups/[groupId]', {
        serverId: serverSegment,
        groupId: group.id
      })
    );
  }

  function openCreateRoom(group: RoomsListGroup): void {
    groupContextMenu = null;
    createRoomGroupId = group.id;
    createRoomDialogVisible = true;
  }

  function handleRoomCreated(roomId: string): void {
    createRoomDialogVisible = false;
    createRoomGroupId = null;
    toast.success(m('admin.rooms_admin.room_created'));
    void goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  function openCreateLink(group: RoomsListGroup): void {
    groupContextMenu = null;
    editingLinkId = null;
    linkGroupId = group.id;
    linkLabel = '';
    linkUrl = '';
    linkDialogVisible = true;
  }

  function openEditLink(item: RoomsListGroupItem): void {
    if (item.type !== 'link') return;
    linkContextMenu = null;
    editingLinkId = item.link.id;
    linkGroupId = null;
    linkLabel = item.link.label;
    linkUrl = item.link.url;
    linkDialogVisible = true;
  }

  async function saveLink(event: Event): Promise<void> {
    event.preventDefault();
    const label = linkLabel.trim();
    const url = normalizeSidebarLinkURL(linkUrl);
    if (!label || !url) return;
    try {
      if (editingLinkId) {
        await roomLayoutAPI.updateSidebarLink({ linkId: editingLinkId, label, url });
        toast.success(m('admin.rooms_admin.link_updated'));
      } else if (linkGroupId) {
        await roomLayoutAPI.createSidebarLink({ groupId: linkGroupId, label, url });
        toast.success(m('admin.rooms_admin.link_created'));
      }
      linkDialogVisible = false;
    } catch (error) {
      toast.error(
        m('admin.rooms_admin.save_link_failed', {
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }

  function confirmDeleteGroup(group: RoomsListGroup): void {
    if (!groupIsEmpty(group)) return;
    groupContextMenu = null;
    deleteGroupTarget = group;
    deleteGroupDialogVisible = true;
  }

  function groupIsEmpty(group: RoomsListGroup): boolean {
    return group.roomIds.length === 0 && (group.items?.length ?? 0) === 0;
  }

  async function deleteGroup(): Promise<void> {
    if (!deleteGroupTarget) return;
    try {
      await roomLayoutAPI.deleteRoomGroup(deleteGroupTarget.id);
      toast.success(m('admin.rooms_admin.group_deleted'));
      deleteGroupDialogVisible = false;
      deleteGroupTarget = null;
    } catch (error) {
      toast.error(
        m('admin.rooms_admin.delete_group_failed', {
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }

  function confirmDeleteLink(item: RoomsListGroupItem): void {
    if (item.type !== 'link') return;
    linkContextMenu = null;
    deleteLinkTarget = item;
    deleteLinkDialogVisible = true;
  }

  async function deleteLink(): Promise<void> {
    if (!deleteLinkTarget || deleteLinkTarget.type !== 'link') return;
    try {
      await roomLayoutAPI.deleteSidebarLink(deleteLinkTarget.link.id);
      toast.success(m('admin.rooms_admin.link_deleted'));
      deleteLinkDialogVisible = false;
      deleteLinkTarget = null;
    } catch (error) {
      toast.error(
        m('admin.rooms_admin.delete_link_failed', {
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }

  function confirmArchiveRoom(room: RoomsListItem): void {
    roomContextMenu = null;
    archiveRoomTarget = room;
    archiveRoomDialogVisible = true;
  }

  async function archiveRoom(): Promise<void> {
    if (!archiveRoomTarget) return;
    try {
      await roomCommandAPI.archiveRoom(archiveRoomTarget.id);
      toast.success(m('admin.rooms_admin.room_archived'));
      archiveRoomDialogVisible = false;
      archiveRoomTarget = null;
    } catch (error) {
      toast.error(
        m('admin.rooms_admin.archive_room_failed', {
          error: error instanceof Error ? error.message : String(error)
        })
      );
    }
  }

  function handleMarkRoomRead(room: RoomsListItem): void {
    roomContextMenu = null;
    void markNavigationRoomAsRead(activeServerId, room.id);
  }

  async function handleCopyRoomId(roomId: string): Promise<void> {
    roomContextMenu = null;
    try {
      await navigator.clipboard.writeText(roomId);
      toast.success(m('common.copied_to_clipboard'));
    } catch {
      toast.error(m('common.error.generic'));
    }
  }

  function handleLeaveRoom(room: RoomsListItem): void {
    roomContextMenu = null;
    pushState('', {
      modal: {
        type: 'leaveRoom',
        serverId: activeServerId,
        roomId: room.id,
        roomName: room.name
      }
    });
  }

  function handleConfigureRoom(room: RoomsListItem): void {
    roomContextMenu = null;
    void goto(
      resolve('/chat/[serverId]/manage/rooms/[roomId]', {
        serverId: serverSegment,
        roomId: room.id
      })
    );
  }

  async function handleJoinRoom(room: RoomsListItem): Promise<void> {
    roomContextMenu = null;
    const result = await stores.roomDirectory.joinRoom(room.id);
    if (!serverScope.isCurrent()) return;
    if (result.ok) {
      toast.success(m('room.join.success', { room: room.name }));
      return;
    }

    toast.error(m('room.join.failed'));
    console.error('Error joining room:', result.error);
  }

  // --- Derived layout helpers ---

  // Channels and DMs are stored together. Operator-managed room groups only
  // contain channels, while every visible collection is normalized into the
  // same collapsible navigation-section presentation below.
  let channels = $derived(navigation.rooms.filter((r) => r.type === RoomKind.CHANNEL));
  let dmRooms = $derived(
    navigation.rooms.filter((r) => r.type === RoomKind.DM && isNavigationVisibleRoom(r))
  );

  let roomMap = $derived(new Map(navigation.rooms.map((r) => [r.id, r])));
  let channelMap = $derived(new Map(channels.map((r) => [r.id, r])));

  type NavigationSection = {
    id: string;
    label: string;
    items: RoomsListGroupItem[];
    persistKey: string;
    keepVisibleWhenCollapsed: typeof isGroupItemHighlighted;
    contextMenuTrigger?: ReturnType<typeof groupMenuTrigger>;
    itemsAttachment?: Attachment<HTMLDivElement>;
    group?: RoomsListGroup;
  };

  type ManagedNavigationSection = NavigationSection & { group: RoomsListGroup };

  function roomItems(rooms: RoomsListItem[]): RoomsListGroupItem[] {
    return rooms.map((room) => ({
      id: `room:${room.id}`,
      type: 'room',
      roomId: room.id
    }));
  }

  function getSetItems(set: RoomsListGroup): RoomsListGroupItem[] {
    const items =
      set.items ??
      set.roomIds.map((roomId) => ({
        id: `room:${roomId}`,
        type: 'room' as const,
        roomId
      }));
    return items.filter((item) => item.type === 'link' || channelMap.has(item.roomId));
  }

  // Keep actionable groups discoverable even when none of their rooms are
  // visible to the viewer.
  let visibleSets = $derived.by(() => {
    const sets = navigation.roomGroups;
    return sets.filter(
      (s) => s.viewerCanManageGroup || s.viewerCanCreateRoom || getSetItems(s).length > 0
    );
  });

  let itemDragAttachments = $derived(
    new Map(
      visibleSets
        .filter((group) => supportsRelativeSidebarMoves && group.viewerCanManageGroup)
        .map((group) => [group.id, createItemDragAttachment(group.id)] as const)
    )
  );

  // When no layout exists, display channels alphabetically
  let sortedRooms = $derived([...channels].sort((a, b) => a.name.localeCompare(b.name)));

  // DMs remain outside the operator-managed room-group domain. Treating every
  // visible collection as a navigation section gives configured groups, the
  // unconfigured fallback, and DMs identical disclosure and spacing behavior.
  let navigationSections = $derived.by((): NavigationSection[] => {
    const sections: NavigationSection[] = [];

    if (navigation.roomGroups.length > 0) {
      sections.push(
        ...visibleSets.map((group) => ({
          id: `group:${group.id}`,
          label: group.name,
          items: getSetItems(group),
          persistKey: serverStorageKey(activeServerId, `collapsible:set:${group.id}`),
          keepVisibleWhenCollapsed: isGroupItemHighlighted,
          contextMenuTrigger: groupMenuTrigger(group),
          itemsAttachment: itemDragAttachments.get(group.id),
          group
        }))
      );
    } else if (sortedRooms.length > 0) {
      sections.push({
        id: 'rooms',
        label: m('common.rooms'),
        items: roomItems(sortedRooms),
        persistKey: serverStorageKey(activeServerId, 'collapsible:rooms'),
        keepVisibleWhenCollapsed: isGroupItemHighlighted
      });
    }

    if (dmRooms.length > 0) {
      sections.push({
        id: 'direct-messages',
        label: m('room_list.direct_messages'),
        items: roomItems(dmRooms),
        persistKey: serverStorageKey(activeServerId, 'collapsible:dms'),
        keepVisibleWhenCollapsed: isGroupItemHighlighted
      });
    }

    return sections;
  });

  let groupByRoomId = $derived.by(() => {
    const groups = new SvelteMap<string, RoomsListGroup>();
    for (const group of navigation.roomGroups) {
      for (const roomId of group.roomIds) groups.set(roomId, group);
    }
    return groups;
  });

  let groupByItemId = $derived.by(() => {
    const groups = new SvelteMap<string, RoomsListGroup>();
    for (const group of navigation.roomGroups) {
      for (const item of getSetItems(group)) groups.set(item.id, group);
    }
    return groups;
  });

  let managedSections = $derived.by(() => {
    const sections = navigationSections.filter(
      (section): section is ManagedNavigationSection => !!section.group
    );
    return sections.map((section) => ({
      ...section,
      items: optimisticGroupItems.get(section.group.id) ?? section.items
    }));
  });

  let renderManagedSections = $derived(optimisticGroupSections ?? managedSections);

  let unmanagedSections = $derived(navigationSections.filter((section) => !section.group));

  // The viewer ID and DM members must come from the same server projection.
  // Reading the viewer ID from a global auth context here is unsafe — the
  // [serverId] layout intentionally renders children while the per-instance
  // CurrentUserState is still loading.
  function dmPresentation(room: RoomsListItem) {
    return buildDirectMessagePresentation(
      room.members,
      navigation.currentUserId,
      m('common.you'),
      getLiveDisplayName
    );
  }

  function callParticipantAvatarUser(participant: CallRoomParticipant): UserAvatarUserView {
    return {
      id: participant.userId,
      login: participant.login,
      displayName: participant.displayName,
      deleted: false,
      isBot: participant.isBot,
      avatarUrl: participant.avatarUrl,
      presenceStatus: PresenceStatus.OFFLINE
    };
  }

  // Keep active rooms and rooms needing attention visible when their group is collapsed.
  function isHighlighted(room: RoomsListItem): boolean {
    return (
      room.id === activeRoomId ||
      activeCallRooms.has(room.id) ||
      roomUnreadStore.roomIsUnread(room.id) ||
      room.viewerNotificationCount > 0
    );
  }

  function isGroupItemHighlighted(item: RoomsListGroupItem): boolean {
    if (item.type === 'link') return false;
    const room = roomMap.get(item.roomId);
    return room ? isHighlighted(room) : false;
  }

  function wasCallIconClick(event: MouseEvent): boolean {
    const target = event.target;
    return target instanceof Element && target.closest('[data-testid="room-call-icon"]') !== null;
  }

  async function openRoomCallPanel(roomId: string): Promise<void> {
    appUi.requestRoomSidebarPanel(activeServerId, roomId, 'call', getRoomSidebarPresentation());
    await goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  function handleRoomLinkClick(event: MouseEvent, room: RoomsListItem): void {
    if (room.viewerIsMember && activeCallRooms.has(room.id) && wasCallIconClick(event)) {
      event.preventDefault();
      void openRoomCallPanel(room.id);
    }
  }

  function handleRoomLinkKeydown(event: KeyboardEvent, room: RoomsListItem): void {
    if (event.target !== event.currentTarget) return;
    if (!room.viewerIsMember) return;
    if (!activeCallRooms.has(room.id)) return;
    if (event.key !== 'Enter' && event.key !== ' ') return;

    event.preventDefault();
    void openRoomCallPanel(room.id);
  }

  async function handleNotificationBadgeClick(event: MouseEvent, roomId: string, isDM: boolean) {
    event.preventDefault();
    event.stopPropagation();

    const lookup = await notificationStore.resolveRoomNotification(roomId, { isDM });
    if (!serverScope.isCurrent()) return;
    const notification = lookup.notification;

    if (!notification) {
      if (!lookup.ok || lookup.totalCount !== 0) {
        await goto(resolve('/chat/notifications'));
      }
      return;
    }

    const target = notificationTarget(notification);
    prepareUiForNotificationTarget(appUi, activeServerId, target);
    if (target.eventId && target.roomId) {
      stores.pendingHighlights.set(
        target.roomId,
        target.threadRootId,
        target.eventId,
        notification.id
      );
    }

    const path = notificationStore.getCleanPath(activeServerId, notification);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- getCleanPath returns a resolved app path.
    await goto(path);
  }
</script>

{#snippet activeCallIcon()}
  <span
    class="relative sidebar-icon text-action"
    aria-label={m('room_list.active_call')}
    data-testid="room-call-icon"
  >
    <span class="relative inline-flex">
      <span
        class="absolute inset-0 icon-[uil--phone] pane-header-icon-glyph animate-ping opacity-45"
        aria-hidden="true"
        data-testid="active-call-pulse-icon"
      ></span>
      <span class="relative icon-[uil--phone] pane-header-icon-glyph text-action" aria-hidden="true"
      ></span>
    </span>
  </span>
{/snippet}

{#snippet activeCallParticipants(roomId: string)}
  {@const participants = activeCallRooms.getParticipants(roomId)}
  {#if participants.length > 0}
    <div
      class="hidden shrink-0 items-center -space-x-1 @min-[220px]:flex"
      aria-label={m('room_list.call_participants', { count: participants.length })}
      data-testid="room-call-participants"
    >
      {#each participants.slice(0, 4) as participant, i (participant.userId)}
        <span
          class={[
            'inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full ring-1 ring-background',
            i === 2 ? 'hidden @min-[280px]:inline-flex' : '',
            i === 3 ? 'hidden @min-[340px]:inline-flex' : ''
          ]}
          data-testid="room-call-participant-avatar"
        >
          <UserAvatar user={callParticipantAvatarUser(participant)} size="xs" />
        </span>
      {/each}
      {#if participants.length > 4}
        <span
          class="hidden h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-surface-emphasized px-1 text-[10px] leading-none font-medium text-muted ring-1 ring-background @min-[380px]:inline-flex"
          data-testid="room-call-overflow"
        >
          +{participants.length - 4}
        </span>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet navigationRoomLink(room: RoomsListItem)}
  {@const isDM = room.type === RoomKind.DM}
  {@const isCurrentRoom = room.id === activeRoomId}
  {@const hasActiveCall = activeCallRooms.has(room.id)}
  {@const hasUnread = roomUnreadStore.roomIsUnread(room.id)}
  {@const isJoined = room.viewerIsMember}
  {@const showUnread = hasUnread && (isDM || isJoined)}
  {@const showActiveCall = hasActiveCall && (isDM || isJoined)}
  {@const presentation = isDM ? dmPresentation(room) : null}
  {@const owningGroup = groupByRoomId.get(room.id)}
  {@const showDragHandle = supportsRelativeSidebarMoves && owningGroup?.viewerCanManageGroup}
  <a
    href={resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: room.id })}
    class={[
      'group/room group/badges @container sidebar-item',
      showUnread && !isCurrentRoom ? 'sidebar-item-attention' : '',
      !isDM && !isJoined ? 'opacity-60 hover:opacity-85' : ''
    ]}
    aria-current={isCurrentRoom ? 'page' : undefined}
    onclick={(e) => handleRoomLinkClick(e, room)}
    onkeydown={(e) => handleRoomLinkKeydown(e, room)}
    {@attach roomMenuTrigger(room)}
  >
    {#if presentation}
      <div class="flex shrink-0 -space-x-1">
        {#each presentation.visibleParticipants.slice(0, 3) as participant (participant.id)}
          <UserAvatar user={participant} size="xs" />
        {/each}
      </div>
      <span class="flex-1 truncate">{presentation.label}</span>
    {:else}
      <span class="relative flex shrink-0">
        {#if isJoined}
          {#if room.isUniversal}
            <span
              class={[
                'iconify sidebar-icon icon-[uil--globe] transition-opacity',
                showUnread ? 'text-text-top' : 'text-muted',
                showDragHandle
                  ? 'group-focus-within/room:opacity-0 group-hover/room:opacity-0 [@media(hover:none)]:opacity-0'
                  : ''
              ]}
              role="img"
              aria-label={m('room.directory.universal')}
              title={m('room.directory.universal_title')}
            ></span>
          {:else}
            <span
              class={[
                'sidebar-icon transition-opacity',
                showUnread ? 'text-text-top' : 'text-muted',
                showDragHandle
                  ? 'group-focus-within/room:opacity-0 group-hover/room:opacity-0 [@media(hover:none)]:opacity-0'
                  : ''
              ]}>#</span
            >
          {/if}
        {:else if room.viewerCanJoinRoom}
          <span
            class={[
              'sidebar-icon text-muted transition-opacity',
              showDragHandle
                ? 'group-focus-within/room:opacity-0 group-hover/room:opacity-0 [@media(hover:none)]:opacity-0'
                : ''
            ]}>+</span
          >
        {:else}
          <span
            class={[
              'iconify sidebar-icon icon-[uil--lock] text-muted transition-opacity',
              showDragHandle
                ? 'group-focus-within/room:opacity-0 group-hover/room:opacity-0 [@media(hover:none)]:opacity-0'
                : ''
            ]}
          ></span>
        {/if}
        {#if showDragHandle}
          <button
            type="button"
            class="pointer-events-none absolute inset-0 mini-icon-action cursor-grab items-center justify-center opacity-0 transition-opacity group-focus-within/room:pointer-events-auto group-focus-within/room:opacity-100 group-hover/room:pointer-events-auto group-hover/room:opacity-100 active:cursor-grabbing [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100"
            aria-label={m('admin.rooms_admin.drag_room')}
            onclick={(event) => {
              event.preventDefault();
              event.stopPropagation();
            }}
            onpointerdown={(event) => event.stopPropagation()}
            data-sidebar-swipe-ignore
            data-testid="room-drag-handle"
            {@attach dndHandleAttachment}
          >
            <span class="iconify icon-[uil--draggabledots]" aria-hidden="true"></span>
          </button>
        {/if}
      </span>
      <span class="flex-1 truncate">{room.name}</span>
    {/if}
    <div class="relative ml-auto flex shrink-0 items-center">
      <div class="flex shrink-0 items-center gap-2">
        {#if showActiveCall}
          {@render activeCallParticipants(room.id)}
          {@render activeCallIcon()}
        {/if}

        {#if (isDM || isJoined) && room.viewerNotificationCount > 0}
          <button
            type="button"
            onclick={(e) => handleNotificationBadgeClick(e, room.id, isDM)}
            class="flex h-6 min-w-6 cursor-pointer items-center justify-center notification-dot"
            aria-label={isDM
              ? m('room_list.go_to_dm_notifications', { count: room.viewerNotificationCount })
              : m('room_list.go_to_notifications', { count: room.viewerNotificationCount })}
          >
            <NotificationBadge
              count={room.viewerNotificationCount}
              color={room.viewerImportantNotificationCount > 0 ? 'warning' : 'ambient'}
              testid={isDM ? 'dm-notification-badge' : 'room-notification-badge'}
            />
          </button>
          <span class="sr-only">
            {isDM
              ? m('room_list.new_direct_messages', { count: room.viewerNotificationCount })
              : m('room_list.notifications', { count: room.viewerNotificationCount })}
          </span>
        {:else if showUnread}
          <UnreadDot color="neutral" testid={isDM ? 'dm-unread-dot' : 'room-unread-dot'} />
          <span class="sr-only">{m('room_list.unread_messages')}</span>
        {/if}
      </div>
    </div>
  </a>
{/snippet}

{#snippet sidebarLink(item: RoomsListGroupItem)}
  {#if item.type === 'room'}
    {@const room = roomMap.get(item.roomId)}
    {#if room}
      {@render navigationRoomLink(room)}
    {/if}
  {:else}
    {@const target = sidebarLinkTarget(item.link.url, activeServerBaseURL)}
    {@const owningGroup = groupByItemId.get(item.id)}
    {@const showDragHandle = supportsRelativeSidebarMoves && owningGroup?.viewerCanManageGroup}
    <a
      {...sidebarLinkAnchorAttributes(target)}
      aria-disabled={!target.valid}
      class={[
        'group/link sidebar-item w-full text-left',
        !target.valid && 'cursor-not-allowed opacity-60'
      ]}
      {@attach owningGroup ? linkMenuTrigger(owningGroup, item) : undefined}
      onclick={(event) => {
        if (!target.valid) event.preventDefault();
      }}
    >
      <span class="relative flex shrink-0" data-testid="sidebar-link-leading-icon">
        <span
          class={[
            'iconify sidebar-icon icon-[uil--external-link-alt] text-muted transition-opacity',
            showDragHandle
              ? 'group-focus-within/link:opacity-0 group-hover/link:opacity-0 [@media(hover:none)]:opacity-0'
              : ''
          ]}
        ></span>
        {#if showDragHandle}
          <button
            type="button"
            class="pointer-events-none absolute inset-0 mini-icon-action cursor-grab items-center justify-center opacity-0 transition-opacity group-focus-within/link:pointer-events-auto group-focus-within/link:opacity-100 group-hover/link:pointer-events-auto group-hover/link:opacity-100 active:cursor-grabbing [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100"
            aria-label={m('admin.rooms_admin.drag_link')}
            onclick={(event) => {
              event.preventDefault();
              event.stopPropagation();
            }}
            onpointerdown={(event) => event.stopPropagation()}
            data-sidebar-swipe-ignore
            data-testid="sidebar-link-drag-handle"
            {@attach dndHandleAttachment}
          >
            <span class="iconify icon-[uil--draggabledots]" aria-hidden="true"></span>
          </button>
        {/if}
      </span>
      <span class="flex-1 truncate">{item.link.label}</span>
    </a>
  {/if}
{/snippet}

{#snippet groupLeadingOverlay()}
  <button
    type="button"
    class="pointer-events-none absolute inset-0 mini-icon-action cursor-grab items-center justify-center opacity-0 transition-opacity group-focus-within/section-header:pointer-events-auto group-focus-within/section-header:opacity-100 group-hover/section-header:pointer-events-auto group-hover/section-header:opacity-100 active:cursor-grabbing [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100"
    aria-label={m('admin.rooms_admin.drag_group')}
    onclick={(event) => event.stopPropagation()}
    onpointerdown={(event) => event.stopPropagation()}
    data-sidebar-swipe-ignore
    data-room-group-drag-handle
    data-testid="room-group-drag-handle"
    {@attach dndHandleAttachment}
  >
    <span class="iconify icon-[uil--draggabledots]" aria-hidden="true"></span>
  </button>
{/snippet}

{#snippet groupHeaderActions(group: RoomsListGroup)}
  {#if group.viewerCanCreateRoom}
    <button
      type="button"
      class="pointer-events-none mini-icon-action h-6 w-6 items-center justify-center opacity-0 transition-opacity group-focus-within/section-header:pointer-events-auto group-focus-within/section-header:opacity-100 group-hover/section-header:pointer-events-auto group-hover/section-header:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100"
      aria-label={m('admin.rooms_admin.new_room')}
      onclick={(event) => {
        event.stopPropagation();
        openCreateRoom(group);
      }}
      data-testid="create-room-button"
    >
      <span class="iconify icon-[uil--plus]" aria-hidden="true"></span>
    </button>
  {/if}
{/snippet}

{#if channels.length === 0 && dmRooms.length === 0 && visibleSets.length === 0 && !navigation.isInitialLoading}
  <EmptyState icon="icon-[uil--comments]" title={m('room_list.empty_title')}>
    {m('room_list.empty_prefix')}
    <a href={resolve('/chat/[serverId]/overview', { serverId: serverSegment })} class="link"
      >{m('room_list.empty_overview')}</a
    >
    {m('room_list.empty_suffix')}
  </EmptyState>
{:else}
  <nav class="room-list md:w-full">
    <div
      data-testid={supportsRelativeSidebarMoves && canReorderGroups
        ? 'room-groups-dropzone'
        : undefined}
      {@attach supportsRelativeSidebarMoves && canReorderGroups ? groupDragAttachment : undefined}
    >
      {#each renderManagedSections as section, i (section.id)}
        {#snippet headerActions()}
          {#if !isDndShadow(section)}
            {@render groupHeaderActions(section.group)}
          {/if}
        {/snippet}
        {#snippet leadingOverlay()}
          {#if !isDndShadow(section) && supportsRelativeSidebarMoves && canReorderGroups}
            {@render groupLeadingOverlay()}
          {/if}
        {/snippet}
        <RoomGroupSection
          label={section.label}
          items={section.items}
          item={sidebarLink}
          persistKey={section.persistKey}
          keepVisibleWhenCollapsed={section.keepVisibleWhenCollapsed}
          contextMenuTrigger={section.contextMenuTrigger}
          itemsAttachment={isDndShadow(section) ? undefined : section.itemsAttachment}
          containItemDrag
          isDndShadow={isDndShadow(section)}
          {headerActions}
          {leadingOverlay}
          separated={i > 0}
        />
      {/each}
    </div>
    {#if canReorderGroups}
      <CreateRoomGroupControl />
    {/if}
    {#each unmanagedSections as section, i (section.id)}
      <RoomGroupSection
        label={section.label}
        items={section.items}
        item={sidebarLink}
        persistKey={section.persistKey}
        keepVisibleWhenCollapsed={section.keepVisibleWhenCollapsed}
        separated={renderManagedSections.length > 0 || i > 0}
      />
    {/each}
  </nav>
{/if}

{#if groupContextMenu}
  {@const contextGroup = groupContextMenu.group}
  <ContextMenu
    position={groupContextMenu.position}
    presentation={groupContextMenu.presentation}
    ariaLabel={m('room_list.group_settings', { group: contextGroup.name })}
    onclose={() => (groupContextMenu = null)}
  >
    {#if contextGroup.viewerCanCreateRoom || contextGroup.viewerCanManageGroup}
      <MenuSection>
        {#if contextGroup.viewerCanCreateRoom}
          <MenuItem icon="icon-[uil--plus]" onclick={() => openCreateRoom(contextGroup)}>
            {m('admin.rooms_admin.new_room')}
          </MenuItem>
        {/if}
        {#if contextGroup.viewerCanManageGroup}
          <MenuItem
            icon="icon-[uil--external-link-alt]"
            onclick={() => openCreateLink(contextGroup)}
          >
            {m('admin.rooms_admin.new_link')}
          </MenuItem>
        {/if}
      </MenuSection>
    {/if}
    {#if contextGroup.viewerCanManageGroup}
      <MenuSection>
        <MenuItem icon="icon-[uil--setting]" onclick={() => handleConfigureGroup(contextGroup)}>
          {m('settings.nav.title')}
        </MenuItem>
      </MenuSection>
      <MenuSection>
        <MenuItem
          icon="icon-[uil--trash-alt]"
          tone="danger"
          disabled={!groupIsEmpty(contextGroup)}
          onclick={() => confirmDeleteGroup(contextGroup)}
        >
          {m('admin.rooms_admin.delete_group')}
        </MenuItem>
      </MenuSection>
    {/if}
  </ContextMenu>
{/if}

{#if roomContextMenu}
  {@const contextRoom = roomContextMenu.room}
  <ContextMenu
    position={roomContextMenu.position}
    presentation={roomContextMenu.presentation}
    ariaLabel={m('room_list.room_actions', { room: contextRoom.name })}
    onclose={() => (roomContextMenu = null)}
  >
    <NavigationContextMenu
      kind="room"
      isRoomMember={contextRoom.viewerIsMember}
      canJoin={contextRoom.viewerCanJoinRoom}
      canMarkRead={roomUnreadStore.roomIsUnread(contextRoom.id) ||
        contextRoom.viewerNotificationCount > 0}
      canConfigure={contextRoom.viewerCanManageRoom && contextRoom.type !== RoomKind.DM}
      canLeave={!contextRoom.isUniversal && contextRoom.type !== RoomKind.DM}
      onJoin={() => void handleJoinRoom(contextRoom)}
      onMarkRead={() => handleMarkRoomRead(contextRoom)}
      onConfigure={() => handleConfigureRoom(contextRoom)}
      onLeave={() => handleLeaveRoom(contextRoom)}
    />
    {#if contextRoom.viewerCanManageRoom && contextRoom.type !== RoomKind.DM}
      <MenuSection>
        <MenuItem icon="icon-[uil--archive]" onclick={() => confirmArchiveRoom(contextRoom)}>
          {m('admin.rooms_admin.archive_room')}
        </MenuItem>
      </MenuSection>
    {/if}
    <MenuSection>
      <MenuItem
        icon="icon-[uil--copy]"
        onclick={() => void handleCopyRoomId(contextRoom.id)}
        dataTestid="copy-room-id"
      >
        {m('room_list.copy_room_id')}
      </MenuItem>
    </MenuSection>
  </ContextMenu>
{/if}

{#if linkContextMenu && linkContextMenu.item.type === 'link'}
  {@const contextLink = linkContextMenu.item}
  <ContextMenu
    position={linkContextMenu.position}
    presentation={linkContextMenu.presentation}
    ariaLabel={m('admin.rooms_admin.edit_link')}
    onclose={() => (linkContextMenu = null)}
  >
    <MenuSection>
      <MenuItem icon="icon-[uil--pen]" onclick={() => openEditLink(contextLink)}>
        {m('admin.rooms_admin.edit_link')}
      </MenuItem>
    </MenuSection>
    <MenuSection>
      <MenuItem
        icon="icon-[uil--trash-alt]"
        tone="danger"
        onclick={() => confirmDeleteLink(contextLink)}
      >
        {m('admin.rooms_admin.delete_link')}
      </MenuItem>
    </MenuSection>
  </ContextMenu>
{/if}

{#if createRoomDialogVisible && createRoomGroupId}
  {#await import('$lib/CreateRoom.svelte') then CreateRoomModule}
    <CreateRoomModule.default
      bind:visible={createRoomDialogVisible}
      groupId={createRoomGroupId}
      onclose={() => (createRoomGroupId = null)}
      onroomcreated={handleRoomCreated}
    />
  {/await}
{/if}

{#if linkDialogVisible}
  {#await Promise.all([
    import('$lib/ui').then(({ FormDialog }) => ({ default: FormDialog })),
    import('$lib/ui/form').then(({ TextInput }) => ({ default: TextInput }))
  ]) then [FormDialogModule, TextInputModule]}
    <FormDialogModule.default
      bind:visible={linkDialogVisible}
      title={editingLinkId ? m('admin.rooms_admin.edit_link') : m('admin.rooms_admin.create_link')}
      size="sm"
      submitLabel={editingLinkId ? m('rbac.role_form.save') : m('admin.rooms_admin.create_link')}
      submitIcon={editingLinkId ? undefined : 'iconify icon-[uil--plus]'}
      disabled={!linkLabel.trim() || !linkUrlIsValid}
      onsubmit={saveLink}
      onclose={() => (linkDialogVisible = false)}
    >
      <TextInputModule.default
        id="sidebar-link-label"
        label={m('admin.rooms_admin.label')}
        bind:value={linkLabel}
      />
      <TextInputModule.default
        id="sidebar-link-url"
        label={m('admin.rooms_admin.url')}
        bind:value={linkUrl}
        placeholder={m('admin.rooms_admin.link_url_placeholder')}
      />
    </FormDialogModule.default>
  {/await}
{/if}

{#if deleteGroupDialogVisible && deleteGroupTarget}
  {#await import('$lib/ui').then(({ ConfirmDialog }) => ({ default: ConfirmDialog })) then ConfirmDialogModule}
    <ConfirmDialogModule.default
      title={m('admin.rooms_admin.delete_group')}
      actionLabel={m('admin.rooms_admin.delete_group')}
      actionIcon="iconify icon-[uil--trash-alt]"
      tone="danger"
      onconfirm={deleteGroup}
      onclose={() => {
        deleteGroupDialogVisible = false;
        deleteGroupTarget = null;
      }}
    >
      {m('admin.rooms_admin.delete_group_prompt', { name: deleteGroupTarget.name })}
    </ConfirmDialogModule.default>
  {/await}
{/if}

{#if deleteLinkDialogVisible && deleteLinkTarget?.type === 'link'}
  {#await import('$lib/ui').then(({ ConfirmDialog }) => ({ default: ConfirmDialog })) then ConfirmDialogModule}
    <ConfirmDialogModule.default
      title={m('admin.rooms_admin.delete_link')}
      actionLabel={m('admin.rooms_admin.delete_link')}
      actionIcon="iconify icon-[uil--trash-alt]"
      tone="danger"
      onconfirm={deleteLink}
      onclose={() => {
        deleteLinkDialogVisible = false;
        deleteLinkTarget = null;
      }}
    >
      {m('admin.rooms_admin.delete_link_prompt', { label: deleteLinkTarget.link.label })}
    </ConfirmDialogModule.default>
  {/await}
{/if}

{#if archiveRoomDialogVisible && archiveRoomTarget}
  {#await import('$lib/ui').then(({ ConfirmDialog }) => ({ default: ConfirmDialog })) then ConfirmDialogModule}
    <ConfirmDialogModule.default
      title={m('admin.rooms_admin.archive_room')}
      actionLabel={m('admin.rooms_admin.archive_room')}
      actionIcon="iconify icon-[uil--archive]"
      tone="warning"
      onconfirm={archiveRoom}
      onclose={() => {
        archiveRoomDialogVisible = false;
        archiveRoomTarget = null;
      }}
    >
      {m('admin.rooms_admin.archive_room_prompt', { room: archiveRoomTarget.name })}
    </ConfirmDialogModule.default>
  {/await}
{/if}
