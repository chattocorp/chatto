<!-- @component Entity actions in the target server's scope. Loaded only when a link menu opens. -->
<script lang="ts">
  import { goto, pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { EntityLink } from '$lib/chatLinks';
  import type { RoomListItem } from '@chatto/client/server/rooms';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import { createRoomCommandAPI } from '@chatto/client/api/rooms';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { serverRegistry } from '$lib/client';
  import { serverIdToSegment } from '$lib/navigation';
  import { userPath, conversationPath } from '$lib/navigation/conversationPath';
  import {
    markNavigationRoomAsRead,
    markNavigationServerAsRead
  } from '$lib/navigation/readActions';
  import { copyRoomLinkToClipboard } from '$lib/copyChatLink';
  import { copyToClipboard } from '$lib/utils/copyToClipboard';
  import { toast } from '$lib/ui/toast';
  import { toastError } from '$lib/utils/errorMessage';
  import { m } from '$lib/i18n/messages';
  import {
    ContextMenu,
    MenuItem,
    MenuSection,
    ConfirmDialog,
    type ContextMenuTriggerDetails
  } from '$lib/ui';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import RoomContextMenu from '$lib/components/menus/RoomContextMenu.svelte';
  import InviteRoomMemberDialog from '$lib/components/chat/InviteRoomMemberDialog.svelte';

  let {
    target,
    user,
    room,
    name,
    menu,
    onclose
  }: {
    target: EntityLink;
    user?: UserAvatarUserView;
    room?: RoomListItem;
    name: string;
    menu: ContextMenuTriggerDetails;
    onclose: () => void;
  } = $props();
  const scope = useServerScope();
  const { serverId, store, connection } = scope;
  const segment = serverIdToSegment(serverId);
  const unread = serverUi(store).roomUnread;
  let dialog = $state<'archive' | 'invite' | null>(null);
  const server = $derived(serverRegistry.getServer(serverId));

  function leaveRoom() {
    if (!room) return;
    pushState('', { modal: { type: 'leaveRoom', serverId, roomId: room.id, roomName: room.name } });
    onclose();
  }

  async function joinRoom() {
    if (!room) return;
    if (room.viewerInvitation) {
      void goto(conversationPath(serverId, room.id));
      onclose();
      return;
    }
    const result = await serverUi(store).roomDirectory.joinRoom(room.id);
    if (!scope.isCurrent()) return;
    if (result.ok) {
      toast.success(m('room.join.success', { room: room.name }));
      onclose();
    } else toast.error(m('room.join.failed'));
  }

  async function archiveRoom() {
    if (!room) return;
    try {
      await connection.getAPI(createRoomCommandAPI).archiveRoom(room.id);
      if (!scope.isCurrent()) return;
      toast.success(m('room.management.room_archived'));
      onclose();
    } catch (error) {
      toastError(error);
    }
  }

  async function markRead() {
    const ok = room
      ? await markNavigationRoomAsRead(serverId, room.id)
      : await markNavigationServerAsRead(serverId);
    if (!scope.isCurrent()) return;
    if (!ok) toast.error(m('common.error.generic'));
    onclose();
  }
</script>

{#if user}
  <UserContextMenu
    {user}
    position={menu.position}
    presentation={menu.presentation}
    canSendMessage={true}
    onSendMessage={() => {
      void goto(userPath(serverId, user.id));
      onclose();
    }}
    onClose={onclose}
  />
{:else if dialog === 'archive' && room}
  <ConfirmDialog
    title={m('room.management.archive_room')}
    actionLabel={m('room.management.archive_room')}
    actionIcon="iconify icon-[uil--archive]"
    tone="warning"
    onconfirm={archiveRoom}
    {onclose}
  >
    {m('room.management.archive_room_prompt', { room: room.name })}
  </ConfirmDialog>
{:else if dialog === 'invite' && room}
  <InviteRoomMemberDialog
    room={{
      ...room,
      viewerNotificationCount: store.notifications.roomUnreadCounts[room.id] ?? 0,
      viewerImportantNotificationCount: store.notifications.roomImportantUnreadCounts[room.id] ?? 0
    }}
    {onclose}
  />
{:else}
  <ContextMenu
    position={menu.position}
    presentation={menu.presentation}
    ariaLabel={room
      ? m('room_list.room_actions', { room: name })
      : m('room_list.server_actions', { server: name })}
    {onclose}
  >
    {#if room}
      <RoomContextMenu
        isRoomMember={room.viewerIsMember === true}
        canJoin={room.viewerCanJoinRoom || !!room.viewerInvitation}
        joinLabel={room.viewerInvitation ? m('room.invitation.invited') : m('room.join.action')}
        canInvite={room.viewerCanInvite}
        onInvite={() => (dialog = 'invite')}
        canMarkRead={unread.roomIsUnread(room.id) ||
          (store.notifications.roomUnreadCounts[room.id] ?? 0) > 0}
        canConfigure={room.viewerCanManageRoom && room.type !== RoomKind.DM}
        canLeave={!room.isUniversal && room.type !== RoomKind.DM}
        onJoin={() => void joinRoom()}
        onMarkRead={() => void markRead()}
        onConfigure={() => {
          void goto(
            resolve('/chat/[serverId]/manage/rooms/[roomId]', {
              serverId: segment,
              roomId: room.id
            })
          );
          onclose();
        }}
        onLeave={leaveRoom}
        onCopyLink={() => {
          void copyRoomLinkToClipboard(serverId, room.id);
          onclose();
        }}
        onCopyId={() => {
          void copyToClipboard(room.id);
          onclose();
        }}
        onArchive={() => (dialog = 'archive')}
      />
    {:else if target.kind === 'server' && server}
      <MenuSection>
        <MenuItem
          icon="icon-[uil--info-circle]"
          href={resolve('/chat/[serverId]/info', { serverId: segment })}
          onclick={onclose}>{m('chat.server_info.title')}</MenuItem
        >
        <MenuItem
          icon="icon-[uil--copy]"
          onclick={() => {
            void copyToClipboard(new URL(server.url).host);
            onclose();
          }}>{m('room_list.copy_server_hostname')}</MenuItem
        >
        {#if store.isAuthenticated}
          <MenuItem
            icon="icon-[uil--check-circle]"
            disabled={!unread.hasAnyUnread && store.notifications.unreadNotificationCount === 0}
            onclick={() => void markRead()}>{m('room_list.mark_as_read')}</MenuItem
          >
        {/if}
      </MenuSection>
      {#if !serverRegistry.isOriginServer(serverId)}
        <MenuSection>
          <MenuItem
            icon="icon-[uil--minus-circle]"
            tone="danger"
            onclick={() => {
              pushState('', { modal: { type: 'removeServer', serverId, spaceName: name } });
              onclose();
            }}>{m('room_list.remove_server')}</MenuItem
          >
        </MenuSection>
      {/if}
    {/if}
  </ContextMenu>
{/if}
