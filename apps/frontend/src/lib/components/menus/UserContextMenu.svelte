<!--
@component

Shows a user's profile card. On desktop, renders as a floating popover anchored to the trigger
element. On mobile (touch devices), renders as a bottom sheet. This dual behavior comes from
ContextMenu, which handles both modes automatically.

When the current viewer can open Server Admin user pages, the menu links to the selected user's
page on the active server.

**Props:**
- `user` - The user to display (must include id, login, displayName, presenceStatus)
- `anchorRect` - Bounding rect of the trigger element (used for desktop positioning)
- `position` - Viewport point used by right-click and long-press triggers
- `presentation` - Optional floating/sheet presentation selected by the trigger
- `canSendMessage` - Whether to show the "Send Message" button
- `onSendMessage` - Callback when "Send Message" is clicked
- `canBanFromRoom` - Whether to show the room-ban action
- `banningFromRoom` - Whether the room-ban action is currently running
- `onBanFromRoom` - Callback when "Ban from room" is clicked
- `onClose` - Callback to close the popover/sheet
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserCustomStatusBadge from '$lib/components/UserCustomStatusBadge.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import {
    getLiveCustomStatus,
    getLiveDisplayName,
    getLiveLogin,
    type CustomUserStatus
  } from '$lib/state/userProfiles.svelte';
  import { m } from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';

  let {
    user,
    anchorRect,
    position,
    presentation = 'auto',
    canSendMessage = false,
    canBanFromRoom = false,
    banningFromRoom = false,
    onSendMessage,
    onBanFromRoom,
    onClose
  }: {
    user: {
      id: string;
      login: string;
      displayName: string;
      avatarUrl?: string | null;
      presenceStatus: PresenceStatus;
      customStatus?: CustomUserStatus | null;
    };
    anchorRect?: { top: number; bottom: number; left: number } | null;
    position?: { x: number; y: number };
    presentation?: 'auto' | 'floating' | 'sheet';
    canSendMessage?: boolean;
    canBanFromRoom?: boolean;
    banningFromRoom?: boolean;
    onSendMessage?: () => void;
    onBanFromRoom?: () => void;
    onClose?: () => void;
  } = $props();

  const serverScope = useServerScope();
  const displayName = $derived(getLiveDisplayName(user.id, user.displayName || user.login));
  const customStatus = $derived(getLiveCustomStatus(user.id, user.customStatus));
  const adminUserHref = $derived(
    serverScope.store.permissions.loaded && serverScope.store.permissions.canAdminViewUsers
      ? resolve('/chat/[serverId]/manage/server/members/[userId]', {
          serverId: serverIdToSegment(serverScope.serverId),
          userId: user.id
        })
      : null
  );

  function handleSendMessage() {
    onSendMessage?.();
    onClose?.();
  }

  function handleBanFromRoom() {
    onBanFromRoom?.();
  }

  async function handleCopyUserId(): Promise<void> {
    try {
      const write = navigator.clipboard.writeText(user.id);
      onClose?.();
      await write;
      toast.success(m('common.copied_to_clipboard'));
    } catch {
      toast.error(m('common.error.generic'));
    }
  }
</script>

<ContextMenu
  {position}
  anchor={anchorRect}
  {presentation}
  role="dialog"
  ariaLabel={m('chat.user_menu.profile')}
  class="w-64"
  onclose={() => onClose?.()}
>
  <div class="flex items-center gap-3 menu-section p-3">
    <UserAvatar {user} size="md" />
    <div class="min-w-0 flex-1">
      <div class="truncate font-semibold">{displayName}</div>
      <div class="truncate text-xs text-muted">@{getLiveLogin(user.id, user.login)}</div>
      <UserCustomStatusBadge status={customStatus} showText class="mt-1 max-w-full" />
    </div>
  </div>

  {#if canSendMessage || adminUserHref || canBanFromRoom}
    <div class="menu-section">
      <nav class="sidebar-nav">
        {#if canSendMessage}
          <button type="button" class="sidebar-item" onclick={handleSendMessage}>
            {m('chat.user_menu.send_message')}
          </button>
        {/if}
        {#if adminUserHref}
          <a
            class="sidebar-item"
            href={adminUserHref}
            onclick={() => onClose?.()}
            data-testid="view-user-admin"
          >
            {m('chat.user_menu.view_in_admin')}
          </a>
        {/if}
        {#if canBanFromRoom}
          <button
            type="button"
            class="sidebar-item text-danger disabled:cursor-not-allowed disabled:opacity-50"
            onclick={handleBanFromRoom}
            disabled={banningFromRoom}
          >
            {banningFromRoom ? m('admin.moderation.banning') : m('admin.moderation.ban_action')}
          </button>
        {/if}
      </nav>
    </div>
  {/if}

  <div class="menu-section">
    <nav class="sidebar-nav">
      <button
        type="button"
        class="sidebar-item"
        onclick={() => void handleCopyUserId()}
        data-testid="copy-user-id"
      >
        {m('chat.user_menu.copy_user_id')}
      </button>
    </nav>
  </div>
</ContextMenu>
