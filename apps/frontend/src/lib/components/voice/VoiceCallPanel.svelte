<!--
@component

Room sidebar panel for voice/video calls.

**Two modes:**
- **Observer mode**: Call is active but user hasn't joined. Shows participants
  from server state and a Join button.
- **Participant mode**: User is connected to LiveKit. Shows live audio levels,
  mute toggle, camera/screen-share controls, audio device selector, and hang-up button.

**Props:**
- `roomId` - The room ID
- `livekitUrl` - The LiveKit server WebSocket URL (needed for joining)
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import * as m from '$lib/i18n/messages';

  const stores = serverRegistry.getStore(getActiveServer());
  const voiceCallState = stores.voiceCall;
  const activeCallRooms = stores.activeCallRooms;
  const callParticipantsState = stores.callParticipants;
  import { useEvent } from '$lib/hooks';
  import { useRenderData } from '$lib/render/data';
  import { UserAvatarViewData } from '$lib/components/UserAvatar.svelte';
  import type { PresenceStatus } from '$lib/render/types';
  import type { EventEnvelope } from '$lib/eventBus.svelte';
  import { RoomEventKind, roomEventKind } from '$lib/render/eventKinds';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import VideoThumbnail from './VideoThumbnail.svelte';
  import AudioDeviceMenu from './AudioDeviceMenu.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import { getVoiceCallJoinErrorMessage } from '$lib/state/server/voiceCall.svelte';
  import type { Track } from 'livekit-client';
  import type { Attachment } from 'svelte/attachments';
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

  function callEventPayload(
    event: EventEnvelope['event']
  ): { roomId: string; callId: string } | null {
    if (
      !event ||
      !('roomId' in event) ||
      typeof event.roomId !== 'string' ||
      !('callId' in event) ||
      typeof event.callId !== 'string'
    ) {
      return null;
    }
    return { roomId: event.roomId, callId: event.callId };
  }

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

    const call = callEventPayload(event);
    if (!call || call.roomId !== roomId) return;

    switch (roomEventKind(event)) {
      case RoomEventKind.CallParticipantJoined: {
        const actor = spaceEvent.actor ? useRenderData(UserAvatarViewData, spaceEvent.actor) : null;
        void callParticipantsState.handleJoin(call.roomId, call.callId, actor);
        break;
      }
      case RoomEventKind.CallParticipantLeft:
        callParticipantsState.handleLeave(call.roomId, call.callId, spaceEvent.actorId ?? null);
        voiceCallState.handleParticipantLeftEvent(
          call.roomId,
          call.callId,
          spaceEvent.actorId ?? null,
          stores.rooms.currentUserId
        );
        break;
      case RoomEventKind.CallEnded:
        callParticipantsState.handleEnd(call.roomId, call.callId);
        activeCallRooms.handleEnd(call.roomId, call.callId);
        voiceCallState.handleCallEndedEvent(call.roomId, call.callId);
        break;
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
    isScreenShareEnabled: boolean;
    screenShareTrack: Track | null;
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
        videoTrack: p.videoTrack,
        isScreenShareEnabled: p.isScreenShareEnabled,
        screenShareTrack: p.screenShareTrack
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
      videoTrack: null,
      isScreenShareEnabled: false,
      screenShareTrack: null
    }));
  });

  let sortedParticipants = $derived(
    [...participants].sort((a, b) => {
      if (a.isCameraEnabled && a.videoTrack && !(b.isCameraEnabled && b.videoTrack)) return -1;
      if (b.isCameraEnabled && b.videoTrack && !(a.isCameraEnabled && a.videoTrack)) return 1;
      return 0;
    })
  );
  let screenShareParticipants = $derived(
    sortedParticipants.filter((p) => p.isScreenShareEnabled && p.screenShareTrack)
  );
  let videoParticipants = $derived(
    sortedParticipants.filter((p) => p.isCameraEnabled && p.videoTrack)
  );
  let mediaTileCount = $derived(screenShareParticipants.length + videoParticipants.length);
  let isIdle = $derived(!hasActiveCall && !isInThisCall);
  let joinLabel = $derived.by(() => {
    if (isConnecting) return hasActiveCall ? m['voice.joining']() : m['voice.starting']();
    return hasActiveCall ? m['voice.join_call']() : m['voice.start_call']();
  });
  const controlButtonClass = 'btn-secondary btn-sm h-9 w-full !px-0';
  const activeControlButtonClass = 'btn-success btn-sm h-9 w-full !px-0';
  const dangerControlButtonClass = 'btn-danger btn-sm h-9 w-full !px-0';

  function hasVideo(participant: DisplayParticipant) {
    return participant.isCameraEnabled && participant.videoTrack;
  }

  function hasScreenShare(participant: DisplayParticipant) {
    return participant.isScreenShareEnabled && participant.screenShareTrack;
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

  const speakingCards: Array<{ identity: string; node: HTMLElement }> = [];
  let speakingIndicatorInterval: ReturnType<typeof setInterval> | null = null;

  function updateSpeakingIndicators() {
    for (const { identity, node } of speakingCards) {
      const indicator = node.querySelector<HTMLElement>('[data-speaking-indicator]');
      if (!indicator) continue;

      const { isSpeaking, audioLevel } = voiceCallState.getAudioLevel(identity);
      const opacity = audioLevel > 0.01 ? 0.35 + Math.pow(audioLevel, 0.35) * 0.65 : 0;
      const visible = isSpeaking || opacity > 0;

      indicator.style.opacity = visible ? String(opacity || 0.85) : '0';
      indicator.setAttribute('aria-hidden', visible ? 'false' : 'true');
    }
  }

  function startSpeakingIndicatorLoop() {
    if (speakingIndicatorInterval) return;

    speakingIndicatorInterval = setInterval(updateSpeakingIndicators, 60);
  }

  function stopSpeakingIndicatorLoopIfIdle() {
    if (speakingCards.length > 0 || !speakingIndicatorInterval) return;

    clearInterval(speakingIndicatorInterval);
    speakingIndicatorInterval = null;
  }

  function speakingCard(identity: string): Attachment<HTMLElement> {
    return (node) => {
      const entry = { identity, node };
      speakingCards.push(entry);
      updateSpeakingIndicators();
      startSpeakingIndicatorLoop();

      return () => {
        const index = speakingCards.indexOf(entry);
        if (index !== -1) speakingCards.splice(index, 1);
        stopSpeakingIndicatorLoopIfIdle();
      };
    };
  }

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
    } catch (err) {
      stores.handleVoiceCallJoinFailed(roomId);
      toast.error(getVoiceCallJoinErrorMessage(err));
    }
  }
