<!--
@component

Shows a user's profile card. On desktop, renders as a floating popover anchored to the trigger
element. On mobile (touch devices), renders as a bottom sheet. This dual behavior comes from
ContextMenu, which handles both modes automatically.

The optional profile callback is supplied only by room surfaces. Other uses
keep the compact menu. The viewer's own human profile also links to its server's
profile settings.

**Props:**
- `user` - The user to display (must include id, login, displayName, presenceStatus)
- `anchorRect` - Bounding rect of the trigger element (used for desktop positioning)
- `position` - Viewport point used by right-click and long-press triggers
- `presentation` - Optional floating/sheet presentation selected by the trigger
- `canSendMessage` - Whether to show the "Open conversation" quick action
- `onSendMessage` - Callback when "Open conversation" is clicked
- `onMention` - Optional callback that mentions the user in the host's composer
- `canBanFromRoom` - Whether to show the room removal action
- `banningFromRoom` - Whether the room removal action is currently running
- `onBanFromRoom` - Callback when "Remove from room" is clicked
- `onOpenProfile` - Optional callback that opens the full room-sidebar profile
- `extraActions` - Optional host actions, rendered before profile actions
- `viewerSettings` - Optional viewer preferences for the user's local-time display
- `onClose` - Callback to close the popover/sheet
-->
<script lang="ts">
  import ProfileVerifiedIdentities from '$lib/components/users/ProfileVerifiedIdentities.svelte';
  import ProfileDetails from '$lib/components/users/ProfileDetails.svelte';
  import ProfileHeader from '$lib/components/users/ProfileHeader.svelte';
  import ProfileQuickActions from '$lib/components/users/ProfileQuickActions.svelte';
  import { searchUserMessages } from '$lib/components/users/searchUserMessages';
  import { serverUi } from '$lib/state/server/serverUi';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { resolve } from '$app/paths';

  import ParticipantAudioControls from '$lib/components/voice/ParticipantAudioControls.svelte';
  import type { ParticipantVolumeControl } from '$lib/state/server/callPreferences.svelte';
  import UserBio from '$lib/components/users/UserBio.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { ContextMenu, ScrollFader, MenuItem, MenuSection } from '$lib/ui';
  import {
    getLiveBio,
    getLiveCustomStatus,
    getLiveDisplayName,
    getLiveLogin,
    getLiveBotOwnerUserId,
    getLiveTimezone
  } from '$lib/state/userProfiles.svelte';
  import type { CustomUserStatus } from '@chatto/client/api/userSummary';
  import { m } from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';
  import type { Snippet } from 'svelte';
  import type { ViewerTimeSettings } from '$lib/utils/formatTime';

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
    extraActions,
    header,
    onSendMessage,
    onBanFromRoom,
    onMention,
    onOpenProfile,
    onClose
  }: {
    user: {
      id: string;
      login: string;
      displayName: string;
      isBot?: boolean;
      /** Public human owner of an active bot. */
      bot?: { ownerUserId: string };
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
    /** Host-owned actions rendered before profile actions, within this menu's context. */
    extraActions?: Snippet;
    /** Optional host content before the profile and its actions. */
    header?: Snippet;
    onSendMessage?: () => void;
    onBanFromRoom?: () => void;
    /** Mention the user in the host's composer. Hosts pass it only while one accepts input. */
    onMention?: (login: string) => void;
    onOpenProfile?: (userId: string) => void;
    onClose?: () => void;
  } = $props();

  const serverScope = useServerScope();
  const voiceCall = $derived(serverUi(serverScope.store).voiceCall);
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
  const login = $derived(getLiveLogin(user.id, user.login));
  const presence = $derived(serverScope.store.presence.get(user.id) ?? user.presenceStatus);
  const botOwnerUserId = $derived(
    user.isBot ? getLiveBotOwnerUserId(user.id, user.bot?.ownerUserId ?? null) : null
  );
  // The bot's owner, bot managers, and account managers can open its management
  // page; the page itself enforces the same rule through BotService.GetBot.
  const manageBotHref = $derived.by(() => {
    if (!user.isBot || user.deleted) return null;
    const permissions = serverScope.store.permissions;
    const isOwner = !!botOwnerUserId && botOwnerUserId === serverScope.store.accountId;
    const canManage =
      permissions.loaded && (permissions.canManageBots || permissions.canAdminManageAccounts);
    if (!isOwner && !canManage) return null;
    return resolve('/chat/[serverId]/manage/server/bots/[botId]', {
      serverId: serverIdToSegment(serverScope.serverId),
      botId: user.id
    });
  });
  const adminUserHref = $derived(
    serverScope.store.permissions.loaded && serverScope.store.permissions.canAdminViewUsers
      ? resolve('/chat/[serverId]/manage/server/members/[userId]', {
          serverId: serverIdToSegment(serverScope.serverId),
          userId: user.id
        })
      : null
  );
  const editProfileHref = $derived(
    !user.isBot && !user.deleted && user.id === serverScope.store.accountId
      ? resolve('/chat/[serverId]/settings/profile', {
          serverId: serverIdToSegment(serverScope.serverId)
        })
      : null
  );
  function handleSendMessage() {
    onSendMessage?.();
    onClose?.();
  }

  function handleMention() {
    onMention?.(login);
    onClose?.();
  }

  function handleSearchMessages() {
    // Closing can clear the host's user, so read the login first.
    const target = login;
    onClose?.();
    searchUserMessages(serverScope.serverId, serverUi(serverScope.store).messageSearch, target);
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
  class="w-72"
  onclose={() => onClose?.()}
>
  {@render header?.()}
  <div class="overflow-hidden inset-content" data-testid="user-menu-profile">
    <ProfileHeader {user} name={displayName} {login} {presence} {customStatus} />
    {#if !user.deleted}
      <ProfileQuickActions
        class="px-3 pb-3"
        onSendMessage={canSendMessage ? handleSendMessage : undefined}
        onMention={onMention ? handleMention : undefined}
        onSearchMessages={handleSearchMessages}
      />
    {/if}
  </div>

  {#if !user.isBot && !user.deleted}
    <ProfileVerifiedIdentities userId={user.id} />
  {/if}

  {#if bio && !user.deleted}
    <div class="menu-section px-3 py-2 text-start">
      <ScrollFader
        top
        bottom
        fill={false}
        fadeHeight="h-5"
        scrollClass="max-h-40 overscroll-contain"
        aria-label={m('settings.profile.bio.label')}
        data-testid="user-menu-bio-scroll"
      >
        <UserBio {bio} />
      </ScrollFader>
    </div>
  {/if}

  {#if !user.deleted}
    <ProfileDetails userId={user.id} {timezone} {viewerSettings} class="menu-section px-3 py-2" />
  {/if}

  {@render extraActions?.()}

  {#if onOpenProfile || editProfileHref || manageBotHref || adminUserHref || canBanFromRoom}
    <MenuSection>
      {#if onOpenProfile}
        <MenuItem icon="icon-[uil--user]" onclick={handleOpenProfile}>
          {m('chat.user_menu.view_profile')}
        </MenuItem>
      {/if}
      {#if editProfileHref}
        <MenuItem
          href={editProfileHref}
          icon="icon-[uil--edit]"
          onclick={() => onClose?.()}
          dataTestid="edit-own-profile"
        >
          {m('chat.user_menu.edit_profile')}
        </MenuItem>
      {/if}
      {#if manageBotHref}
        <MenuItem
          href={manageBotHref}
          icon="icon-[uil--robot]"
          onclick={() => onClose?.()}
          dataTestid="manage-bot"
        >
          {m('chat.user_menu.manage_bot')}
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
          {banningFromRoom ? m('room.moderation.removing') : m('room.moderation.remove_action')}
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
