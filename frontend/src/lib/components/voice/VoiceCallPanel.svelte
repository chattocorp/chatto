<!--
@component

Compact bar shown below the room header when there is an active voice call.

**Two modes:**
- **Observer mode**: Call is active but user hasn't joined. Shows participants
  from server state and a Join button.
- **Participant mode**: User is connected to LiveKit. Shows live audio levels,
  mute toggle, audio device selector, and hang-up button.

Both modes share the same layout — only the participant data source and action
buttons differ. This prevents layout shift when joining/leaving a call.

**Props:**
- `roomId` - The room ID
- `livekitUrl` - The LiveKit server WebSocket URL (needed for joining)
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';

  const getInstanceId = getActiveServer();
  const stores = serverRegistry.getStore(getInstanceId());
  const voiceCallState = stores.voiceCall;
  const activeCallRooms = stores.activeCallRooms;
  const callParticipantsState = stores.callParticipants;
  import { useServerEvent } from '$lib/hooks';
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
  let visible = $derived(isInThisCall || hasActiveCall);
  let deviceMenuAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);

  // Load server-side participants when there's an active call and we're not in it
  $effect(() => {
    if (!isInThisCall && hasActiveCall) {
      callParticipantsState.load(roomId);
    } else if (!hasActiveCall && !isInThisCall) {
      callParticipantsState.clear();
    }
  });

  // Handle call join/leave events to optimistically update the observer participant list
  useServerEvent((spaceEvent) => {
    const event = spaceEvent.event;
    if (!event) return;

    if (event.__typename === 'CallParticipantJoinedEvent' && event.roomId === roomId) {
      const actor = spaceEvent.actor ? useFragment(UserAvatarFragment, spaceEvent.actor) : null;
      callParticipantsState.handleJoin(event.roomId, actor);
    } else if (event.__typename === 'CallParticipantLeftEvent' && event.roomId === roomId) {
      callParticipantsState.handleLeave(event.roomId, spaceEvent.actorId);
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

  const MAX_VISIBLE_PARTICIPANTS = 6;

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
  let visibleParticipants = $derived(sortedParticipants.slice(0, MAX_VISIBLE_PARTICIPANTS));
  let overflowCount = $derived(sortedParticipants.length - MAX_VISIBLE_PARTICIPANTS);
  let videoParticipants = $derived(visibleParticipants.filter((p) => p.isCameraEnabled && p.videoTrack));
  let voiceParticipants = $derived(visibleParticipants.filter((p) => !(p.isCameraEnabled && p.videoTrack)));

  // --- Imperative audio level ring animation ---
  // Reads from voiceCallState.getAudioLevel() (non-reactive) and directly
  // mutates DOM elements at ~60ms. Completely bypasses Svelte's reactive graph.
  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative DOM ref map, not read reactively
  const buttonRefs = new Map<string, HTMLButtonElement>();

  function trackButton(node: HTMLButtonElement, identity: string) {
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

  // DM permissions
  const instancePerms = getServerPermissions();
  const canWriteDMs = $derived(instancePerms.current.canWriteDMs);

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
      toast.error('Failed to join voice call');
    }
  }
</script>

{#if visible}
  <div
    class="flex min-h-10 flex-row items-center gap-3 bg-surface-100 px-3 py-1.5"
    data-testid={isInThisCall ? undefined : 'call-observer-panel'}
  >
    <!-- Call indicator -->
    <div class="flex flex-row items-center gap-1.5 text-accent">
      <span class="iconify animate-pulse uil--phone"></span>
    </div>

    <!-- Participants -->
    <div class="flex flex-row items-center gap-2">
      {#each videoParticipants as participant (participant.key)}
        <button
          type="button"
          class="voice-ring voice-ring-video cursor-pointer"
          class:voice-ring-muted={participant.isMuted}
          use:trackButton={participant.key}
          title={participant.connectionQuality === 'poor' || participant.connectionQuality === 'lost'
            ? `${participant.displayName} — poor connection`
            : participant.displayName}
          onclick={(e) => showUserMenu(participant, e)}
        >
          <VideoThumbnail track={participant.videoTrack!} name={participant.displayName} user={participant.avatarUser} />
          {#if participant.connectionQuality === 'poor' || participant.connectionQuality === 'lost'}
            <span
              class="connection-warning iconify uil--exclamation-triangle"
              class:text-danger={participant.connectionQuality === 'lost'}
              class:text-warning={participant.connectionQuality === 'poor'}
            ></span>
          {/if}
        </button>
      {/each}

      {#if voiceParticipants.length > 0}
        <div class="voice-only-grid">
          {#each voiceParticipants as participant (participant.key)}
            <button
              type="button"
              class="voice-ring cursor-pointer"
              class:voice-ring-muted={participant.isMuted}
              use:trackButton={participant.key}
              title={participant.connectionQuality === 'poor' || participant.connectionQuality === 'lost'
                ? `${participant.displayName} — poor connection`
                : participant.displayName}
              onclick={(e) => showUserMenu(participant, e)}
            >
              <UserAvatar user={participant.avatarUser} size="xs" showPresence={false} />
              {#if participant.connectionQuality === 'poor' || participant.connectionQuality === 'lost'}
                <span
                  class="connection-warning iconify uil--exclamation-triangle"
                  class:text-danger={participant.connectionQuality === 'lost'}
                  class:text-warning={participant.connectionQuality === 'poor'}
                ></span>
              {/if}
            </button>
          {/each}
        </div>
      {/if}

      {#if overflowCount > 0}
        <span class="text-xs text-muted">+{overflowCount}</span>
      {/if}
    </div>

    <!-- Actions -->
    <div class="ml-auto flex flex-row items-center gap-2">
      {#if isInThisCall}
        <!-- Device selector -->
        <button
          type="button"
          class="iconify cursor-pointer text-muted uil--setting hover:text-text"
          title="Devices"
          data-testid="call-device-menu-button"
          onclick={openDeviceMenu}
        ></button>

        {#if deviceMenuAnchor}
          <AudioDeviceMenu anchor={deviceMenuAnchor} onclose={() => (deviceMenuAnchor = null)} />
        {/if}

        <!-- Camera toggle -->
        <button
          type="button"
          class={[
            'iconify cursor-pointer hover:text-text',
            voiceCallState.isCameraEnabled
              ? 'text-muted uil--video'
              : 'text-danger uil--video-slash'
          ]}
          title={voiceCallState.isCameraEnabled ? 'Turn off camera' : 'Turn on camera'}
          data-testid="call-camera-toggle"
          onclick={() => voiceCallState.toggleCamera()}
        ></button>

        <!-- Mute toggle -->
        <button
          type="button"
          class={[
            'iconify cursor-pointer hover:text-text',
            voiceCallState.isMuted
              ? 'text-danger uil--microphone-slash'
              : 'text-muted uil--microphone'
          ]}
          title={voiceCallState.isMuted ? 'Unmute' : 'Mute'}
          onclick={() => voiceCallState.toggleMute()}
        ></button>

        <!-- Hang up -->
        <button
          type="button"
          class="iconify cursor-pointer text-danger uil--phone-slash hover:brightness-125"
          onclick={() => voiceCallState.leave()}
          title="Leave call"
        ></button>
      {:else}
        <!-- Join button -->
        <button
          type="button"
          class="cursor-pointer rounded bg-accent px-3 py-1 text-xs font-medium text-white hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
          data-testid="call-join-button"
          onclick={handleJoin}
          disabled={isInAnotherCall || isConnecting}
          title={isInAnotherCall ? 'Already in another call' : 'Join voice call'}
        >
          {#if isConnecting}
            Joining...
          {:else}
            Join
          {/if}
        </button>
      {/if}
    </div>
  </div>

  {#if popoverParticipant && popoverAnchorRect}
    <UserContextMenu
      user={popoverParticipant.avatarUser}
      anchorRect={popoverAnchorRect}
      canSendMessage={canWriteDMs}
      onSendMessage={() => startDMWith(getInstanceId(), popoverParticipant!.avatarUser.id)}
      onClose={closeUserMenu}
    />
  {/if}
{/if}

<style>
  .voice-ring {
    position: relative;
    display: inline-flex;
    outline: 2px solid var(--color-border);
    outline-offset: 1px;
    border-radius: 9999px;
    transition:
      outline-color 150ms ease-out,
      outline-width 150ms ease-out,
      outline-offset 150ms ease-out;
  }

  .voice-ring-muted {
    outline-color: var(--color-danger);
  }

  .voice-ring-video {
    border-radius: 0.25rem;
  }

  .voice-only-grid {
    display: flex;
    flex-wrap: wrap;
    align-content: center;
    gap: 0.375rem;
    max-height: 68px;
  }

  .connection-warning {
    position: absolute;
    bottom: -2px;
    right: -2px;
    font-size: 0.625rem;
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
