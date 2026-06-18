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
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { RoomType } from '$lib/gql/graphql';
  import {
    roomSidebarPanelStorageSuffix,
    setPendingRoomSidebarPanel,
    setRoomSidebarPanel
  } from '$lib/storage/roomSidebarPanel';
  import { serverStorageKey } from '$lib/storage/serverStorage';
  import UserAvatar from './UserAvatar.svelte';

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
    activeCallRoomId ? (roomsStore?.rooms.find((room) => room.id === activeCallRoomId) ?? null) : null
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
  const compactCallDangerButtonClass = 'btn-danger h-7 w-7 shrink-0 !px-0 !py-0 text-xs';

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
          <span class="iconify uil--phone shrink-0 animate-pulse text-accent"></span>
          <span class="truncate">{activeCallRoomName}</span>
        </button>
        <button
          type="button"
          class={voiceCallState.isMuted ? compactCallDangerButtonClass : compactCallButtonClass}
          title={voiceCallState.isMuted ? 'Unmute' : 'Mute'}
          aria-label={voiceCallState.isMuted ? 'Unmute' : 'Mute'}
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
          class={voiceCallState.isCameraEnabled ? compactCallButtonClass : compactCallDangerButtonClass}
          title={voiceCallState.isCameraEnabled ? 'Turn off camera' : 'Turn on camera'}
          aria-label={voiceCallState.isCameraEnabled ? 'Turn off camera' : 'Turn on camera'}
          data-testid="current-user-call-camera"
          onclick={() => voiceCallState.toggleCamera()}
        >
          <span
            class={[
              'iconify',
              voiceCallState.isCameraEnabled ? 'uil--video' : 'uil--video-slash'
            ]}
            aria-hidden="true"
          ></span>
        </button>
        <button
          type="button"
          class={compactCallDangerButtonClass}
          title="Leave call"
          aria-label="Leave call"
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
        <span class="truncate text-sm font-semibold">{displayName}</span>
        {#if showLogin}
          <span class="truncate text-xs text-muted">@{login}</span>
        {/if}
      </div>
      <a
        href={resolve('/chat/[serverId]/settings', { serverId: serverSegment })}
        title="User Settings"
        class="iconify shrink-0 cursor-pointer text-muted uil--setting hover:text-text"
      ></a>
    </div>
  </div>
{/if}
