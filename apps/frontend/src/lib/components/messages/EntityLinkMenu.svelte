<!-- @component Entity actions in the target server's scope. Loaded only when a link menu opens. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { goto, pushState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { EntityLink } from '$lib/chatLinks';
  import type { RoomListItem } from '@chatto/client/server/rooms';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';
  import { RoomKind } from '@chatto/client/api/roomDirectory';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { serverRegistry } from '$lib/client';
  import { serverIdToSegment } from '$lib/navigation';
  import { userPath } from '$lib/navigation/conversationPath';
  import {
    archiveRoomFromMenu,
    joinRoomFromMenu,
    openLeaveRoomDialog,
    openRoomSettings
  } from '$lib/navigation/roomActions';
  import {
    markNavigationRoomAsRead,
    markNavigationServerAsRead
  } from '$lib/navigation/readActions';
  import { copyRoomLinkToClipboard, copyServerLinkToClipboard } from '$lib/copyChatLink';
  import { copyToClipboard } from '$lib/utils/copyToClipboard';
  import { toast } from '$lib/ui/toast';
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
    onclose,
    sheetPreview
  }: {
    target: EntityLink;
    user?: UserAvatarUserView;
    room?: RoomListItem;
    name: string;
    menu: ContextMenuTriggerDetails;
    onclose: () => void;
    sheetPreview?: Snippet<[() => void]>;
  } = $props();
  const scope = useServerScope();
  const { serverId, store } = scope;
  const segment = serverIdToSegment(serverId);
  const unread = serverUi(store).roomUnread;
  let dialog = $state<'archive' | 'invite' | null>(null);
  const server = $derived(serverRegistry.getServer(serverId));

  function leaveRoom() {
    if (!room) return;
    openLeaveRoomDialog(serverId, room);
    onclose();
  }

  async function joinRoom() {
    if (!room) return;
    if (await joinRoomFromMenu(scope, room)) onclose();
  }

  async function archiveRoom() {
    if (!room) return;
    if (await archiveRoomFromMenu(scope, room.id)) onclose();
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
    canSendMessage={store.permissions.canStartDMs}
    onSendMessage={() => {
      void goto(userPath(serverId, user.id));
      onclose();
    }}
    onClose={onclose}
    header={menu.presentation === 'sheet' && sheetPreview ? previewHeader : undefined}
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
    role={menu.presentation === 'sheet' ? 'dialog' : 'menu'}
    position={menu.position}
    presentation={menu.presentation}
    ariaLabel={room
      ? m('room_list.room_actions', { room: name })
      : m('room_list.server_actions', { server: name })}
    {onclose}
  >
    {#if menu.presentation === 'sheet'}{@render previewHeader()}{/if}
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
          openRoomSettings(serverId, room.id);
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
          icon="icon-[uil--link]"
          dataTestid="copy-server-link"
          onclick={() => {
            void copyServerLinkToClipboard(serverId);
            onclose();
          }}>{m('room_list.copy_server_link')}</MenuItem
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

{#snippet previewHeader()}
  {@render sheetPreview?.(onclose)}
{/snippet}
