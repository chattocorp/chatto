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
  import WipeReveal from '$lib/ui/WipeReveal.svelte';
  import PillButtonGroup from '$lib/ui/PillButtonGroup.svelte';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);
  const stores = $derived(serverScope.store);
  const voiceCallState = $derived(stores.voiceCall);
  const activeCallRooms = $derived(stores.activeCallRooms);

  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import VideoThumbnail from './VideoThumbnail.svelte';
  import AudioDeviceMenu from './AudioDeviceMenu.svelte';
  import VoiceCallControlButton from './VoiceCallControlButton.svelte';
  import ScreenShareControlButton from './ScreenShareControlButton.svelte';
  import ParticipantCardMenu from './ParticipantCardMenu.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import { getVoiceCallJoinErrorMessage } from '$lib/state/server/voiceCall.svelte';
  import type { Track } from 'livekit-client';
  import type { Attachment } from 'svelte/attachments';
  import { startDMWith } from '$lib/dm/startDM';
  import { toast } from '$lib/ui/toast';

  let {
    roomId,
    livekitUrl,
    layout = 'sidebar',
    onOpenProfile
  }: {
    roomId: string;
    livekitUrl: string;
    layout?: 'sidebar' | 'stage';
    onOpenProfile?: (userId: string) => void;
  } = $props();

  let isInThisCall = $derived(voiceCallState.isInCall(roomId));
  let isInAnotherCall = $derived(voiceCallState.isInAnyCall && !isInThisCall);
  let isConnecting = $derived(voiceCallState.connecting && voiceCallState.roomId === roomId);
  let hasActiveCall = $derived(activeCallRooms.has(roomId));
  let callPermissions = $derived(voiceCallState.permissionsFor(roomId));
  let canEnterCall = $derived(callPermissions.join && (hasActiveCall || callPermissions.start));
  let isStageLayout = $derived(layout === 'stage');
  let deviceMenuAnchor = $state<{ top: number; bottom: number; left: number } | null>(null);

  /** Unified participant shape for rendering (structural data only). */
  type DisplayParticipant = {
    key: string;
    displayName: string;
    avatarUser: {
      id: string;
      login: string;
      displayName: string;
      avatarUrl: string | null;
      isBot: boolean;
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
          isBot: p.isBot ?? false,
          presenceStatus: PresenceStatus.ONLINE
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

    return activeCallRooms.getParticipants(roomId).map((p) => ({
      key: p.userId,
      displayName: p.displayName,
      avatarUser: {
        id: p.userId,
        login: p.login,
        displayName: p.displayName,
        avatarUrl: p.avatarUrl,
        isBot: p.isBot,
        presenceStatus: PresenceStatus.ONLINE
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
    screenShareTiles[0] ??
      participantTiles.find((tile) => tile.kind === 'video') ??
      participantTiles[0]
  );
  let secondaryStageTiles = $derived(
    featuredStageTile ? stageTiles.filter((tile) => tile.key !== featuredStageTile.key) : []
  );
  let isIdle = $derived(!hasActiveCall && !isInThisCall);
  let joinLabel = $derived.by(() => {
    if (isConnecting) return hasActiveCall ? m('voice.joining') : m('voice.starting');
    return hasActiveCall ? m('voice.join_call') : m('voice.start_call');
  });
  const controlButtonClass = 'pill-button';
  const activeControlButtonClass = 'pill-button-success';
  const dangerControlButtonClass = 'pill-button-danger';
  const callTileCardClass =
    'call-speaking-card participant-card group/media relative flex w-full min-w-0 flex-col gap-1 overflow-hidden shell-surface border border-text/10 p-1.5 text-start text-text shadow-[var(--depth-shadow-xs)]';
  const callTileHeaderClass = 'flex min-w-0 shrink-0 items-center gap-2 p-1';
  const callTileIdentityButtonClass =
    'flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-md text-left text-text outline-none transition-colors hover:text-text focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-neutral-action';
  const callTileMediaButtonClass =
    'flex w-full flex-1 cursor-pointer flex-col overflow-hidden rounded-sm text-left text-text outline-none focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-neutral-action';

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
      const { isSpeaking, audioLevel } = voiceCallState.getAudioLevel(identity);
      const opacity = audioLevel > 0.01 ? 0.35 + Math.pow(audioLevel, 0.35) * 0.65 : 0;
      const visible = isSpeaking || opacity > 0;

      node.style.setProperty(
        '--call-speaking-ring-opacity',
        visible ? String(opacity || 0.85) : '0'
      );
      node.style.setProperty('--call-speaking-ring-strength', visible ? String(audioLevel) : '0');
      node.dataset.callSpeaking = visible ? 'true' : 'false';
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
        node.style.removeProperty('--call-speaking-ring-opacity');
        node.style.removeProperty('--call-speaking-ring-strength');
        delete node.dataset.callSpeaking;
      };
    };
  }

  const canStartDMs = $derived(stores.permissions.canStartDMs);

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
      if (!serverScope.isCurrent()) return;
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
  {@const isMutedForViewer = participant.isLocal
    ? voiceCallState.isMuted
    : participant.isLocallyMuted}
  {#if !participant.isLocal || !isMutedForViewer || voiceCallState.canUseVoice}
    <PillButtonGroup compact label={m('room.sidebar.call')} class="w-7 shrink-0">
      <VoiceCallControlButton
        icon={isMutedForViewer ? 'icon-[uil--volume-mute]' : 'icon-[uil--volume-up]'}
        class={isMutedForViewer ? 'pill-button bg-surface-emphasized text-text' : 'pill-button'}
        label={participant.isLocal
          ? isMutedForViewer
            ? m('voice.unmute')
            : m('voice.mute')
          : isMutedForViewer
            ? m('voice.locally_unmute_participant')
            : m('voice.locally_mute_participant')}
        testId="call-feed-local-mute-button"
        onclick={(event) => toggleFeedMute(participant, event)}
      />
    </PillButtonGroup>
  {/if}
{/snippet}

{#snippet mediaTileActions()}
  <PillButtonGroup
    compact
    label={m('voice.fullscreen_feed')}
    class="w-7 shrink-0"
    testId="call-media-actions"
  >
    <VoiceCallControlButton
      class="pill-button"
      icon="icon-[mdi--fullscreen]"
      label={m('voice.fullscreen_feed')}
      testId="call-feed-fullscreen-button"
      onclick={toggleClosestMediaFullscreen}
    />
  </PillButtonGroup>
{/snippet}

{#snippet participantIndicators(participant: DisplayParticipant)}
  <span class="inline-flex h-5 min-w-5 shrink-0 items-center justify-end gap-1.5 text-sm">
    {#if participant.isMuted}
      <span
        class="iconify icon-[uil--microphone-slash] text-danger"
        aria-label={m('voice.muted')}
        data-testid="call-muted-indicator"
      ></span>
    {/if}
    {#if participant.isLocallyMuted}
      <span
        class="iconify icon-[uil--volume-mute] text-muted"
        aria-label={m('voice.locally_muted')}
        data-testid="call-locally-muted-indicator"
      ></span>
    {/if}
    {#if hasConnectionWarning(participant)}
      <span
        class={[
          'iconify icon-[uil--exclamation-triangle]',
          participant.connectionQuality === 'lost' && 'text-danger',
          participant.connectionQuality === 'poor' && 'text-warning'
        ]}
        aria-label={m('voice.poor_connection')}
      ></span>
    {/if}
  </span>
{/snippet}

{#snippet participantHeader(
  participant: DisplayParticipant,
  label: string,
  actions: 'media' | 'none',
  showIndicators = true
)}
  <div class={callTileHeaderClass}>
    <button
      type="button"
      class={callTileIdentityButtonClass}
      onclick={(e) => showUserMenu(participant, e)}
    >
      <UserAvatar user={participant.avatarUser} size="sm" />
      <span class="min-w-0 flex-1 truncate text-sm font-medium">{label}</span>
      {#if showIndicators}
        {@render participantIndicators(participant)}
      {/if}
    </button>

    {#if actions === 'media'}
      {@render mediaTileActions()}
    {/if}
    {#if isInThisCall}
      {@render localMuteButton(participant)}
    {/if}
    {@render participantAudio(participant)}
  </div>
{/snippet}

{#snippet participantAudio(participant: DisplayParticipant)}
  {#if isInThisCall && !participant.isLocal}
    <ParticipantCardMenu
      settings={voiceCallState.getParticipantAudio(participant.key)}
      boostAvailable={voiceCallState.audioBoostAvailable}
      onVolumeChange={(source, value) =>
        voiceCallState.setParticipantVolume(participant.key, source, value)}
    />
  {/if}
{/snippet}

{#snippet participantCard(participant: DisplayParticipant, mode: 'compact' | 'video')}
  {@const showVideo = mode === 'video' && hasVideo(participant)}
  {@const actions = showVideo ? 'media' : 'none'}
  <div
    class={[
      callTileCardClass,
      mode === 'video' ? 'participant-card-video' : 'participant-card-compact'
    ]}
    {@attach isInThisCall && speakingCard(participant.key)}
    title={participantTitle(participant)}
    data-testid="call-participant-card"
    data-speaking-ring={isInThisCall ? true : undefined}
    data-call-media-card={showVideo ? true : undefined}
  >
    {@render participantHeader(
      participant,
      participant.displayName,
      isInThisCall ? actions : 'none',
      isInThisCall
    )}

    {#if showVideo}
      <button
        type="button"
        class={callTileMediaButtonClass}
        onclick={(e) => showUserMenu(participant, e)}
      >
        <VideoThumbnail
          track={participant.videoTrack!}
          name={participant.displayName}
          user={participant.avatarUser}
          showIdentityOverlay={false}
        />
      </button>
    {/if}
  </div>
{/snippet}

{#snippet screenShareCard(participant: DisplayParticipant)}
  <div
    class={[callTileCardClass, 'participant-card-video col-span-full']}
    {@attach isInThisCall && speakingCard(participant.key)}
    title={m('voice.screen_title', { name: participant.displayName })}
    data-testid="call-screen-share-card"
    data-speaking-ring={isInThisCall ? true : undefined}
    data-call-media-card
  >
    {@render participantHeader(
      participant,
      m('voice.screen_title', { name: participant.displayName }),
      'media',
      false
    )}
    <button
      type="button"
      class={callTileMediaButtonClass}
      onclick={(e) => showUserMenu(participant, e)}
    >
      <VideoThumbnail
        track={participant.screenShareTrack!}
        name={m('voice.screen_title', { name: participant.displayName })}
        user={participant.avatarUser}
        showIdentityOverlay={false}
        fit="contain"
      />
    </button>
  </div>
{/snippet}

{#snippet featuredStageCard(tile: StageTile)}
  {@const participant = tile.participant}
  {@const isScreen = tile.kind === 'screen'}
  {@const isVideo = tile.kind === 'video'}
  <div
    class={[callTileCardClass, 'participant-card-video h-full min-h-0']}
    {@attach isInThisCall && speakingCard(participant.key)}
    title={isScreen
      ? m('voice.screen_title', { name: participant.displayName })
      : participantTitle(participant)}
    data-testid="call-featured-stage-card"
    data-speaking-ring={isInThisCall ? true : undefined}
    data-call-media-card={isScreen || isVideo ? true : undefined}
  >
    {@render participantHeader(
      participant,
      isScreen
        ? m('voice.screen_title', { name: participant.displayName })
        : participant.displayName,
      isScreen || isVideo ? 'media' : 'none',
      true
    )}
    <button
      type="button"
      class={[
        callTileMediaButtonClass,
        'min-h-0 items-center justify-center',
        !isScreen && !isVideo && 'p-6'
      ]}
      onclick={(e) => showUserMenu(participant, e)}
    >
      {#if isScreen}
        <VideoThumbnail
          track={participant.screenShareTrack!}
          name={m('voice.screen_title', { name: participant.displayName })}
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
    </button>
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
  {#if isInThisCall && voiceCallState.audioPlaybackBlocked}
    <button
      type="button"
      class="mb-2 btn-secondary w-full"
      onclick={() => voiceCallState.resumeAudio()}
    >
      {m('voice.participant_audio.enable_audio')}
    </button>
  {/if}
  <div class="grid items-end">
    {#if isInThisCall}
      <WipeReveal class={['col-start-1 row-start-1 w-full', isStageLayout && 'mx-auto max-w-2xl']}>
        <PillButtonGroup label={m('room.sidebar.call')}>
          <VoiceCallControlButton
            class={controlButtonClass}
            label={m('voice.devices')}
            testId="call-device-menu-button"
            icon="icon-[uil--setting]"
            iconClass="text-lg"
            onclick={openDeviceMenu}
          />

          <VoiceCallControlButton
            class={voiceCallState.isCameraEnabled ? activeControlButtonClass : controlButtonClass}
            label={voiceCallState.isCameraEnabled
              ? m('voice.turn_off_camera')
              : m('voice.turn_on_camera')}
            testId="call-camera-toggle"
            icon={voiceCallState.isCameraEnabled ? 'icon-[uil--video]' : 'icon-[uil--video-slash]'}
            iconClass="text-lg"
            onclick={() => voiceCallState.toggleCamera()}
            pending={voiceCallState.isCameraPending}
            disabled={!voiceCallState.canUseCamera && !voiceCallState.isCameraEnabled}
          />

          <VoiceCallControlButton
            class={voiceCallState.isMuted ? controlButtonClass : activeControlButtonClass}
            label={voiceCallState.isMuted ? m('voice.unmute') : m('voice.mute')}
            testId="call-mute-toggle"
            icon={voiceCallState.isMuted ? 'icon-[uil--microphone-slash]' : 'icon-[uil--microphone]'}
            iconClass="text-lg"
            onclick={() => voiceCallState.toggleMute()}
            pending={voiceCallState.isMicrophonePending}
            disabled={!voiceCallState.canUseVoice && voiceCallState.isMuted}
          />

          <ScreenShareControlButton
            {voiceCallState}
            class={voiceCallState.isScreenShareEnabled
              ? activeControlButtonClass
              : controlButtonClass}
            testId="call-screen-share-toggle"
            iconClass="text-lg"
          />

          <VoiceCallControlButton
            class={dangerControlButtonClass}
            onclick={() => voiceCallState.leave()}
            label={m('voice.leave')}
            testId="call-leave-button"
            icon="icon-[uil--phone-slash]"
            iconClass="text-lg"
          />
        </PillButtonGroup>
      </WipeReveal>
    {:else}
      <WipeReveal class={['col-start-1 row-start-1 w-full', isStageLayout && 'mx-auto max-w-sm']}>
        <button
          type="button"
          class="btn-action min-h-12 w-full"
          data-testid="call-join-button"
          onclick={handleJoin}
          disabled={!canEnterCall || isInAnotherCall || isConnecting}
          title={!canEnterCall
            ? m('voice.permission_denied')
            : isInAnotherCall
              ? m('voice.already_in_another_call')
              : joinLabel}
        >
          {joinLabel}
        </button>
      </WipeReveal>
    {/if}
  </div>
{/snippet}

<div
  class="flex min-h-0 flex-1 flex-col"
  data-testid={isInThisCall ? 'call-participant-panel' : 'call-observer-panel'}
>
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
          aria-label={m('voice.participants')}
          data-testid="call-stage-layout"
        >
          <div class="flex min-h-0 flex-1" data-testid="call-featured-stage">
            {@render featuredStageCard(featuredStageTile)}
          </div>

          {#if secondaryStageTiles.length > 0}
            <div
              class="flex max-h-[190px] shrink-0 flex-wrap content-start justify-center gap-3 overflow-y-auto"
              data-testid="call-secondary-stage-list"
            >
              {#each secondaryStageTiles as tile (tile.key)}
                <div class="w-[clamp(180px,22vw,240px)] max-w-full min-w-0">
                  {@render stageTile(tile)}
                </div>
              {/each}
            </div>
          {/if}
        </section>
      {:else}
        <section class="@container flex flex-col gap-2" aria-label={m('voice.participants')}>
          <div
            class={[
              'grid grid-cols-1 gap-3',
              isInThisCall &&
                (screenShareParticipants.length > 0 || mediaTileCount > 1) &&
                '@min-[368px]:grid-cols-2'
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

  <div class="shrink-0 p-2" data-testid="call-controls-bar">
    {@render callControls()}
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
    viewerSettings={serverScope.store.currentUser.user?.settings}
    onSendMessage={() => startDMWith(activeServerId, popoverParticipant!.avatarUser.id)}
    {onOpenProfile}
    onClose={closeUserMenu}
  />
{/if}

<style>
  :global(.call-speaking-card) {
    --call-speaking-ring-opacity: 0;
    --call-speaking-ring-strength: 0;
  }

  :global(.call-speaking-card)::after {
    position: absolute;
    inset: 0;
    border: 2px solid var(--color-action);
    border-radius: inherit;
    box-shadow: 0 0 0.75rem color-mix(in srgb, var(--color-action) 30%, transparent);
    content: '';
    opacity: var(--call-speaking-ring-opacity);
    pointer-events: none;
    transition: opacity 80ms linear;
    animation: call-speaking-ring-pulse 1.25s ease-in-out infinite;
  }

  @keyframes call-speaking-ring-pulse {
    0%,
    100% {
      transform: scale(1);
    }

    50% {
      transform: scale(1.012);
    }
  }
</style>
