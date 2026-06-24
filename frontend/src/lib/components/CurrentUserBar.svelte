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
  import { PresenceStatus, RoomType } from '$lib/gql/graphql';
  import {
    roomSidebarPanelStorageSuffix,
    setPendingRoomSidebarPanel,
    setRoomSidebarPanel
  } from '$lib/storage/roomSidebarPanel';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import ContextMenu from '$lib/ui/ContextMenu.svelte';
  import UserAvatar from './UserAvatar.svelte';
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
    if (!room) return 'Current call';
    if (room.type === RoomType.Dm) {
      const meId = roomsStore?.currentUserId;
      const others = room.members.filter((member) => member.id !== meId);
      if (others.length === 0) return 'You';
      return others
        .map((member) => getLiveDisplayName(member.id, member.displayName || member.login))
        .join(', ');
    }
    return `# ${room.name}`;
  });
  const compactCallButtonClass = 'btn-secondary h-7 w-7 shrink-0 !px-0 !py-0 text-xs';
  const compactCallActiveButtonClass = 'btn-success h-7 w-7 shrink-0 !px-0 !py-0 text-xs';
  const compactCallDangerButtonClass = 'btn-danger h-7 w-7 shrink-0 !px-0 !py-0 text-xs';
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
  let statusEditorAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let presenceMenuAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);

  function customStatusAPIConfig() {
    const conn = connection();
    return {
      serverId: activeServerId,
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    };
  }

  function openStatusEditor(event: MouseEvent) {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    statusEditorAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  function openPresenceMenu(event: MouseEvent) {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    presenceMenuAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
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

  function choosePresenceMode(mode: PresenceMode) {
    setPresenceMode(mode);
    presenceMenuAnchor = null;
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
        >
          <span
            class={[
              'iconify',
              voiceCallState.isMuted ? 'uil--microphone-slash' : 'uil--microphone'
            ]}
            aria-hidden="true"
          ></span>
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
        >
          <span
            class={['iconify', voiceCallState.isCameraEnabled ? 'uil--video' : 'uil--video-slash']}
            aria-hidden="true"
          ></span>
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
        >
          <span class="iconify uil--desktop" aria-hidden="true"></span>
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
      <UserAvatar user={activeServerUser} size="md" />
      <div class="flex min-w-0 flex-1 flex-col leading-tight">
        <span class="flex min-w-0 items-center gap-1.5 text-sm font-semibold">
          <span class="min-w-0 truncate">{displayName}</span>
        </span>
        {#if showLogin}
          <span class="truncate text-xs text-muted">@{login}</span>
        {/if}
      </div>
      <button
        type="button"
        title={m['settings.profile.presence.button']({ status: presenceLabel })}
        aria-label={m['settings.profile.presence.button']({ status: presenceLabel })}
        class="grid h-7 w-7 shrink-0 cursor-pointer place-items-center rounded-md hover:bg-surface-100"
        data-testid="current-user-presence-menu"
        onclick={openPresenceMenu}
      >
        <span class={`h-3 w-3 rounded-full ${presenceDotClass}`} aria-hidden="true"></span>
      </button>
      <button
        type="button"
        title={m['settings.profile.status.edit_button']()}
        aria-label={m['settings.profile.status.edit_button']()}
        class="iconify shrink-0 cursor-pointer text-muted uil--pen hover:text-text"
        data-testid="current-user-edit-status"
        onclick={openStatusEditor}
      ></button>
      <a
        href={resolve('/chat/[serverId]/settings', { serverId: serverSegment })}
        title={m['voice.user_settings']()}
        class="iconify shrink-0 cursor-pointer text-muted uil--setting hover:text-text"
      ></a>
    </div>
  </div>
{/if}

{#if presenceMenuAnchor && activeServerUser}
  <ContextMenu
    anchor={presenceMenuAnchor}
    role="menu"
    ariaLabel={m['settings.profile.presence.title']()}
    class="w-56"
    onclose={() => (presenceMenuAnchor = null)}
  >
    <div class="menu-section p-1">
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
          <span
            class={[
              'h-2.5 w-2.5 rounded-full',
              mode === 'auto'
                ? 'bg-green-500'
                : mode === 'away'
                  ? 'bg-yellow-500'
                  : mode === 'doNotDisturb'
                    ? 'bg-red-500'
                    : 'bg-gray-400'
            ]}
            aria-hidden="true"
          ></span>
          <span class="min-w-0 truncate">{presenceModeLabel(mode)}</span>
          {#if presencePreference.mode === mode}
            <span class="iconify ml-auto shrink-0 uil--check" aria-hidden="true"></span>
          {/if}
        </button>
      {/each}
    </div>
  </ContextMenu>
{/if}

{#if statusEditorAnchor && activeServerUser}
  <ContextMenu
    anchor={statusEditorAnchor}
    role="dialog"
    ariaLabel={m['settings.profile.status.edit_button']()}
    class="w-auto"
    onclose={() => (statusEditorAnchor = null)}
  >
    <UserCustomStatusEditor
      status={activeServerUser.customStatus}
      config={customStatusAPIConfig()}
      compact
      onChange={updateCurrentCustomStatus}
      onClose={() => (statusEditorAnchor = null)}
    />
  </ContextMenu>
{/if}
