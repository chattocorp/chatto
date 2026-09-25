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
- `canBanFromRoom` - Whether to show the room removal action
- `banningFromRoom` - Whether the room removal action is currently running
- `onBanFromRoom` - Callback when "Remove from room" is clicked
- `onOpenProfile` - Optional callback that opens the full room-sidebar profile
- `viewerSettings` - Optional viewer preferences for the user's local-time display
- `onClose` - Callback to close the popover/sheet
-->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { resolve } from '$app/paths';

  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import ParticipantAudioControls from '$lib/components/voice/ParticipantAudioControls.svelte';
  import type { ParticipantVolumeControl } from '$lib/state/server/callPreferences.svelte';
  import UserCustomStatusBadge from '$lib/components/UserCustomStatusBadge.svelte';
  import UserBio from '$lib/components/users/UserBio.svelte';
  import Interval from '$lib/lifecycle/Interval.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import ScrollFader from '$lib/ui/ScrollFader.svelte';
  import MenuItem from '$lib/ui/MenuItem.svelte';
  import MenuSection from '$lib/ui/MenuSection.svelte';
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
    audioSource = 'voiceVolume',
    onSendMessage,
    onBanFromRoom,
    onOpenProfile,
    onClose
  }: {
    user: {
      id: string;
      login: string;
      displayName: string;
      isBot?: boolean;
      deleted?: boolean;
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
    /** Defaults to microphone controls; screen-share cards select streamVolume. */
    audioSource?: ParticipantVolumeControl;
    onSendMessage?: () => void;
    onBanFromRoom?: () => void;
    onOpenProfile?: (userId: string) => void;
    onClose?: () => void;
  } = $props();

  const serverScope = useServerScope();
  const voiceCall = $derived(serverScope.store.voiceCall);
  // Only the active call's remote participants have listener-local volume controls.
  const audioParticipant = $derived(
    voiceCall?.connected
      ? voiceCall.participants?.find(
          (participant) => participant.identity === user.id && !participant.isLocal
        )
      : undefined
  );
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
  class={audioParticipant ? 'w-72' : 'w-64'}
  onclose={() => onClose?.()}
>
  <div class="flex items-center gap-3 menu-section p-3">
    <UserAvatar {user} size="md" />
    <div class="min-w-0 flex-1">
      <AccountName name={displayName} identity={user} class="font-semibold" />
      <div class="truncate text-xs text-muted">@{getLiveLogin(user.id, user.login)}</div>
      <UserCustomStatusBadge status={customStatus} showText class="mt-1 max-w-full" />
    </div>
  </div>

  {#if bio || localTime}
    <div class="space-y-1 menu-section px-3 py-2">
      {#if bio}
        <ScrollFader
          top
          bottom
          fill={false}
          fadeHeight="h-5"
          scrollClass="max-h-40 overscroll-contain"
          aria-label={m('settings.profile.bio.label')}
          data-testid="user-menu-bio-scroll"
        >
          <UserBio {bio} class="text-sm" />
        </ScrollFader>
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
    <MenuSection>
      {#if canSendMessage}
        <MenuItem icon="icon-[uil--comment-alt-message]" onclick={handleSendMessage}>
          {m('chat.user_menu.send_message')}
        </MenuItem>
      {/if}
      {#if onOpenProfile}
        <MenuItem icon="icon-[uil--user]" onclick={handleOpenProfile}>
          {m('chat.user_menu.view_profile')}
        </MenuItem>
      {/if}
      {#if adminUserHref}
        <MenuItem
          href={adminUserHref}
          icon="icon-[uil--servers]"
          onclick={() => onClose?.()}
          dataTestid="view-user-admin"
        >
          {m('chat.user_menu.view_in_admin')}
        </MenuItem>
      {/if}
      {#if canBanFromRoom}
        <MenuItem
          icon="icon-[uil--user-minus]"
          tone="danger"
          onclick={handleBanFromRoom}
          disabled={banningFromRoom}
        >
          {banningFromRoom ? m('admin.moderation.removing') : m('admin.moderation.remove_action')}
        </MenuItem>
      {/if}
    </MenuSection>
  {/if}

  {#if audioParticipant}
    <ParticipantAudioControls
      source={audioSource}
      settings={voiceCall.getParticipantAudio(audioParticipant.identity)}
      boostAvailable={voiceCall.audioBoostAvailable}
      onVolumeChange={(source, value) => voiceCall.setParticipantVolume(user.id, source, value)}
    />
  {/if}

  <MenuSection>
    <MenuItem
      icon="icon-[uil--copy]"
      onclick={() => void handleCopyUserId()}
      dataTestid="copy-user-id"
    >
      {m('chat.user_menu.copy_user_id')}
    </MenuItem>
  </MenuSection>
</ContextMenu>
