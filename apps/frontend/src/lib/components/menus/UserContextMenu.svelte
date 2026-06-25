<!--
@component

Shows a user's profile card. On desktop, renders as a floating popover anchored to the trigger
element. On mobile (touch devices), renders as a bottom sheet. This dual behavior comes from
ContextMenu, which handles both modes automatically.

**Props:**
- `user` - The user to display (must include id, login, displayName, presenceStatus)
- `anchorRect` - Bounding rect of the trigger element (used for desktop positioning)
- `canSendMessage` - Whether to show the "Send Message" button
- `onSendMessage` - Callback when "Send Message" is clicked
- `canBanFromRoom` - Whether to show the room-ban action
- `banningFromRoom` - Whether the room-ban action is currently running
- `onBanFromRoom` - Callback when "Ban from room" is clicked
- `onClose` - Callback to close the popover/sheet
-->
<script lang="ts">
  import type { PresenceStatus } from '$lib/gql/graphql';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UserCustomStatusBadge from '$lib/components/UserCustomStatusBadge.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import {
    getLiveCustomStatus,
    getLiveDisplayName,
    getLiveLogin,
    type CustomUserStatus
  } from '$lib/state/userProfiles.svelte';

  let {
    user,
    anchorRect,
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
    canSendMessage?: boolean;
    canBanFromRoom?: boolean;
    banningFromRoom?: boolean;
    onSendMessage?: () => void;
    onBanFromRoom?: () => void;
    onClose?: () => void;
  } = $props();

  const displayName = $derived(getLiveDisplayName(user.id, user.displayName || user.login));
  const customStatus = $derived(getLiveCustomStatus(user.id, user.customStatus));

  function handleSendMessage() {
    onSendMessage?.();
    onClose?.();
  }

  function handleBanFromRoom() {
    onBanFromRoom?.();
  }
</script>

<ContextMenu
  anchor={anchorRect}
  role="dialog"
  ariaLabel="User profile"
  class="w-64"
  onclose={() => onClose?.()}
>
  <div class="rounded-md bg-background">
    <div class="flex items-center gap-3 p-3">
      <UserAvatar {user} size="md" />
      <div class="min-w-0 flex-1">
        <div class="truncate font-semibold">{displayName}</div>
        <div class="truncate text-xs text-muted">@{getLiveLogin(user.id, user.login)}</div>
        <UserCustomStatusBadge status={customStatus} showText class="mt-1 max-w-full" />
      </div>
    </div>

    {#if canSendMessage || canBanFromRoom}
      <div class="border-t border-border p-1">
        {#if canSendMessage}
          <button type="button" class="sidebar-item" onclick={handleSendMessage}>
            Send Message
          </button>
        {/if}
        {#if canBanFromRoom}
          <button
            type="button"
            class="sidebar-item text-danger disabled:cursor-not-allowed disabled:opacity-50"
            onclick={handleBanFromRoom}
            disabled={banningFromRoom}
          >
            {banningFromRoom ? 'Banning...' : 'Ban from room'}
          </button>
        {/if}
      </div>
    {/if}
  </div>
</ContextMenu>