</script>

{#snippet participantCard(participant: DisplayParticipant, mode: 'compact' | 'video')}
  {@const showVideo = mode === 'video' && hasVideo(participant)}
  {#if isInThisCall}
    <button
      type="button"
      class={[
        'participant-card flex w-full cursor-pointer flex-col overflow-hidden rounded-md border border-border bg-surface-100 text-left text-text transition-colors hover:bg-surface-200',
        mode === 'video' ? 'participant-card-video' : 'participant-card-compact'
      ]}
      {@attach speakingCard(participant.key)}
      title={participantTitle(participant)}
      data-testid="call-participant-card"
      onclick={(e) => showUserMenu(participant, e)}
    >
      <div class="flex min-w-0 items-center gap-2 p-2">
        <UserAvatar user={participant.avatarUser} size="sm" />
        <span class="min-w-0 flex-1 truncate text-sm font-medium">{participant.displayName}</span>
        <span class="inline-flex h-5 min-w-5 shrink-0 items-center justify-end gap-1.5 text-sm">
          <span
            class="iconify text-muted opacity-0 transition-opacity uil--volume-up"
            aria-label={m['voice.speaking']()}
            aria-hidden="true"
            data-speaking-indicator
            data-testid="call-speaking-indicator"
          ></span>
          {#if participant.isMuted}
            <span
              class="iconify text-danger uil--microphone-slash"
              aria-label={m['voice.muted']()}
              data-testid="call-muted-indicator"
            ></span>
          {/if}
          {#if hasConnectionWarning(participant)}
            <span
              class="iconify uil--exclamation-triangle"
              class:text-danger={participant.connectionQuality === 'lost'}
              class:text-warning={participant.connectionQuality === 'poor'}
              aria-label={m['voice.poor_connection']()}
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
        <UserAvatar user={participant.avatarUser} size="sm" />
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

{#snippet screenShareCard(participant: DisplayParticipant)}
  <button
    type="button"
    class="participant-card participant-card-video flex w-full cursor-pointer flex-col overflow-hidden rounded-md border border-border bg-surface-100 text-left text-text transition-colors hover:bg-surface-200 @min-[368px]:col-span-2"
    title={m['voice.screen_title']({ name: participant.displayName })}
    data-testid="call-screen-share-card"
    onclick={(e) => showUserMenu(participant, e)}
  >
    <div class="flex min-w-0 items-center gap-2 p-2">
      <UserAvatar user={participant.avatarUser} size="sm" />
      <span class="min-w-0 flex-1 truncate text-sm font-medium">
        {m['voice.screen_title']({ name: participant.displayName })}
      </span>
      <span class="iconify shrink-0 text-muted uil--desktop" aria-label={m['voice.screen_share']()}
      ></span>
    </div>
    <div class="p-2 pt-0">
      <VideoThumbnail
        track={participant.screenShareTrack!}
        name={m['voice.screen_title']({ name: participant.displayName })}
        user={participant.avatarUser}
        showIdentityOverlay={false}
        fit="contain"
      />
    </div>
  </button>
{/snippet}

<div
  class="flex min-h-0 flex-1 flex-col"
  data-testid={isInThisCall ? 'call-participant-panel' : 'call-observer-panel'}
>
  <div class="border-b border-border bg-background p-3">
    {#if isInThisCall}
      <div class="grid grid-cols-5 gap-2">
        <button
          type="button"
          class={controlButtonClass}
          title={m['voice.devices']()}
          aria-label={m['voice.devices']()}
          data-testid="call-device-menu-button"
          onclick={openDeviceMenu}
        >
          <span class="iconify text-lg uil--setting" aria-hidden="true"></span>
        </button>

        <button
          type="button"
          class={voiceCallState.isCameraEnabled ? activeControlButtonClass : controlButtonClass}
          title={voiceCallState.isCameraEnabled
            ? m['voice.turn_off_camera']()
            : m['voice.turn_on_camera']()}
          aria-label={voiceCallState.isCameraEnabled
            ? m['voice.turn_off_camera']()
            : m['voice.turn_on_camera']()}
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
          class={voiceCallState.isMuted ? controlButtonClass : activeControlButtonClass}
          title={voiceCallState.isMuted ? m['voice.unmute']() : m['voice.mute']()}
          aria-label={voiceCallState.isMuted ? m['voice.unmute']() : m['voice.mute']()}
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
          class={voiceCallState.isScreenShareEnabled
            ? activeControlButtonClass
            : controlButtonClass}
          title={voiceCallState.isScreenShareEnabled
            ? m['voice.stop_share_screen']()
            : m['voice.share_screen']()}
          aria-label={voiceCallState.isScreenShareEnabled
            ? m['voice.stop_share_screen']()
            : m['voice.share_screen']()}
          data-testid="call-screen-share-toggle"
          onclick={() => voiceCallState.toggleScreenShare()}
        >
          <span class="iconify text-lg uil--desktop" aria-hidden="true"></span>
        </button>

        <button
          type="button"
          class={dangerControlButtonClass}
          onclick={() => voiceCallState.leave()}
          title={m['voice.leave']()}
          aria-label={m['voice.leave']()}
          data-testid="call-leave-button"
        >
          <span class="iconify text-lg uil--phone-slash" aria-hidden="true"></span>
        </button>
      </div>
    {:else}
      <button
        type="button"
        class="btn-accent w-full btn-sm"
        data-testid="call-join-button"
        onclick={handleJoin}
        disabled={isInAnotherCall || isConnecting}
        title={isInAnotherCall ? m['voice.already_in_another_call']() : joinLabel}
      >
        {joinLabel}
      </button>
    {/if}
  </div>

  <div class="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto p-3">
    {#if !isIdle}
      <section class="@container flex flex-col gap-2" aria-label={m['voice.participants']()}>
        <div
          class={[
            'grid grid-cols-1 gap-3',
            isInThisCall && mediaTileCount > 1 && '@min-[368px]:grid-cols-2'
          ]}
          data-testid="call-participants-list"
        >
          {#each screenShareParticipants as participant (`${participant.key}:screen`)}
            {#if hasScreenShare(participant)}
              {@render screenShareCard(participant)}
            {/if}
          {/each}
          {#each sortedParticipants as participant (participant.key)}
            {@render participantCard(
              participant,
              isInThisCall && hasVideo(participant) ? 'video' : 'compact'
            )}
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
