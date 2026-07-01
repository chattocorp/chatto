<!--
@component

Displays the current (server-scoped) user at the bottom of the secondary
sidebar. Shows the avatar with presence and the live display name, and links
to the user settings page for the active server.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { serverIdToSegment } from '$lib/navigation';
  import * as m from '$lib/i18n/messages';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { getLiveDisplayName, type CustomUserStatus } from '$lib/state/userProfiles.svelte';
  import { setPresenceMode } from '$lib/presenceTracking';
  import { presencePreference, type PresenceMode } from '$lib/state/presencePreference.svelte';
  import { PresenceStatus, RoomType } from '$lib/render/types';
  import {
    roomSidebarPanelStorageSuffix,
    setPendingRoomSidebarPanel,
    setRoomSidebarPanel
  } from '$lib/storage/roomSidebarPanel';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import { isTouchDevice } from '$lib/utils/isTouchDevice';
  import BottomSheet from '$lib/ui/BottomSheet.svelte';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import Dialog from '$lib/ui/Dialog.svelte';
  import UserAvatar from './UserAvatar.svelte';
  import UserCustomStatusBadge from './UserCustomStatusBadge.svelte';
  import UserCustomStatusEditor from './UserCustomStatusEditor.svelte';

  const connection = useConnection();
  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const activeStore = $derived(serverRegistry.tryGetStore(activeServerId));
  const activeServerUser = $derived(activeStore?.currentUser.user);
  const voiceCallState = $derived(activeStore?.voiceCall);
  const roomsStore = $derived(activeStore?.rooms);

  const displayName = $derived(
    activeServerUser
      ? getLiveDisplayName(
          activeServerUser.id,
          activeServerUser.displayName || activeServerUser.login
        )
      : ''
  );

  const login = $derived(activeServerUser?.login ?? '');
  const showLogin = $derived(!!login && login !== displayName);
  const activeCallRoomId = $derived(
    voiceCallState?.connected && voiceCallState.roomId ? voiceCallState.roomId : null
  );
  const activeCallRoom = $derived(
    activeCallRoomId
      ? (roomsStore?.rooms.find((room) => room.id === activeCallRoomId) ?? null)
      : null
  );
  const activeCallRoomName = $derived.by(() => {
    const room = activeCallRoom;
    if (!room) return m['common.current_call']();
    if (room.type === RoomType.Dm) {
      const meId = roomsStore?.currentUserId;
      const others = room.members.filter((member) => member.id !== meId);
      if (others.length === 0) return m['common.you']();
      return others
        .map((member) => getLiveDisplayName(member.id, member.displayName || member.login))
        .join(', ');
    }
    return `# ${room.name}`;
  });
  const compactCallButtonClass = 'btn-secondary h-7 w-7 shrink-0 !px-0 !py-0 text-xs';
  const compactCallActiveButtonClass = 'btn-success h-7 w-7 shrink-0 !px-0 !py-0 text-xs';
  const compactCallDangerButtonClass = 'btn-danger h-7 w-7 shrink-0 !px-0 !py-0 text-xs';
  const isTouch = isTouchDevice();
  const presenceModes: PresenceMode[] = ['auto', 'away', 'doNotDisturb', 'invisible'];
  const presenceLabel = $derived.by(() => presenceModeLabel(presencePreference.mode));
  const presenceDotClass = $derived(
    presencePreference.effectiveStatus === PresenceStatus.Online
      ? 'bg-green-500'
      : presencePreference.effectiveStatus === PresenceStatus.Away
        ? 'bg-yellow-500'
        : presencePreference.effectiveStatus === PresenceStatus.DoNotDisturb
          ? 'bg-red-500'
          : 'bg-gray-400'
  );
  let statusMenuAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let customStatusDialogVisible = $state(false);

  function customStatusAPIConfig() {
    const conn = connection();
    return {
      serverId: activeServerId,
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    };
  }

  function openStatusMenu(event: MouseEvent) {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    statusMenuAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  function presenceModeLabel(mode: PresenceMode): string {
    switch (mode) {
      case 'away':
        return m['settings.profile.presence.away']();
      case 'doNotDisturb':
        return m['settings.profile.presence.do_not_disturb']();
      case 'invisible':
        return m['settings.profile.presence.invisible']();
      default:
        return m['settings.profile.presence.auto']();
    }
  }

  function presenceModeDotClass(mode: PresenceMode): string {
    switch (mode) {
      case 'away':
        return 'bg-yellow-500';
      case 'doNotDisturb':
        return 'bg-red-500';
      case 'invisible':
        return 'bg-gray-400';
      default:
        return 'bg-green-500';
    }
  }

  function choosePresenceMode(mode: PresenceMode) {
    setPresenceMode(mode);
    statusMenuAnchor = null;
  }

  function openCustomStatusDialog() {
    statusMenuAnchor = null;
    customStatusDialogVisible = true;
  }

  function updateCurrentCustomStatus(status: CustomUserStatus | null) {
    const store = activeStore;
    if (!store?.currentUser.user) return;
    store.currentUser.user = {
      ...store.currentUser.user,
      customStatus: status
    };
  }

  function openActiveCallRoom(): void {
    const roomId = activeCallRoomId;
    if (!roomId) return;

    setRoomSidebarPanel(activeServerId, roomId, 'call');
    setPendingRoomSidebarPanel(activeServerId, roomId, 'call');
    window.dispatchEvent(
      new StorageEvent('storage', {
        key: serverStorageKey(activeServerId, roomSidebarPanelStorageSuffix(roomId)),
        newValue: 'call'
      })
    );
    goto(
      resolve('/chat/[serverId]/[roomId]', {
        serverId: serverSegment,
        roomId
      })
    );
  }
</script>

{#snippet customStatusEditor(sheet = false)}
  {#if activeServerUser}
    <UserCustomStatusEditor
      status={activeServerUser.customStatus}
      config={customStatusAPIConfig()}
      {sheet}
      onChange={updateCurrentCustomStatus}
      onClose={() => (customStatusDialogVisible = false)}
    />
  {/if}
{/snippet}

{#if activeServerUser}
  <div class="flex shrink-0 flex-col gap-1 p-2">
    {#if activeCallRoomId && voiceCallState}
      <div
        class="flex min-w-0 items-center gap-1.5 rounded-xl bg-surface p-1"
        data-testid="current-user-call-card"
      >
        <button
          type="button"
          class="btn-secondary h-7 min-w-0 flex-1 cursor-pointer !justify-start !px-2 !py-0 text-xs"
          title={`Open ${activeCallRoomName}`}
          data-testid="current-user-call-link"
          onclick={openActiveCallRoom}
        >
          <span class="iconify shrink-0 animate-pulse text-accent uil--phone"></span>
          <span class="truncate">{activeCallRoomName}</span>
        </button>
        <button
          type="button"
          class={voiceCallState.isMuted ? compactCallButtonClass : compactCallActiveButtonClass}
          title={voiceCallState.isMuted ? m['voice.unmute']() : m['voice.mute']()}
          aria-label={voiceCallState.isMuted ? m['voice.unmute']() : m['voice.mute']()}
          data-testid="current-user-call-mute"
          onclick={() => voiceCallState.toggleMute()}
          disabled={voiceCallState.isMicrophonePending}
          aria-busy={voiceCallState.isMicrophonePending || undefined}
        >
          {#if voiceCallState.isMicrophonePending}
            <span class="iconify animate-spin uil--spinner" aria-hidden="true"></span>
          {:else}
            <span
              class={[
                'iconify',
                voiceCallState.isMuted ? 'uil--microphone-slash' : 'uil--microphone'
              ]}
              aria-hidden="true"
            ></span>
          {/if}
        </button>
        <button
          type="button"
          class={voiceCallState.isCameraEnabled
            ? compactCallActiveButtonClass
            : compactCallButtonClass}
          title={voiceCallState.isCameraEnabled
            ? m['voice.turn_off_camera']()
            : m['voice.turn_on_camera']()}
          aria-label={voiceCallState.isCameraEnabled
            ? m['voice.turn_off_camera']()
            : m['voice.turn_on_camera']()}
          data-testid="current-user-call-camera"
          onclick={() => voiceCallState.toggleCamera()}
          disabled={voiceCallState.isCameraPending}
          aria-busy={voiceCallState.isCameraPending || undefined}
        >
          {#if voiceCallState.isCameraPending}
            <span class="iconify animate-spin uil--spinner" aria-hidden="true"></span>
          {:else}
            <span
              class={[
                'iconify',
                voiceCallState.isCameraEnabled ? 'uil--video' : 'uil--video-slash'
              ]}
              aria-hidden="true"
            ></span>
          {/if}
        </button>
        <button
          type="button"
          class={voiceCallState.isScreenShareEnabled
            ? compactCallActiveButtonClass
            : compactCallButtonClass}
          title={voiceCallState.isScreenShareEnabled
            ? m['voice.stop_share_screen']()
            : m['voice.share_screen']()}
          aria-label={voiceCallState.isScreenShareEnabled
            ? m['voice.stop_share_screen']()
            : m['voice.share_screen']()}
          data-testid="current-user-call-screen-share"
          onclick={() => voiceCallState.toggleScreenShare()}
          disabled={voiceCallState.isScreenSharePending}
          aria-busy={voiceCallState.isScreenSharePending || undefined}
        >
          {#if voiceCallState.isScreenSharePending}
            <span class="iconify animate-spin uil--spinner" aria-hidden="true"></span>
          {:else}
            <span class="iconify uil--desktop" aria-hidden="true"></span>
          {/if}
        </button>
        <button
          type="button"
          class={compactCallDangerButtonClass}
          title={m['voice.leave']()}
          aria-label={m['voice.leave']()}
          data-testid="current-user-call-leave"
          onclick={() => voiceCallState.leave()}
        >
          <span class="iconify uil--phone-slash" aria-hidden="true"></span>
        </button>
      </div>
    {/if}

    <div
      class="flex items-center gap-3 rounded-xl bg-surface py-1 pr-3 pl-1"
      data-testid="current-user-identity-card"
    >
      <button
        type="button"
        title={m['settings.profile.presence.button']({ status: presenceLabel })}
        aria-label={m['settings.profile.presence.button']({ status: presenceLabel })}
        class="relative shrink-0 cursor-pointer rounded-full"
        data-testid="current-user-presence-menu"
        onclick={openStatusMenu}
      >
        <UserAvatar user={activeServerUser} size="md" />
        <span
          class="absolute right-0 bottom-0 grid h-4 w-4 translate-x-1/4 translate-y-1/4 place-items-center rounded-full border-2 border-surface bg-surface"
          aria-hidden="true"
        >
          <span class={['h-2.5 w-2.5 rounded-full', presenceDotClass]}></span>
        </span>
      </button>
      <div class="flex min-w-0 flex-1 flex-col leading-tight">
        <span class="flex min-w-0 items-center gap-1.5 text-sm font-semibold">
          <span class="min-w-0 truncate">{displayName}</span>
          <UserCustomStatusBadge status={activeServerUser.customStatus} class="text-xs" />
        </span>
        {#if showLogin}
          <span class="truncate text-xs text-muted">@{login}</span>
        {/if}
      </div>
      <a
        href={resolve('/chat/[serverId]/settings', { serverId: serverSegment })}
        title={m['voice.user_settings']()}
        class="iconify shrink-0 cursor-pointer text-muted uil--setting hover:text-text"
      ></a>
    </div>
  </div>
{/if}

{#if statusMenuAnchor && activeServerUser}
  <ContextMenu
    anchor={statusMenuAnchor}
    role="dialog"
    ariaLabel={m['settings.profile.status.edit_button']()}
    class="w-80 max-w-[calc(100vw-2rem)]"
    onclose={() => (statusMenuAnchor = null)}
  >
    <div class="flex w-full flex-col gap-1">
      <div class="menu-section p-1">
        <div class="px-2 py-1 text-xs font-semibold text-muted">
          {m['settings.profile.presence.title']()}
        </div>
        {#each presenceModes as mode (mode)}
          <button
            type="button"
            class={[
              'sidebar-item w-full gap-3 text-left',
              presencePreference.mode === mode ? 'bg-surface-100' : ''
            ]}
            role="menuitemradio"
            aria-checked={presencePreference.mode === mode}
            onclick={() => choosePresenceMode(mode)}
          >
            <span class="grid w-5 shrink-0 place-items-center" aria-hidden="true">
              <span class={['h-2.5 w-2.5 rounded-full', presenceModeDotClass(mode)]}></span>
            </span>
            <span class="min-w-0 truncate">{presenceModeLabel(mode)}</span>
            {#if presencePreference.mode === mode}
              <span class="ml-auto iconify shrink-0 uil--check" aria-hidden="true"></span>
            {/if}
          </button>
        {/each}
      </div>
      <div class="menu-section p-1">
        <button
          type="button"
          class="sidebar-item w-full gap-3 text-left"
          data-testid="current-user-custom-status-action"
          onclick={openCustomStatusDialog}
        >
          <span class="grid w-5 shrink-0 place-items-center" aria-hidden="true">
            {#if activeServerUser.customStatus}
              {activeServerUser.customStatus.emoji}
            {:else}
              <span class="iconify text-muted uil--comment-alt-edit"></span>
            {/if}
          </span>
          <span class="min-w-0 truncate">
            {m['settings.profile.status.set_custom_status']()}
          </span>
        </button>
      </div>
    </div>
  </ContextMenu>
{/if}

{#if activeServerUser}
  {#if isTouch}
    <BottomSheet
      bind:visible={customStatusDialogVisible}
      onclose={() => (customStatusDialogVisible = false)}
    >
      <div class="flex max-h-[78vh] flex-col gap-2 overflow-y-auto pb-2 text-text">
        <header class="flex items-center justify-between gap-3 menu-section px-3 py-2">
          <h2 class="text-base font-semibold text-text">
            {m['settings.profile.status.dialog_title']()}
          </h2>
          <button
            type="button"
            onclick={() => (customStatusDialogVisible = false)}
            class="-m-1 grid h-8 w-8 shrink-0 cursor-pointer place-items-center rounded-md text-text/50 transition-colors hover:bg-surface-100 hover:text-text"
            aria-label={m['ui.close']()}
          >
            <span class="iconify text-xl uil--times"></span>
          </button>
        </header>
        {@render customStatusEditor(true)}
      </div>
    </BottomSheet>
  {:else}
    <Dialog
      bind:visible={customStatusDialogVisible}
      title={m['settings.profile.status.dialog_title']()}
      size="md"
      onclose={() => (customStatusDialogVisible = false)}
    >
      {@render customStatusEditor()}
    </Dialog>
  {/if}
{/if}
