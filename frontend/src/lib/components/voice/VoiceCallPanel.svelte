<!--
@component

Room sidebar panel for voice/video calls.

**Two modes:**
- **Observer mode**: Call is active but user hasn't joined. Shows participants
  from server state and a Join button.
- **Participant mode**: User is connected to LiveKit. Shows live audio levels,
  mute toggle, audio device selector, and hang-up button.

**Props:**
- `roomId` - The room ID
- `livekitUrl` - The LiveKit server WebSocket URL (needed for joining)
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const stores = serverRegistry.getStore(getActiveServer());
  const voiceCallState = stores.voiceCall;
  const activeCallRooms = stores.activeCallRooms;
  const callParticipantsState = stores.callParticipants;
  import { useEvent } from '$lib/hooks';
  import { useFragment } from '$lib/gql';
  import { UserAvatarFragment } from '$lib/components/UserAvatar.svelte';
  import type { PresenceStatus } from '$lib/gql/graphql';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import VideoThumbnail from './VideoThumbnail.svelte';
  import AudioDeviceMenu from './AudioDeviceMenu.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import type { Track } from 'livekit-client';
  import { startDMWith } from '$lib/dm/startDM';
  import { toast } from '$lib/ui/toast';

  let {
    roomId,
    livekitUrl
  }: {
    roomId: string;
    livekitUrl: string;
  } = $props();

  let isInThisCall = $derived(voiceCallState.isInCall(roomId));
  let isInAnotherCall = $derived(voiceCallState.isInAnyCall && !isInThisCall);
  let isConnecting = $derived(voiceCallState.connecting && voiceCallState.roomId === roomId);
  let hasActiveCall = $derived(activeCallRooms.has(roomId));
  let deviceMenuAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);

  // The call tab can be opened directly from a room even if the sidebar room
  // list has not refreshed its active-call snapshot yet. Refresh here so
  // observers see the active participants before deciding whether to join.
  $effect(() => {
    if (!isInThisCall) void activeCallRooms.load();
  });

  // Load server-side participants when there's an active call and we're not in it
  $effect(() => {
    if (!isInThisCall && hasActiveCall) {
      callParticipantsState.load(roomId);
    } else if (!hasActiveCall && !isInThisCall) {
      callParticipantsState.clear();
    }
  });

  // Handle call join/leave events to optimistically update the observer participant list
  useEvent((spaceEvent) => {
    const event = spaceEvent.event;
    if (!event) return;

    if (event.__typename === 'CallParticipantJoinedEvent' && event.roomId === roomId) {
      const actor = spaceEvent.actor ? useFragment(UserAvatarFragment, spaceEvent.actor) : null;
      callParticipantsState.handleJoin(event.roomId, event.callId, actor);
    } else if (event.__typename === 'CallParticipantLeftEvent' && event.roomId === roomId) {
      callParticipantsState.handleLeave(event.roomId, event.callId, spaceEvent.actorId ?? null);
      voiceCallState.handleParticipantLeftEvent(
        event.roomId,
        event.callId,
        spaceEvent.actorId ?? null,
        stores.rooms.currentUserId
      );
    } else if (event.__typename === 'CallEndedEvent' && event.roomId === roomId) {
      callParticipantsState.handleEnd(event.roomId, event.callId);
      activeCallRooms.handleEnd(event.roomId, event.callId);
      voiceCallState.handleCallEndedEvent(event.roomId, event.callId);
    }
  });

  /** Unified participant shape for rendering (structural data only). */
  type DisplayParticipant = {
    key: string;
    displayName: string;
    avatarUser: {
      id: string;
      login: string;
      displayName: string;
      avatarUrl: string | null;
      presenceStatus: PresenceStatus;
    };
    isMuted: boolean;
    connectionQuality: string;
    isCameraEnabled: boolean;
    videoTrack: Track | null;
  };

  let participants: DisplayParticipant[] = $derived.by(() => {
    if (isInThisCall) {
      return voiceCallState.participants.map((p) => ({
        key: p.identity,
        displayName: p.name,
        avatarUser: {
          id: p.identity,
          login: p.login,
          displayName: p.name,
          avatarUrl: p.avatarUrl,
          presenceStatus: 'ONLINE' as PresenceStatus
        },
        isMuted: p.isMuted,
        connectionQuality: p.connectionQuality,
        isCameraEnabled: p.isCameraEnabled,
        videoTrack: p.videoTrack
      }));
    }

    return callParticipantsState.participants.map((p) => ({
      key: p.userId,
      displayName: p.displayName,
      avatarUser: {
        id: p.userId,
        login: p.login,
        displayName: p.displayName,
        avatarUrl: p.avatarUrl,
        presenceStatus: 'ONLINE' as PresenceStatus
      },
      isMuted: false,
      connectionQuality: 'unknown',
      isCameraEnabled: false,
      videoTrack: null
    }));
  });

  let sortedParticipants = $derived(
    [...participants].sort((a, b) => {
      if (a.isCameraEnabled && a.videoTrack && !(b.isCameraEnabled && b.videoTrack)) return -1;
      if (b.isCameraEnabled && b.videoTrack && !(a.isCameraEnabled && a.videoTrack)) return 1;
      return 0;
    })
  );
  let videoParticipants = $derived(sortedParticipants.filter((p) => p.isCameraEnabled && p.videoTrack));
  let isIdle = $derived(!hasActiveCall && !isInThisCall);
  let joinLabel = $derived.by(() => {
    if (isConnecting) return hasActiveCall ? 'Joining...' : 'Starting...';
    return hasActiveCall ? 'Join call' : 'Start call';
  });
  const controlButtonClass = 'btn-secondary btn-sm h-9 w-full !px-0';
  const dangerControlButtonClass = 'btn-danger btn-sm h-9 w-full !px-0';

  function hasVideo(participant: DisplayParticipant) {
    return participant.isCameraEnabled && participant.videoTrack;
  }

  function hasConnectionWarning(participant: DisplayParticipant) {
    return participant.connectionQuality === 'poor' || participant.connectionQuality === 'lost';
  }

  function participantTitle(participant: DisplayParticipant) {
    if (isInThisCall && hasConnectionWarning(participant)) {
      return `${participant.displayName} — poor connection`;
    }

    return participant.displayName;
  }

  // --- Imperative audio level ring animation ---
  // Reads from voiceCallState.getAudioLevel() (non-reactive) and directly
  // mutates DOM elements at ~60ms. Completely bypasses Svelte's reactive graph.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative DOM ref map, not read reactively
  const buttonRefs = new Map<string, HTMLElement>();

  function trackButton(node: HTMLElement, identity: string) {
    buttonRefs.set(identity, node);
    return {
      update(newIdentity: string) {
        buttonRefs.delete(identity);
        identity = newIdentity;
        buttonRefs.set(identity, node);
      },
      destroy() {
        buttonRefs.delete(identity);
      }
    };
  }

  $effect(() => {
    if (!isInThisCall) return;

    const interval = setInterval(() => {
      for (const [identity, button] of buttonRefs) {
        const { isSpeaking, audioLevel } = voiceCallState.getAudioLevel(identity);
        const ringOpacity = audioLevel > 0.01 ? 0.3 + Math.pow(audioLevel, 0.3) * 0.7 : 0;
        button.style.setProperty('--ring-opacity', String(ringOpacity));
        button.classList.toggle('voice-ring-speaking', ringOpacity > 0);

        // Also update muted ring (muted + speaking should show speaking ring)
        if (button.classList.contains('voice-ring-muted') && isSpeaking) {
          button.classList.remove('voice-ring-muted');
        }
      }
    }, 60);

    return () => clearInterval(interval);
  });

  // DM start capability
  const serverPerms = getServerPermissions();
  const canStartDMs = $derived(serverPerms.current.canStartDMs);

  // User context menu popover
  let popoverParticipant = $state<DisplayParticipant | null>(null);
  let popoverAnchorRect = $state<{ top: number; bottom: number; left: number } | null>(null);

  function showUserMenu(participant: DisplayParticipant, e: MouseEvent) {
    const button = (e.target as HTMLElement).closest('button');
    const rect = button?.getBoundingClientRect();
    if (!rect) return;
    popoverParticipant = participant;
    popoverAnchorRect = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  function closeUserMenu() {
    popoverParticipant = null;
    popoverAnchorRect = null;
  }

  function openDeviceMenu(e: MouseEvent) {
    const button = e.currentTarget as HTMLElement;
    const rect = button.getBoundingClientRect();
    voiceCallState.refreshDevices();
    deviceMenuAnchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  async function handleJoin() {
    try {
      await voiceCallState.join(livekitUrl, roomId);
    } catch {
      stores.handleVoiceCallJoinFailed(roomId);
      toast.error('Failed to join voice call');
    }
  }
</script>

{#snippet participantCard(participant: DisplayParticipant, mode: 'compact' | 'video')}
  {@const showVideo = mode === 'video' && hasVideo(participant)}
  {#if isInThisCall}
    <button
      type="button"
      class={[
        'participant-card voice-ring voice-ring-card flex w-full cursor-pointer flex-col overflow-hidden rounded-md border border-border bg-surface-100 text-left text-text transition-colors hover:bg-surface-200',
        mode === 'video' ? 'participant-card-video' : 'participant-card-compact',
        participant.isMuted && 'voice-ring-muted'
      ]}
      use:trackButton={participant.key}
      title={participantTitle(participant)}
      data-testid="call-participant-card"
      onclick={(e) => showUserMenu(participant, e)}
    >
      <div class="flex min-w-0 items-center gap-2 p-2">
        <UserAvatar user={participant.avatarUser} size="sm" showPresence={false} />
        <span class="min-w-0 flex-1 truncate text-sm font-medium">{participant.displayName}</span>
        <span class="inline-flex min-w-4 shrink-0 items-center justify-end gap-1.5 text-sm">
          {#if participant.isMuted}
            <span class="iconify uil--microphone-slash text-danger" aria-label="Muted"></span>
          {/if}
          {#if hasConnectionWarning(participant)}
            <span
              class="iconify uil--exclamation-triangle"
              class:text-danger={participant.connectionQuality === 'lost'}
              class:text-warning={participant.connectionQuality === 'poor'}
              aria-label="Poor connection"
            ></span>
          {/if}
        </span>
      </div>

      {#if showVideo}
        <div class="p-2 pt-0">
          <VideoThumbnail
            track={participant.videoTrack!}
            name={participant.displayName}
            user={participant.avatarUser}
            showIdentityOverlay={false}
          />
        </div>
      {/if}
    </button>
  {:else}
    <button
      type="button"
      class={[
        'participant-card flex w-full cursor-pointer flex-col overflow-hidden rounded-md border border-border bg-surface-100 text-left text-text transition-colors hover:bg-surface-200',
        mode === 'video' ? 'participant-card-video' : 'participant-card-compact'
      ]}
      title={participantTitle(participant)}
      data-testid="call-participant-card"
      onclick={(e) => showUserMenu(participant, e)}
    >
      <div class="flex min-w-0 items-center gap-2 p-2">
        <UserAvatar user={participant.avatarUser} size="sm" showPresence={false} />
        <span class="min-w-0 flex-1 truncate text-sm font-medium">{participant.displayName}</span>
      </div>

      {#if showVideo}
        <div class="p-2 pt-0">
          <VideoThumbnail
            track={participant.videoTrack!}
            name={participant.displayName}
            user={participant.avatarUser}
            showIdentityOverlay={false}
          />
        </div>
      {/if}
    </button>
  {/if}
{/snippet}

<div
  class="flex min-h-0 flex-1 flex-col"
  data-testid={isInThisCall ? 'call-participant-panel' : 'call-observer-panel'}
>
  <div class="border-b border-border bg-background p-3">
    {#if isInThisCall}
      <div class="grid grid-cols-4 gap-2">
        <button
          type="button"
          class={controlButtonClass}
          title="Devices"
          aria-label="Devices"
          data-testid="call-device-menu-button"
          onclick={openDeviceMenu}
        >
          <span class="iconify uil--setting text-lg" aria-hidden="true"></span>
        </button>

        <button
          type="button"
          class={voiceCallState.isCameraEnabled ? controlButtonClass : dangerControlButtonClass}
          title={voiceCallState.isCameraEnabled ? 'Turn off camera' : 'Turn on camera'}
          aria-label={voiceCallState.isCameraEnabled ? 'Turn off camera' : 'Turn on camera'}
          data-testid="call-camera-toggle"
          onclick={() => voiceCallState.toggleCamera()}
        >
          <span
            class={[
              'iconify text-lg',
              voiceCallState.isCameraEnabled ? 'uil--video' : 'uil--video-slash'
            ]}
            aria-hidden="true"
          ></span>
        </button>

        <button
          type="button"
          class={voiceCallState.isMuted ? dangerControlButtonClass : controlButtonClass}
          title={voiceCallState.isMuted ? 'Unmute' : 'Mute'}
          aria-label={voiceCallState.isMuted ? 'Unmute' : 'Mute'}
          data-testid="call-mute-toggle"
          onclick={() => voiceCallState.toggleMute()}
        >
          <span
            class={[
              'iconify text-lg',
              voiceCallState.isMuted ? 'uil--microphone-slash' : 'uil--microphone'
            ]}
            aria-hidden="true"
          ></span>
        </button>

        <button
          type="button"
          class={dangerControlButtonClass}
          onclick={() => voiceCallState.leave()}
          title="Leave call"
          aria-label="Leave call"
          data-testid="call-leave-button"
        >
          <span class="iconify uil--phone-slash text-lg" aria-hidden="true"></span>
        </button>
      </div>
    {:else}
      <button
        type="button"
        class="btn-accent btn-sm w-full"
        data-testid="call-join-button"
        onclick={handleJoin}
        disabled={isInAnotherCall || isConnecting}
        title={isInAnotherCall ? 'Already in another call' : joinLabel}
      >
        {joinLabel}
      </button>
    {/if}
  </div>

  <div class="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-3">
    {#if !isIdle}
      <section class="@container flex flex-col gap-2" aria-label="Call participants">
        <div
          class={[
            'grid grid-cols-1 gap-3',
            isInThisCall && videoParticipants.length > 1 && '@min-[368px]:grid-cols-2'
          ]}
          data-testid="call-participants-list"
        >
          {#each sortedParticipants as participant (participant.key)}
            {@render participantCard(participant, isInThisCall && hasVideo(participant) ? 'video' : 'compact')}
          {/each}
        </div>
      </section>
    {/if}
  </div>
</div>

{#if deviceMenuAnchor}
  <AudioDeviceMenu anchor={deviceMenuAnchor} onclose={() => (deviceMenuAnchor = null)} />
{/if}

{#if popoverParticipant && popoverAnchorRect}
  <UserContextMenu
    user={popoverParticipant.avatarUser}
    anchorRect={popoverAnchorRect}
    canSendMessage={canStartDMs}
    onSendMessage={() => startDMWith(getActiveServer(), popoverParticipant!.avatarUser.id)}
    onClose={closeUserMenu}
  />
{/if}

<style>
  .voice-ring {
    position: relative;
    outline: 0 solid transparent;
    outline-offset: 0;
    transition:
      outline-color 150ms ease-out,
      outline-width 150ms ease-out,
      outline-offset 150ms ease-out;
  }

  .voice-ring-muted {
    outline-color: var(--color-danger);
  }

  .voice-ring-card {
    border-radius: 0.5rem;
  }

  /* Applied imperatively via classList.toggle() in the 60ms audio level loop */
  .voice-ring:global(.voice-ring-speaking) {
    outline-color: color-mix(
      in srgb,
      var(--color-accent) calc(var(--ring-opacity, 0) * 100%),
      var(--color-border)
    );
    outline-width: calc(2px + var(--ring-opacity, 0) * 2.5px);
    outline-offset: calc(1px + var(--ring-opacity, 0) * 2px);
  }
</style>
