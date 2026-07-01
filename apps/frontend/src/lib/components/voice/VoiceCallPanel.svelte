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
    livekitUrl,
    layout = 'sidebar'
  }: {
    roomId: string;
    livekitUrl: string;
    layout?: 'sidebar' | 'stage';
  } = $props();

  let isInThisCall = $derived(voiceCallState.isInCall(roomId));
  let isInAnotherCall = $derived(voiceCallState.isInAnyCall && !isInThisCall);
  let isConnecting = $derived(voiceCallState.connecting && voiceCallState.roomId === roomId);
  let hasActiveCall = $derived(activeCallRooms.has(roomId));
  let isStageLayout = $derived(layout === 'stage');
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
    isLocal: boolean;
    isLocallyMuted: boolean;
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
        isLocal: p.isLocal,
        isLocallyMuted: p.isLocallyMuted ?? false,
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
      isLocal: false,
      isLocallyMuted: false,
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
  type StageTile = {
    key: string;
    kind: 'screen' | 'video' | 'voice';
    participant: DisplayParticipant;
  };
  let screenShareTiles = $derived(
    screenShareParticipants.map((participant) => ({
      key: `${participant.key}:screen`,
      kind: 'screen' as const,
      participant
    }))
  );
  let participantTiles = $derived(
    sortedParticipants.map((participant) => ({
      key: `${participant.key}:${hasVideo(participant) ? 'video' : 'voice'}`,
      kind: hasVideo(participant) ? ('video' as const) : ('voice' as const),
      participant
    }))
  );
  let stageTiles = $derived([...screenShareTiles, ...participantTiles]);
  let featuredStageTile = $derived(
    screenShareTiles[0] ?? participantTiles.find((tile) => tile.kind === 'video') ?? participantTiles[0]
  );
  let secondaryStageTiles = $derived(
    featuredStageTile ? stageTiles.filter((tile) => tile.key !== featuredStageTile.key) : []
  );
  let isIdle = $derived(!hasActiveCall && !isInThisCall);
  let joinLabel = $derived.by(() => {
    if (isConnecting) return hasActiveCall ? m['voice.joining']() : m['voice.starting']();
    return hasActiveCall ? m['voice.join_call']() : m['voice.start_call']();
  });
  const controlButtonClass = 'btn-secondary btn-sm h-9 w-full !px-0';
  const activeControlButtonClass = 'btn-success btn-sm h-9 w-full !px-0';
  const dangerControlButtonClass = 'btn-danger btn-sm h-9 w-full !px-0';
  const tileActionToolbarClass =
    'pointer-events-none absolute top-2 right-2 z-10 flex gap-0.5 rounded-md border border-border bg-surface-100/95 p-0.5 opacity-0 transition-opacity group-hover/media:opacity-100 group-focus-within/media:opacity-100';
  const tileActionButtonClass =
    'pointer-events-auto flex h-7 w-7 cursor-pointer items-center justify-center rounded text-muted transition-colors hover:bg-surface-200 hover:text-text focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-primary';

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

  async function toggleFullscreenElement(element: HTMLElement | null): Promise<void> {
    if (!element || typeof document === 'undefined') return;

    try {
      if (document.fullscreenElement === element) {
        await document.exitFullscreen();
      } else {
        await element.requestFullscreen();
      }
    } catch {
      // Browsers can reject fullscreen requests when system policy denies them.
    }
  }

  function toggleClosestMediaFullscreen(event: MouseEvent): void {
    event.stopPropagation();
    const mediaCard = (event.currentTarget as HTMLElement).closest<HTMLElement>(
      '[data-call-media-card]'
    );
    void toggleFullscreenElement(mediaCard);
  }

  function toggleFeedMute(participant: DisplayParticipant, event: MouseEvent): void {
    event.stopPropagation();
    if (participant.isLocal) {
      void voiceCallState.toggleMute();
    } else {
      voiceCallState.toggleParticipantLocalMute(participant.key);
    }
  }
