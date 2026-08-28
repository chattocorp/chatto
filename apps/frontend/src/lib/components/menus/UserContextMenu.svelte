<!--
@component

Shows a user's profile card. On desktop, renders as a floating popover anchored to the trigger
element. On mobile (touch devices), renders as a bottom sheet. This dual behavior comes from
ContextMenu, which handles both modes automatically.

The optional profile callback is supplied only by room surfaces. Other uses
keep the compact menu without a navigation action.

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
- `onOpenProfile` - Optional callback that opens the full room-sidebar profile
- `viewerSettings` - Optional viewer preferences for the user's local-time display
- `onClose` - Callback to close the popover/sheet
-->
<script lang="ts">
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { resolve } from '$app/paths';

  import { RoomKind } from '$lib/api-client/roomDirectory';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserCustomStatusBadge from '$lib/components/UserCustomStatusBadge.svelte';
  import UserBio from '$lib/components/users/UserBio.svelte';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import {
    getLiveBio,
    getLiveCustomStatus,
    getLiveDisplayName,
    getLiveLogin,
    getLiveTimezone,
    type CustomUserStatus
  } from '$lib/state/userProfiles.svelte';
  import { m } from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';
  import {
    formatMessageTime,
    timeFormatSettingsFor,
    type ViewerTimeSettings
  } from '$lib/utils/formatTime';

  let {
    user,
    anchorRect,
    position,
    presentation = 'auto',
    canSendMessage = false,
    canBanFromRoom = false,
    banningFromRoom = false,
    viewerSettings,
    onSendMessage,
    onBanFromRoom,
    onOpenProfile,
    onClose
  }: {
    user: {
      id: string;
      login: string;
      displayName: string;
      avatarUrl?: string | null;
      bio?: string | null;
      timezone?: string | null;
      presenceStatus: PresenceStatus;
      customStatus?: CustomUserStatus | null;
    };
    anchorRect?: { top: number; bottom: number; left: number } | null;
    position?: { x: number; y: number };
    presentation?: 'auto' | 'floating' | 'sheet';
    canSendMessage?: boolean;
    canBanFromRoom?: boolean;
    banningFromRoom?: boolean;
    viewerSettings?: ViewerTimeSettings | null;
    onSendMessage?: () => void;
    onBanFromRoom?: () => void;
    onOpenProfile?: (userId: string) => void;
    onClose?: () => void;
  } = $props();

  const serverScope = useServerScope();
  const displayName = $derived(getLiveDisplayName(user.id, user.displayName || user.login));
  const customStatus = $derived(getLiveCustomStatus(user.id, user.customStatus));
  const bio = $derived(getLiveBio(user.id, user.bio ?? null));
  const timezone = $derived(getLiveTimezone(user.id, user.timezone ?? null));
  const viewerTimeSettings = $derived(timeFormatSettingsFor(viewerSettings));
  // Re-render the local-time line once a minute while the card is open.
  let now = $state(Date.now());
  const localTime = $derived.by(() => {
    if (!timezone) return null;
    try {
      return formatMessageTime(new Date(now), {
        ...viewerTimeSettings,
        effectiveTimezone: timezone
      });
    } catch {
      return null;
    }
  });
  const adminUserHref = $derived(
    serverScope.store.permissions.loaded && serverScope.store.permissions.canAdminViewUsers
      ? resolve('/chat/[serverId]/manage/server/members/[userId]', {
          serverId: serverIdToSegment(serverScope.serverId),
          userId: user.id
        })
      : null
  );
  const canOpenProfile = $derived.by(() => {
    if (!onOpenProfile) return false;
    if (serverScope.store.permissions.canStartDMs) return true;

    const currentUserId = serverScope.store.currentUser.user?.id;
    if (!currentUserId) return false;
    return [...serverScope.store.projection.rooms.values()].some((entry) => {
      const memberIds = entry.memberUserIds;
      const isSelfDM = memberIds.length === 1 && memberIds[0] === currentUserId && user.id === currentUserId;
      const isOneToOneDM =
        memberIds.length === 2 && memberIds.includes(currentUserId) && memberIds.includes(user.id);
      return entry.room?.room?.kind === RoomKind.DM && entry.room.viewerState?.isMember && (isSelfDM || isOneToOneDM);
    });
  });
  function handleSendMessage() {
    onSendMessage?.();
    onClose?.();
  }

  function handleBanFromRoom() {
    onBanFromRoom?.();
  }

  function handleOpenProfile() {
    onOpenProfile?.(user.id);
    onClose?.();
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

  {#if bio || localTime}
    <div class="space-y-1 menu-section px-3 py-2">
      {#if bio}
        <UserBio {bio} class="max-h-40 overflow-y-auto text-sm" />
      {/if}
      {#if timezone && localTime}
        <p class="flex items-center gap-1.5 text-sm text-muted">
          <span class="iconify icon-[uil--clock-three] shrink-0"></span>
          <span>{localTime}</span>
          <span class="truncate" dir="ltr">({timezone})</span>
        </p>
      {/if}
    </div>
    <Interval milliseconds={60_000} ontick={() => (now = Date.now())} />
  {/if}

  {#if canSendMessage || onOpenProfile || adminUserHref || canBanFromRoom}
    <div class="menu-section">
      <nav class="sidebar-nav">
        {#if canSendMessage}
          <button type="button" class="sidebar-item" onclick={handleSendMessage}>
            {m('chat.user_menu.send_message')}
          </button>
        {/if}
        {#if onOpenProfile}
          <button
            type="button"
            class="sidebar-item disabled:cursor-not-allowed disabled:opacity-50"
            onclick={handleOpenProfile}
            disabled={!canOpenProfile}
            title={canOpenProfile ? undefined : m('chat.user_menu.profile_requires_direct_message')}
          >
            {m('chat.user_menu.view_profile')}
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
