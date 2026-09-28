<!--
@component

Displays the current (server-scoped) user at the bottom of the secondary
sidebar. Shows the avatar with presence and the live display name. Right-click
or touch long-press opens the profile menu; avatar clicks open presence settings.
-->
<script lang="ts">
  import {
    UserCard,
    FadeScale,
    PillButtonGroup,
    ContextMenu,
    ConfirmDialog,
    Dialog,
    MenuItem,
    MenuSection
  } from '$lib/ui';
  import ConnectionQualityHint from './voice/ConnectionQualityHint.svelte';
  import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { serverIdToSegment } from '$lib/navigation';
  import { m } from '$lib/i18n/messages';
  import { deleteCustomStatus } from '$lib/api-client/userStatus';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import {
    getLiveCustomStatus,
    getLiveDisplayName,
    type CustomUserStatus
  } from '$lib/state/userProfiles.svelte';
  import { setPresenceStatus } from '$lib/presenceTracking';
  import { presencePreferences } from '$lib/state/server/presencePreference.svelte';
  import { buildDirectMessagePresentation } from '$lib/render/users';

  import { getAppUiState, getRoomSidebarPresentation } from '$lib/state/appUi.svelte';
  import { Button } from '$lib/ui/form';
  import Deadline from '$lib/lifecycle/Deadline.svelte';
  import { toast } from '$lib/ui/toast';
  import UserAvatar from './UserAvatar.svelte';
  import UserCustomStatusBadge from './UserCustomStatusBadge.svelte';
  import ScreenShareControlButton from './voice/ScreenShareControlButton.svelte';
  import VoiceCallControlButton from './voice/VoiceCallControlButton.svelte';
  import UserMenu from './users/UserMenu.svelte';
  import { UserMenuState } from './users/UserMenuState.svelte';

  const profileMenu = new UserMenuState<string>(() => {
    statusMenuAnchor = null;
  });
  const profileMenuTrigger = profileMenu.trigger(() => activeServerUser?.id ?? null);

  let customStatusEditorModule: Promise<typeof import('./UserCustomStatusEditor.svelte')> | null =
    null;
  let customStatusEditorLoadAttempt = $state(0);

  function loadCustomStatusEditor(_attempt: number) {
    customStatusEditorModule ??= import('./UserCustomStatusEditor.svelte').catch(
      (error: unknown) => {
        customStatusEditorModule = null;
        throw error;
      }
    );
    return customStatusEditorModule;
  }

  const serverScope = useServerScope();
  const appUi = getAppUiState();
  const activeServerId = serverScope.serverId;
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const activeStore = serverScope.store;
  const activeServerUser = $derived(activeStore.viewerUser);
  const presenceScope = $derived(
    activeServerUser ? { serverId: activeServerId, userId: activeServerUser.id } : null
  );
  const presencePreference = $derived(
    presenceScope ? presencePreferences.get(presenceScope) : null
  );
  const voiceCallState = $derived(activeStore.voiceCall);
  const navigation = $derived(activeStore.navigation);

  const displayName = $derived(
    activeServerUser
      ? getLiveDisplayName(
          activeServerUser.id,
          activeServerUser.displayName || activeServerUser.login
        )
      : ''
  );

  const login = $derived(activeServerUser?.login ?? '');
  // The public profile receives status changes from every session of this account.
  const customStatus = $derived(
    activeServerUser
      ? getLiveCustomStatus(activeServerUser.id, activeServerUser.customStatus)
      : null
  );
  const activeCallRoomId = $derived(
    voiceCallState?.connected && voiceCallState.roomId ? voiceCallState.roomId : null
  );
  const activeCallRoom = $derived(
    activeCallRoomId
      ? (navigation?.rooms.find((room) => room.id === activeCallRoomId) ?? null)
      : null
  );
  const activeCallRoomName = $derived.by(() => {
    const room = activeCallRoom;
    if (!room) return m('common.current_call');
    if (room.type === RoomKind.DM) {
      return buildDirectMessagePresentation(
        room.members,
        activeStore.projectionViewerId,
        m('common.you'),
        getLiveDisplayName
      ).label;
    }
    return `# ${room.name}`;
  });
  const compactCallButtonClass = 'pill-button';
  const compactCallActiveButtonClass = 'pill-button-success';
  const compactCallDangerButtonClass = 'pill-button-danger';
  const presenceModes: PresenceStatus[] = [
    PresenceStatus.ONLINE,
    PresenceStatus.AWAY,
    PresenceStatus.DO_NOT_DISTURB,
    PresenceStatus.OFFLINE
  ];
  // The status the server reports for the viewer, then the viewer's own choice.
  const currentPresence = $derived.by(() => {
    if (!activeServerUser) return PresenceStatus.OFFLINE;
    return (
      activeStore.presence.get(activeServerUser.id) ??
      presencePreference?.status ??
      activeServerUser.presenceStatus
    );
  });
  const presenceLabel = $derived.by(() => presenceStatusLabel(currentPresence));
  let statusMenuAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let customStatusDialogVisible = $state(false);
  let clearingCustomStatus = $state(false);
  let privilegedModeDialogVisible = $state(false);
  let privilegedModeLoading = $state(false);
  const privilegedMode = $derived(activeStore.projection.viewer?.privilegedMode);
  const privilegedModeDeadline = $derived(
    privilegedMode?.active ? (privilegedMode.expiresAt?.toDate().getTime() ?? null) : null
  );

  function customStatusAPIConfig() {
    return { ...serverScope.connection.apiConfig, serverId: activeServerId };
  }

  function openStatusMenu(event: MouseEvent) {
    profileMenu.close();
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    statusMenuAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  function presenceModeLabel(mode: PresenceStatus): string {
    switch (mode) {
      case PresenceStatus.AWAY:
        return m('settings.profile.presence.away');
      case PresenceStatus.DO_NOT_DISTURB:
        return m('settings.profile.presence.do_not_disturb');
      case PresenceStatus.OFFLINE:
        return m('settings.profile.presence.invisible');
      default:
        return m('settings.profile.presence.online');
    }
  }

  function presenceStatusLabel(status: PresenceStatus): string {
    switch (status) {
      case PresenceStatus.AWAY:
        return m('settings.profile.presence.away');
      case PresenceStatus.DO_NOT_DISTURB:
        return m('settings.profile.presence.do_not_disturb');
      case PresenceStatus.OFFLINE:
        return m('settings.profile.presence.offline');
      default:
        return m('settings.profile.presence.online');
    }
  }

  function presenceModeDotClass(mode: PresenceStatus): string {
    switch (mode) {
      case PresenceStatus.AWAY:
        return 'bg-presence-away';
      case PresenceStatus.DO_NOT_DISTURB:
        return 'bg-presence-do-not-disturb';
      case PresenceStatus.OFFLINE:
        return 'bg-presence-invisible';
      default:
        return 'bg-presence-online';
    }
  }

  async function choosePresenceMode(mode: PresenceStatus) {
    const scope = presenceScope;
    const store = activeStore;
    try {
      if (scope) {
        await setPresenceStatus(scope, mode);
        // Show the saved choice at once. The server's presence event corrects it
        // if the effective status differs.
        store.presence.set(scope.userId, mode);
      }
      statusMenuAnchor = null;
    } catch {
      toast.error(m('settings.profile.status.save_failed'));
    }
  }

  function openCustomStatusDialog() {
    statusMenuAnchor = null;
    customStatusDialogVisible = true;
  }

  /** Keep a pending clear attached to the account and menu that started it. */
  async function clearCustomStatus() {
    if (clearingCustomStatus || !activeServerUser || !customStatus) return;
    const store = activeStore;
    const userId = activeServerUser.id;
    const serverId = activeServerId;
    const menuAnchor = statusMenuAnchor;
    const config = customStatusAPIConfig();
    clearingCustomStatus = true;
    try {
      const customStatus = await deleteCustomStatus(config);
      store.currentUser.update(userId, () => ({ customStatus }));
      if (activeServerId === serverId && activeServerUser?.id === userId) {
        if (statusMenuAnchor === menuAnchor) statusMenuAnchor = null;
        toast.success(m('settings.profile.status.cleared'));
      }
    } catch {
      if (activeServerId === serverId && activeServerUser?.id === userId) {
        toast.error(m('settings.profile.status.clear_failed'));
      }
    } finally {
      clearingCustomStatus = false;
    }
  }

  function updateCurrentCustomStatus(status: CustomUserStatus | null) {
    const store = activeStore;
    store.currentUser.update(store.accountId, () => ({ customStatus: status }));
  }

  function openActiveCallRoom(): void {
    const roomId = activeCallRoomId;
    if (!roomId) return;

    appUi.requestRoomSidebarPanel(activeServerId, roomId, 'call', getRoomSidebarPresentation());
    goto(
      resolve('/chat/[serverId]/[roomId]', {
        serverId: serverSegment,
        roomId
      })
    );
  }

  async function setPrivilegedMode(active: boolean): Promise<void> {
    if (privilegedModeLoading) return;
    privilegedModeLoading = true;
    try {
      await activeStore.setPrivilegedMode(active);
      privilegedModeDialogVisible = false;
    } catch {
      toast.error(m('common.error.generic'));
    } finally {
      privilegedModeLoading = false;
    }
  }
</script>

{#if privilegedModeDeadline !== null}
  <Deadline at={privilegedModeDeadline} onreached={() => void activeStore.expirePrivilegedMode()} />
{/if}

{#if activeServerUser}
  <div class="flex shrink-0 flex-col gap-1 p-2">
    {#if activeCallRoomId && voiceCallState}
      <FadeScale>
        <PillButtonGroup compact label={m('room.sidebar.call')} testId="current-user-call-card">
          <VoiceCallControlButton
            class={compactCallButtonClass}
            label={m('voice.open_call_room', { room: activeCallRoomName })}
            testId="current-user-call-link"
            icon="icon-[uil--phone]"
            iconClass="text-action"
            onclick={openActiveCallRoom}
          />
          <VoiceCallControlButton
            class={voiceCallState.isMuted ? compactCallButtonClass : compactCallActiveButtonClass}
            label={m('voice.mute')}
            pressed={voiceCallState.isMuted}
            testId="current-user-call-mute"
            icon={voiceCallState.isMuted
              ? 'icon-[uil--microphone-slash]'
              : 'icon-[uil--microphone]'}
            onclick={() => voiceCallState.toggleMute()}
            pending={voiceCallState.isMicrophonePending}
            disabled={!voiceCallState.canUseVoice && voiceCallState.isMuted}
          />
          <VoiceCallControlButton
            class={voiceCallState.isCameraEnabled
              ? compactCallActiveButtonClass
              : compactCallButtonClass}
            label={voiceCallState.isCameraEnabled
              ? m('voice.turn_off_camera')
              : m('voice.turn_on_camera')}
            testId="current-user-call-camera"
            icon={voiceCallState.isCameraEnabled ? 'icon-[uil--video]' : 'icon-[uil--video-slash]'}
            onclick={() => voiceCallState.toggleCamera()}
            pending={voiceCallState.isCameraPending}
            disabled={!voiceCallState.canUseCamera && !voiceCallState.isCameraEnabled}
          />
          <ScreenShareControlButton
            {voiceCallState}
            class={voiceCallState.isScreenShareEnabled
              ? compactCallActiveButtonClass
              : compactCallButtonClass}
            testId="current-user-call-screen-share"
          />
          <VoiceCallControlButton
            class={compactCallDangerButtonClass}
            label={m('voice.leave')}
            testId="current-user-call-leave"
            icon="icon-[uil--phone-slash]"
            onclick={() => voiceCallState.leave()}
          />
        </PillButtonGroup>
      </FadeScale>
    {/if}

    <div {@attach profileMenuTrigger}>
      <UserCard
        variant="card"
        name={displayName}
        identity={activeServerUser}
        username={login}
        testId="current-user-identity-card"
        textTestId="current-user-identity-text"
        secondaryTestId="current-user-login"
      >
        {#snippet avatar()}
          <button
            type="button"
            title={m('settings.profile.presence.button', { status: presenceLabel })}
            aria-label={m('settings.profile.presence.button', { status: presenceLabel })}
            class="flex h-10 shrink-0 cursor-pointer items-center rounded-full"
            data-testid="current-user-presence-menu"
            onclick={openStatusMenu}
          >
            <UserAvatar user={activeServerUser} presence={currentPresence} size="sm" showPresence />
          </button>
        {/snippet}
        {#snippet badges()}
          <UserCustomStatusBadge status={customStatus} class="text-xs" />
        {/snippet}
        {#snippet actions()}
          {#if voiceCallState?.connected}
            <ConnectionQualityHint
              quality={voiceCallState.participants.find((p) => p.isLocal)?.connectionQuality}
            />
          {/if}
          {#if privilegedMode?.available}
            <button
              type="button"
              class={['user-card-toggle', privilegedMode.active && 'user-card-toggle-active']}
              title={privilegedMode.active
                ? m('chat.privileged_mode.disable')
                : m('chat.privileged_mode.enable')}
              aria-label={privilegedMode.active
                ? m('chat.privileged_mode.disable')
                : m('chat.privileged_mode.enable')}
              data-testid="privileged-mode-toggle"
              disabled={privilegedModeLoading}
              onclick={() =>
                privilegedMode.active
                  ? void setPrivilegedMode(false)
                  : (privilegedModeDialogVisible = true)}
            >
              <span
                class={[
                  'iconify text-lg',
                  privilegedMode.active ? 'icon-[uil--shield-check]' : 'icon-[uil--shield]'
                ]}
                aria-hidden="true"
              ></span>
            </button>
          {/if}
        {/snippet}
      </UserCard>
    </div>
  </div>
{/if}

<ConfirmDialog
  bind:visible={privilegedModeDialogVisible}
  title={m('chat.privileged_mode.enable')}
  tone="warning"
  actionLabel={m('chat.privileged_mode.enable')}
  actionIcon="iconify icon-[uil--shield-check]"
  loading={privilegedModeLoading}
  onconfirm={() => void setPrivilegedMode(true)}
  onclose={() => (privilegedModeDialogVisible = false)}
>
  {m('chat.privileged_mode.confirmation')}
</ConfirmDialog>

<UserMenu state={profileMenu} user={activeServerUser} viewerSettings={activeServerUser?.settings} />

{#if statusMenuAnchor && activeServerUser}
  <ContextMenu
    anchor={statusMenuAnchor}
    role="dialog"
    ariaLabel={m('settings.profile.status.edit_button')}
    class="w-80 max-w-[calc(100vw-2rem)]"
    onclose={() => (statusMenuAnchor = null)}
  >
    <MenuSection ariaLabel={m('settings.profile.presence.title_server')}>
      <div class="px-2 py-1 font-semibold text-muted">
        {m('settings.profile.presence.title_server')}
      </div>
      {#each presenceModes as mode (mode)}
        <MenuItem
          role="menuitemradio"
          checked={presencePreference?.status === mode}
          disabled={!presencePreference?.ready}
          selected={presencePreference?.status === mode}
          onclick={() => choosePresenceMode(mode)}
        >
          {#snippet leading()}
            <span class="grid size-full place-items-center">
              <span class={['h-2.5 w-2.5 rounded-full', presenceModeDotClass(mode)]}></span>
            </span>
          {/snippet}
          {#snippet trailing()}
            {#if presencePreference?.status === mode}
              <span class="iconify icon-[uil--check]" aria-hidden="true"></span>
            {/if}
          {/snippet}
          <span class="block truncate">{presenceModeLabel(mode)}</span>
        </MenuItem>
      {/each}
    </MenuSection>
    <MenuSection>
      {#if customStatus}
        <MenuItem
          dataTestid="current-user-clear-status-action"
          icon="icon-[uil--times]"
          busy={clearingCustomStatus}
          onclick={clearCustomStatus}
        >
          {m('settings.profile.status.clear_button')}
        </MenuItem>
      {/if}
      <MenuItem
        dataTestid="current-user-custom-status-action"
        disabled={clearingCustomStatus}
        onclick={openCustomStatusDialog}
      >
        {#snippet leading()}
          <span class="grid size-full place-items-center">
            {#if customStatus}
              {customStatus.emoji}
            {:else}
              <span class="iconify icon-[uil--comment-alt-edit] text-muted" aria-hidden="true"
              ></span>
            {/if}
          </span>
        {/snippet}
        <span class="block truncate">{m('settings.profile.status.set_custom_status')}</span>
      </MenuItem>
    </MenuSection>
  </ContextMenu>
{/if}

{#if activeServerUser && customStatusDialogVisible}
  {#await loadCustomStatusEditor(customStatusEditorLoadAttempt) then { default: UserCustomStatusEditor }}
    <UserCustomStatusEditor
      bind:visible={customStatusDialogVisible}
      status={customStatus}
      config={customStatusAPIConfig()}
      onChange={updateCurrentCustomStatus}
      onClose={() => (customStatusDialogVisible = false)}
    />
  {:catch}
    <Dialog
      bind:visible={customStatusDialogVisible}
      title={m('settings.profile.status.dialog_title')}
      size="sm"
      onclose={() => (customStatusDialogVisible = false)}
    >
      <p class="text-muted" role="alert">{m('common.error.network')}</p>
      {#snippet primaryAction()}
        <Button onclick={() => (customStatusEditorLoadAttempt += 1)}>
          {m('common.retry')}
        </Button>
      {/snippet}
    </Dialog>
  {/await}
{/if}