</script>

{#snippet localMuteButton(participant: DisplayParticipant)}
  {@const isMutedForViewer = participant.isLocal ? voiceCallState.isMuted : participant.isLocallyMuted}
  <button
    type="button"
    class={[tileActionButtonClass, isMutedForViewer && 'bg-surface-200 text-text']}
    title={participant.isLocal
      ? isMutedForViewer
        ? m['voice.unmute']()
        : m['voice.mute']()
      : isMutedForViewer
        ? m['voice.locally_unmute_participant']()
        : m['voice.locally_mute_participant']()}
    aria-label={participant.isLocal
      ? isMutedForViewer
        ? m['voice.unmute']()
        : m['voice.mute']()
      : isMutedForViewer
        ? m['voice.locally_unmute_participant']()
        : m['voice.locally_mute_participant']()}
    data-testid="call-feed-local-mute-button"
    onclick={(event) => toggleFeedMute(participant, event)}
  >
    <span
      class={['iconify text-base', isMutedForViewer ? 'uil--volume-mute' : 'uil--volume-up']}
      aria-hidden="true"
    ></span>
  </button>
{/snippet}

{#snippet mediaTileActions(participant: DisplayParticipant)}
  <div
    class={tileActionToolbarClass}
    data-testid="call-media-actions"
  >
    <button
      type="button"
      class={tileActionButtonClass}
      title={m['voice.fullscreen_feed']()}
      aria-label={m['voice.fullscreen_feed']()}
      data-testid="call-feed-fullscreen-button"
      onclick={toggleClosestMediaFullscreen}
    >
      <span class="iconify text-base mdi--fullscreen" aria-hidden="true"></span>
    </button>
    {#if isInThisCall}
      {@render localMuteButton(participant)}
    {/if}
  </div>
{/snippet}

{#snippet voiceTileActions(participant: DisplayParticipant)}
  {#if isInThisCall}
    <div class={tileActionToolbarClass} data-testid="call-voice-actions">
      {@render localMuteButton(participant)}
    </div>
  {/if}
{/snippet}

{#snippet participantCard(participant: DisplayParticipant, mode: 'compact' | 'video')}
  {@const showVideo = mode === 'video' && hasVideo(participant)}
  {@const showVoiceActions = isInThisCall && !showVideo}
  {#if isInThisCall}
    <div
      class={[
        'participant-card group/media relative flex w-full flex-col overflow-hidden rounded-md border border-border bg-surface-100 text-left text-text transition-colors hover:bg-surface-200',
        mode === 'video' ? 'participant-card-video' : 'participant-card-compact'
      ]}
      {@attach speakingCard(participant.key)}
      title={participantTitle(participant)}
      data-testid="call-participant-card"
      data-call-media-card={showVideo ? true : undefined}
    >
      <button
        type="button"
        class="flex w-full flex-1 cursor-pointer flex-col overflow-hidden text-left text-text"
        onclick={(e) => showUserMenu(participant, e)}
      >
        <div class={['flex min-w-0 items-center gap-2 p-2', showVoiceActions && 'pr-12']}>
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
            {#if participant.isLocallyMuted}
              <span
                class="iconify text-muted uil--volume-mute"
                aria-label={m['voice.locally_muted']()}
                data-testid="call-locally-muted-indicator"
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
          <div class="p-1.5 pt-0">
            <VideoThumbnail
              track={participant.videoTrack!}
              name={participant.displayName}
              user={participant.avatarUser}
              showIdentityOverlay={false}
            />
          </div>
        {/if}
      </button>
      {#if showVideo}
        {@render mediaTileActions(participant)}
      {:else if showVoiceActions}
        {@render voiceTileActions(participant)}
      {/if}
    </div>
  {:else}
    <div
      class={[
        'participant-card group/media relative flex w-full flex-col overflow-hidden rounded-md border border-border bg-surface-100 text-left text-text transition-colors hover:bg-surface-200',
        mode === 'video' ? 'participant-card-video' : 'participant-card-compact'
      ]}
      title={participantTitle(participant)}
      data-testid="call-participant-card"
      data-call-media-card={showVideo ? true : undefined}
    >
      <button
        type="button"
        class="flex w-full flex-1 cursor-pointer flex-col overflow-hidden text-left text-text"
        onclick={(e) => showUserMenu(participant, e)}
      >
        <div class={['flex min-w-0 items-center gap-2 p-2', showVoiceActions && 'pr-12']}>
          <UserAvatar user={participant.avatarUser} size="sm" />
          <span class="min-w-0 flex-1 truncate text-sm font-medium">{participant.displayName}</span>
        </div>

        {#if showVideo}
          <div class="p-1.5 pt-0">
            <VideoThumbnail
              track={participant.videoTrack!}
              name={participant.displayName}
              user={participant.avatarUser}
              showIdentityOverlay={false}
            />
          </div>
        {/if}
      </button>
      {#if showVideo}
        {@render mediaTileActions(participant)}
      {:else if showVoiceActions}
        {@render voiceTileActions(participant)}
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet screenShareCard(participant: DisplayParticipant)}
  <div
    class="participant-card participant-card-video group/media relative flex w-full flex-col overflow-hidden rounded-md border border-border bg-surface-100 text-left text-text transition-colors hover:bg-surface-200 @min-[368px]:col-span-2"
    title={m['voice.screen_title']({ name: participant.displayName })}
    data-testid="call-screen-share-card"
    data-call-media-card
  >
    <button
      type="button"
      class="flex w-full flex-1 cursor-pointer flex-col overflow-hidden text-left text-text"
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
      <div class="p-1.5 pt-0">
        <VideoThumbnail
          track={participant.screenShareTrack!}
          name={m['voice.screen_title']({ name: participant.displayName })}
          user={participant.avatarUser}
          showIdentityOverlay={false}
          fit="contain"
        />
      </div>
    </button>
    {@render mediaTileActions(participant)}
  </div>
{/snippet}

{#snippet featuredStageCard(tile: StageTile)}
  {@const participant = tile.participant}
  {@const isScreen = tile.kind === 'screen'}
  {@const isVideo = tile.kind === 'video'}
  <div
    class="group/media relative flex h-full min-h-0 w-full flex-col overflow-hidden rounded-md border border-border bg-surface-100 text-left text-text transition-colors hover:bg-surface-200"
    title={isScreen
      ? m['voice.screen_title']({ name: participant.displayName })
      : participantTitle(participant)}
    data-testid="call-featured-stage-card"
    data-call-media-card={isScreen || isVideo ? true : undefined}
  >
    <button
      type="button"
      class="flex h-full min-h-0 w-full cursor-pointer flex-col overflow-hidden text-left text-text"
      onclick={(e) => showUserMenu(participant, e)}
    >
      <div
        class={[
          'flex min-w-0 items-center gap-2 border-b border-border/70 p-3',
          !isScreen && !isVideo && isInThisCall && 'pr-12'
        ]}
      >
        <UserAvatar user={participant.avatarUser} size="sm" />
        <span class="min-w-0 flex-1 truncate text-sm font-medium">
          {isScreen
            ? m['voice.screen_title']({ name: participant.displayName })
            : participant.displayName}
        </span>
        {#if isScreen}
          <span
            class="iconify shrink-0 text-muted uil--desktop"
            aria-label={m['voice.screen_share']()}
          ></span>
        {:else}
          <span class="inline-flex h-5 min-w-5 shrink-0 items-center justify-end gap-1.5 text-sm">
            {#if participant.isMuted}
              <span
                class="iconify text-danger uil--microphone-slash"
                aria-label={m['voice.muted']()}
                data-testid="call-muted-indicator"
              ></span>
            {/if}
            {#if participant.isLocallyMuted}
              <span
                class="iconify text-muted uil--volume-mute"
                aria-label={m['voice.locally_muted']()}
                data-testid="call-locally-muted-indicator"
              ></span>
            {/if}
            {#if hasConnectionWarning(participant)}
              <span
                class={[
                  'iconify uil--exclamation-triangle',
                  participant.connectionQuality === 'lost' && 'text-danger',
                  participant.connectionQuality === 'poor' && 'text-warning'
                ]}
                aria-label={m['voice.poor_connection']()}
              ></span>
            {/if}
          </span>
        {/if}
      </div>

      <div
        class={[
          'flex min-h-0 flex-1 items-center justify-center',
          isScreen || isVideo ? 'bg-black p-1.5' : 'p-6'
        ]}
      >
        {#if isScreen}
          <VideoThumbnail
            track={participant.screenShareTrack!}
            name={m['voice.screen_title']({ name: participant.displayName })}
            user={participant.avatarUser}
            showIdentityOverlay={false}
            fit="contain"
            fill
          />
        {:else if isVideo}
          <VideoThumbnail
            track={participant.videoTrack!}
            name={participant.displayName}
            user={participant.avatarUser}
            showIdentityOverlay={false}
            fill
          />
        {:else}
          <div class="flex min-w-0 flex-col items-center gap-4">
            <UserAvatar user={participant.avatarUser} size="xl" showPresence={false} />
            <span class="max-w-full truncate text-lg font-semibold">{participant.displayName}</span>
          </div>
        {/if}
      </div>
    </button>
    {#if isScreen || isVideo}
      {@render mediaTileActions(participant)}
    {:else}
      {@render voiceTileActions(participant)}
    {/if}
  </div>
{/snippet}

{#snippet stageTile(tile: StageTile)}
  {#if tile.kind === 'screen'}
    {@render screenShareCard(tile.participant)}
  {:else}
    {@render participantCard(tile.participant, tile.kind === 'video' ? 'video' : 'compact')}
  {/if}
{/snippet}

{#snippet callControls()}
  {#if isInThisCall}
    <div class={isStageLayout ? 'mx-auto max-w-2xl' : ''}>
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
          class={voiceCallState.isScreenShareEnabled ? activeControlButtonClass : controlButtonClass}
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
    </div>
  {:else}
    <div class={isStageLayout ? 'mx-auto max-w-sm' : ''}>
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
    </div>
  {/if}
{/snippet}

<div
  class="flex min-h-0 flex-1 flex-col"
  data-testid={isInThisCall ? 'call-participant-panel' : 'call-observer-panel'}
>
  {#if !isStageLayout}
    <div class="border-b border-border bg-background p-3" data-testid="call-controls-bar">
      {@render callControls()}
    </div>
  {/if}

  <div
    class={[
      'flex min-h-0 flex-1 flex-col gap-5',
      isStageLayout ? 'p-4' : 'p-3',
      isStageLayout ? 'overflow-hidden' : 'overflow-y-auto'
    ]}
  >
    {#if !isIdle}
      {#if isStageLayout && featuredStageTile}
        <section
          class="flex min-h-0 flex-1 flex-col gap-3"
          aria-label={m['voice.participants']()}
          data-testid="call-stage-layout"
        >
          <div class="flex min-h-0 flex-1" data-testid="call-featured-stage">
            {@render featuredStageCard(featuredStageTile)}
          </div>

          {#if secondaryStageTiles.length > 0}
            <div
              class="grid max-h-[190px] shrink-0 grid-cols-[repeat(auto-fill,minmax(180px,240px))] justify-center gap-3 overflow-y-auto"
              data-testid="call-secondary-stage-list"
            >
              {#each secondaryStageTiles as tile (tile.key)}
                {@render stageTile(tile)}
              {/each}
            </div>
          {/if}
        </section>
      {:else}
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
    {/if}
  </div>

  {#if isStageLayout}
    <div class="border-t border-border bg-background p-3" data-testid="call-controls-bar">
      {@render callControls()}
    </div>
  {/if}
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
