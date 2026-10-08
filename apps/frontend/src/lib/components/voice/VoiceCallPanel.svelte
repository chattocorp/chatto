<!--
@component

Room sidebar panel for voice/video calls.

**Two modes:**
- **Observer mode**: Call is active but user hasn't joined. Shows participants
  from server state and a Join button.
- **Participant mode**: User is connected to LiveKit. Shows live audio levels,
  mute toggle, camera/screen-share controls, preferences shortcut, and hang-up button.

**Layouts:**
- **Sidebar**: Participant cards in one or two columns for the narrow pane.
- **Stage**: For the maximized or fullscreen pane. One featured source with a
  row of equal tiles below it. The viewer can pin a tile to the featured area.
  A call without screen shares or cameras shows an equal grid instead.

**Props:**
- `roomId` - The room ID
- `livekitUrl` - The LiveKit server WebSocket URL (needed for joining)
- `layout` - `sidebar` (default) or `stage`
- `onExitFullscreen` - Set while the pane is fullscreen; adds an exit control
-->
<script lang="ts">
  import { Button } from '$lib/ui/form';
  import { serverUi } from '$lib/state/server/serverUi';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import { UserCard, WipeReveal, CompactActionButton, PillButtonGroup } from '$lib/ui';
  import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const activeServerId = serverScope.serverId;
  const stores = serverScope.store;
  const voiceCallState = $derived(serverUi(stores).voiceCall);
  const activeCallRooms = $derived(serverUi(stores).activeCallRooms);

  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import VideoThumbnail from './VideoThumbnail.svelte';
  import CallPictureInPictureButton from './CallPictureInPictureButton.svelte';
  import ConnectionQualityHint from './ConnectionQualityHint.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import VoiceCallControlButton from './VoiceCallControlButton.svelte';
  import ScreenShareControlButton from './ScreenShareControlButton.svelte';
  import UserMenu from '$lib/components/users/UserMenu.svelte';
  import { UserMenuState } from '$lib/components/users/UserMenuState.svelte';
  import {
    getVoiceCallJoinErrorMessage,
    type CallParticipantInfo
  } from '$lib/state/server/voiceCall.svelte';
  import type { Track } from 'livekit-client';
  import { startDMWith } from '$lib/dm/startDM';
  import { toast } from '$lib/ui/toast';

  let {
    roomId,
    livekitUrl,
    layout = 'sidebar',
    onExitFullscreen,
    onOpenProfile
  }: {
    roomId: string;
    livekitUrl: string;
    layout?: 'sidebar' | 'stage';
    /**
     * Set while the call pane is in browser fullscreen. The pane header is
     * hidden there, so the call controls show an exit button that calls this.
     */
    onExitFullscreen?: () => void;
    onOpenProfile?: (userId: string) => void;
  } = $props();

  let isInThisCall = $derived(voiceCallState.isInCall(roomId));
  let isInAnotherCall = $derived(voiceCallState.isInAnyCall && !isInThisCall);
  let isConnecting = $derived(voiceCallState.connecting && voiceCallState.roomId === roomId);
  let hasActiveCall = $derived(activeCallRooms.has(roomId));
  let callPermissions = $derived(voiceCallState.permissionsFor(roomId));
  let canEnterCall = $derived(callPermissions.join && (hasActiveCall || callPermissions.start));
  let isStageLayout = $derived(layout === 'stage');

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
    connectionQuality: CallParticipantInfo['connectionQuality'];
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
    /** Stable per participant and source, so a pin survives camera toggles. */
    key: string;
    kind: 'screen' | 'video' | 'voice';
    participant: DisplayParticipant;
  };
  let screenShareTiles: StageTile[] = $derived(
    screenShareParticipants.map((participant) => ({
      key: `${participant.key}:screen`,
      kind: 'screen' as const,
      participant
    }))
  );
  let participantTiles: StageTile[] = $derived(
    sortedParticipants.map((participant) => ({
      key: `${participant.key}:person`,
      kind: hasVideo(participant) ? ('video' as const) : ('voice' as const),
      participant
    }))
  );
  let stageTiles = $derived([...screenShareTiles, ...participantTiles]);

  /**
   * Stage tile that the viewer pinned to the featured area. Session-only and
   * local to this viewer. A pin that no longer matches a tile is ignored, so
   * the stage falls back to automatic selection when that source ends.
   */
  let pinnedStageTileKey = $state<string | null>(null);
  let pinnedStageTile = $derived(stageTiles.find((tile) => tile.key === pinnedStageTileKey));
  /** Remote media before the viewer's own media, and screens before cameras. */
  let automaticStageTile = $derived(
    screenShareTiles.find((tile) => !tile.participant.isLocal) ??
      screenShareTiles[0] ??
      participantTiles.find((tile) => tile.kind === 'video' && !tile.participant.isLocal) ??
      participantTiles.find((tile) => tile.kind === 'video')
  );
  /** Without a pin or any media, the stage shows an equal grid instead. */
  let featuredStageTile = $derived(pinnedStageTile ?? automaticStageTile);
  let secondaryStageTiles = $derived(
    featuredStageTile ? stageTiles.filter((tile) => tile.key !== featuredStageTile.key) : []
  );

  /** Near-square voice grid: 2 columns for 2-4 participants, 3 for 5-9. */
  let voiceGridColumns = $derived(Math.max(1, Math.ceil(Math.sqrt(participantTiles.length))));

  /**
   * Largest card whose 16:9 media area fits the featured stage container. The
   * 3rem term is the card header height. This keeps wide and tall screens from
   * cropping camera feeds or adding wide black bars to screen shares.
   */
  const featuredStageCardWidth = 'min(100cqw, calc((100cqh - 3rem) * 16 / 9))';

  // Pixel sizes that match the stage classes: `w-56` filmstrip tiles, a 3rem
  // card header, and `gap-3`. They only choose the filmstrip placement.
  const STAGE_TILE_WIDTH = 224;
  const STAGE_CARD_HEADER = 48;
  const STAGE_TILE_HEIGHT = STAGE_CARD_HEADER + (STAGE_TILE_WIDTH * 9) / 16;
  const STAGE_GAP = 12;

  let stageWidth = $state(0);
  let stageHeight = $state(0);

  /** Near-square side grid, so four tiles show as 2×2 rather than 3 + 1. */
  let sideTileGridMaxWidth = $derived.by(() => {
    const columns = Math.ceil(Math.sqrt(secondaryStageTiles.length));
    return columns * STAGE_TILE_WIDTH + (columns - 1) * STAGE_GAP;
  });

  /** Width of the largest featured card with a 16:9 media area in a box. */
  function featuredCardWidthIn(width: number, height: number): number {
    return Math.max(0, Math.min(width, ((height - STAGE_CARD_HEADER) * 16) / 9));
  }

  /** Featured card width when at least one tile column stands beside it. */
  let featuredWidthBeside = $derived(
    featuredCardWidthIn(stageWidth - STAGE_TILE_WIDTH - STAGE_GAP, stageHeight)
  );

  /**
   * Put the other tiles beside the featured card when that leaves the featured
   * card more room than a row below it, as on ultrawide screens. The featured
   * card then sits at the start and the tiles fill the remaining width.
   */
  let stageFilmstripBeside = $derived(
    secondaryStageTiles.length > 0 &&
      featuredWidthBeside >
        featuredCardWidthIn(stageWidth, stageHeight - STAGE_TILE_HEIGHT - STAGE_GAP)
  );

  function stageTileLabel(tile: StageTile): string {
    return tile.kind === 'screen'
      ? m('voice.screen_title', { name: tile.participant.displayName })
      : tile.participant.displayName;
  }

  function stageTileTitle(tile: StageTile): string {
    const name = formatAccountName(tile.participant.displayName, tile.participant.avatarUser);
    return tile.kind === 'screen' ? m('voice.screen_title', { name }) : name;
  }

  function pinStageTile(tile: StageTile, event: MouseEvent): void {
    event.stopPropagation();
    pinnedStageTileKey = tile.key;
  }

  function unpinStageTile(event: MouseEvent): void {
    event.stopPropagation();
    pinnedStageTileKey = null;
  }
  let isIdle = $derived(!hasActiveCall && !isInThisCall);
  let joinLabel = $derived.by(() => {
    if (isConnecting) return hasActiveCall ? m('voice.joining') : m('voice.starting');
    return hasActiveCall ? m('voice.join_call') : m('voice.start_call');
  });
  const controlButtonClass = 'pill-button';
  const activeControlButtonClass = 'pill-button-success';
  const dangerControlButtonClass = 'pill-button-danger';
  const callTileCardClass =
    'participant-card group/media relative flex w-full min-w-0 flex-col overflow-hidden shell-surface text-start text-text';
  const callTileMediaButtonClass =
    'flex w-full flex-1 cursor-pointer flex-col overflow-hidden rounded-sm text-start text-text outline-none focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-neutral-action';

  function hasVideo(participant: DisplayParticipant) {
    return participant.isCameraEnabled && participant.videoTrack;
  }

  function hasScreenShare(participant: DisplayParticipant) {
    return participant.isScreenShareEnabled && participant.screenShareTrack;
  }

  function participantVoiceLevel(participant: DisplayParticipant) {
    if (participant.isMuted || (participant.isLocal && voiceCallState.isMuted)) return 0;
    const { audioLevel, isSpeaking } = voiceCallState.getAudioLevel(participant.key);
    return audioLevel > 0 ? audioLevel : isSpeaking ? 0.06 : 0;
  }

  const canStartDMs = $derived(stores.permissions.canStartDMs);

  const userMenu = new UserMenuState<{ participant: DisplayParticipant; screen: boolean }>();

  function showUserMenu(participant: DisplayParticipant, e: MouseEvent, screen = false) {
    userMenu.open({ participant, screen }, e);
  }

  function openVoicePreferences() {
    void goto(
      resolve('/chat/[serverId]/settings/voice', { serverId: serverIdToSegment(activeServerId) })
    );
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

  function canShowMuteButton(participant: DisplayParticipant): boolean {
    return !participant.isLocal || !voiceCallState.isMuted || voiceCallState.canUseVoice;
  }
</script>

{#snippet localMuteButton(participant: DisplayParticipant)}
  {@const isMutedForViewer = participant.isLocal
    ? voiceCallState.isMuted
    : participant.isLocallyMuted}
  {#if canShowMuteButton(participant)}
    <CompactActionButton
      aria-pressed={isMutedForViewer}
      label={participant.isLocal ? m('voice.mute') : m('voice.locally_mute_participant')}
      data-testid="call-feed-local-mute-button"
      onclick={(event) => toggleFeedMute(participant, event)}
    >
      <span
        class={[
          'iconify',
          participant.isLocal
            ? isMutedForViewer
              ? 'icon-[uil--microphone-slash] text-danger'
              : 'icon-[uil--microphone]'
            : isMutedForViewer
              ? 'icon-[uil--volume-mute]'
              : 'icon-[uil--volume-up]'
        ]}
        aria-hidden="true"
      ></span>
    </CompactActionButton>
  {/if}
{/snippet}

{#snippet mediaTileActions(track: Track | null)}
  {#key track}
    <CallPictureInPictureButton />
  {/key}
  <CompactActionButton
    label={m('voice.fullscreen_feed')}
    data-testid="call-feed-fullscreen-button"
    onclick={toggleClosestMediaFullscreen}
  >
    <span class="iconify icon-[mdi--monitor-share]" aria-hidden="true"></span>
  </CompactActionButton>
{/snippet}

{#snippet participantIndicators(participant: DisplayParticipant)}
  {@const isMuted = participant.isLocal ? voiceCallState.isMuted : participant.isMuted}
  {#if isMuted && !(participant.isLocal && isInThisCall && canShowMuteButton(participant))}
    <span class="inline-flex h-5 min-w-5 shrink-0 items-center justify-end gap-1.5 text-sm">
      <span
        class="iconify icon-[uil--microphone-slash] text-danger"
        role="img"
        aria-label={m('voice.muted')}
        data-testid="call-muted-indicator"
      ></span>
    </span>
  {/if}
{/snippet}

{#snippet participantHeader(
  participant: DisplayParticipant,
  label: string,
  headerActions: 'media' | 'none',
  showIndicators = true,
  screen = false,
  pinned = false
)}
  <UserCard
    name={label}
    identity={participant.avatarUser}
    username={participant.avatarUser.login}
    voiceLevel={isInThisCall
      ? () =>
          screen
            ? voiceCallState.getScreenShareAudioLevel(participant.key)
            : participantVoiceLevel(participant)
      : undefined}
    class="shrink-0"
    identityAttributes={{ onclick: (e) => showUserMenu(participant, e, screen) }}
    menu={{
      label: m('room.sidebar.view_profile', {
        name: formatAccountName(participant.displayName, participant.avatarUser)
      }),
      onclick: (event) => showUserMenu(participant, event, screen),
      expanded:
        userMenu.target?.participant.key === participant.key && userMenu.target.screen === screen,
      testId: 'call-participant-menu-button'
    }}
  >
    {#snippet avatar()}
      <UserAvatar user={participant.avatarUser} size="sm" />
    {/snippet}
    {#snippet indicators()}
      {#if showIndicators}
        {@render participantIndicators(participant)}
      {/if}
    {/snippet}
    {#snippet actions()}
      {#if isInThisCall && !screen}
        <ConnectionQualityHint quality={participant.connectionQuality} />
      {/if}
      {#if pinned}
        <CompactActionButton
          label={m('voice.unpin_from_stage')}
          aria-pressed="true"
          data-testid="call-stage-unpin-button"
          onclick={unpinStageTile}
        >
          <span class="iconify icon-[mdi--pin-off-outline]" aria-hidden="true"></span>
        </CompactActionButton>
      {/if}
      {#if headerActions === 'media'}
        {@render mediaTileActions(screen ? participant.screenShareTrack : participant.videoTrack)}
      {/if}
      {#if isInThisCall}
        {@render localMuteButton(participant)}
      {/if}
    {/snippet}
  </UserCard>
{/snippet}

{#snippet participantCard(participant: DisplayParticipant, mode: 'compact' | 'video')}
  {@const showVideo = mode === 'video' && hasVideo(participant)}
  {@const actions = showVideo ? 'media' : 'none'}
  <div
    class={[
      callTileCardClass,
      mode === 'video' ? 'participant-card-video' : 'participant-card-compact'
    ]}
    title={formatAccountName(participant.displayName, participant.avatarUser)}
    data-testid="call-participant-card"
    {@attach userMenu.trigger(() => ({ participant, screen: false }))}
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
    title={m('voice.screen_title', {
      name: formatAccountName(participant.displayName, participant.avatarUser)
    })}
    data-testid="call-screen-share-card"
    {@attach userMenu.trigger(() => ({ participant, screen: true }))}
    data-call-media-card
  >
    {@render participantHeader(
      participant,
      m('voice.screen_title', { name: participant.displayName }),
      'media',
      false,
      true
    )}
    <button
      type="button"
      class={callTileMediaButtonClass}
      onclick={(e) => showUserMenu(participant, e, true)}
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

{#snippet stageMedia(tile: StageTile, fill: boolean)}
  {@const participant = tile.participant}
  {#if tile.kind === 'screen'}
    <VideoThumbnail
      track={participant.screenShareTrack!}
      name={stageTileLabel(tile)}
      user={participant.avatarUser}
      showIdentityOverlay={false}
      fit="contain"
      {fill}
    />
  {:else if tile.kind === 'video'}
    <VideoThumbnail
      track={participant.videoTrack!}
      name={participant.displayName}
      user={participant.avatarUser}
      showIdentityOverlay={false}
      {fill}
    />
  {/if}
{/snippet}

{#snippet featuredStageCard(tile: StageTile)}
  {@const participant = tile.participant}
  {@const isScreen = tile.kind === 'screen'}
  {@const hasMedia = tile.kind !== 'voice'}
  <div
    class={[callTileCardClass, 'participant-card-video']}
    style:width={featuredStageCardWidth}
    title={stageTileTitle(tile)}
    data-testid="call-featured-stage-card"
    {@attach userMenu.trigger(() => ({ participant, screen: isScreen }))}
    data-call-media-card={hasMedia ? true : undefined}
  >
    {@render participantHeader(
      participant,
      stageTileLabel(tile),
      hasMedia ? 'media' : 'none',
      !isScreen,
      isScreen,
      tile.key === pinnedStageTileKey
    )}
    <button
      type="button"
      class={[
        callTileMediaButtonClass,
        'aspect-video items-center justify-center',
        !hasMedia && 'p-6'
      ]}
      onclick={(e) => showUserMenu(participant, e, isScreen)}
    >
      {#if hasMedia}
        {@render stageMedia(tile, true)}
      {:else}
        <div class="flex min-w-0 flex-col items-center gap-4">
          <UserAvatar user={participant.avatarUser} size="xl" showPresence={false} />
          <AccountName
            name={participant.displayName}
            identity={participant.avatarUser}
            class="text-lg font-semibold"
          />
        </div>
      {/if}
    </button>
  </div>
{/snippet}

<!--
  Equal-size stage tile for the filmstrip and the voice grid. Filmstrip tiles
  pin their source to the featured area; grid tiles open the user menu.
-->
{#snippet stageTileCard(tile: StageTile, selectAction: 'pin' | 'menu')}
  {@const participant = tile.participant}
  {@const isScreen = tile.kind === 'screen'}
  <div
    class={[callTileCardClass, 'participant-card-video']}
    title={stageTileTitle(tile)}
    data-testid="call-stage-tile"
    data-stage-tile-kind={tile.kind}
    {@attach userMenu.trigger(() => ({ participant, screen: isScreen }))}
  >
    {@render participantHeader(participant, stageTileLabel(tile), 'none', !isScreen, isScreen)}
    <button
      type="button"
      class={[callTileMediaButtonClass, 'group/pin relative']}
      aria-label={selectAction === 'pin'
        ? m('voice.pin_to_stage', { name: stageTileLabel(tile) })
        : undefined}
      data-testid={selectAction === 'pin' ? 'call-stage-pin-button' : undefined}
      onclick={(e) =>
        selectAction === 'pin' ? pinStageTile(tile, e) : showUserMenu(participant, e, isScreen)}
    >
      {#if tile.kind === 'voice'}
        <div class="flex aspect-video w-full items-center justify-center">
          <UserAvatar
            user={participant.avatarUser}
            size={selectAction === 'pin' ? 'lg' : 'xl'}
            showPresence={false}
          />
        </div>
      {:else}
        {@render stageMedia(tile, false)}
      {/if}
      {#if selectAction === 'pin'}
        <span
          class="pointer-events-none absolute inset-0 flex items-center justify-center rounded-md bg-black/40 opacity-0 transition-opacity group-hover/pin:opacity-100 group-focus-visible/pin:opacity-100"
          aria-hidden="true"
        >
          <span class="iconify icon-[mdi--pin-outline] text-2xl text-white" aria-hidden="true"
          ></span>
        </span>
      {/if}
    </button>
  </div>
{/snippet}

{#snippet callControls()}
  {#if isInThisCall && voiceCallState.audioPlaybackBlocked}
    <div class="mb-2">
      <Button variant="secondary" fullWidth onclick={() => voiceCallState.resumeAudio()}>
        {m('voice.participant_audio.enable_audio')}
      </Button>
    </div>
  {/if}
  <WipeReveal active={isInThisCall}>
    {#snippet children(joined)}
      {#if joined}
        <div class={['col-start-1 row-start-1 w-full', isStageLayout && 'mx-auto max-w-2xl']}>
          <PillButtonGroup label={m('room.sidebar.call')}>
            <VoiceCallControlButton
              class={controlButtonClass}
              label={m('voice.preferences.title')}
              testId="call-device-menu-button"
              icon="icon-[uil--setting]"
              iconClass="text-lg"
              onclick={openVoicePreferences}
            />

            <VoiceCallControlButton
              class={voiceCallState.isMuted ? controlButtonClass : activeControlButtonClass}
              label={m('voice.mute')}
              pressed={voiceCallState.isMuted}
              testId="call-mute-toggle"
              icon={voiceCallState.isMuted
                ? 'icon-[uil--microphone-slash]'
                : 'icon-[uil--microphone]'}
              iconClass="text-lg"
              onclick={() => voiceCallState.toggleMute()}
              pending={voiceCallState.isMicrophonePending}
              disabled={!voiceCallState.canUseVoice && voiceCallState.isMuted}
            />

            <VoiceCallControlButton
              class={voiceCallState.isCameraEnabled ? activeControlButtonClass : controlButtonClass}
              label={voiceCallState.isCameraEnabled
                ? m('voice.turn_off_camera')
                : m('voice.turn_on_camera')}
              testId="call-camera-toggle"
              icon={voiceCallState.isCameraEnabled
                ? 'icon-[uil--video]'
                : 'icon-[uil--video-slash]'}
              iconClass="text-lg"
              onclick={() => voiceCallState.toggleCamera()}
              pending={voiceCallState.isCameraPending}
              disabled={!voiceCallState.canUseCamera && !voiceCallState.isCameraEnabled}
            />

            <ScreenShareControlButton
              {voiceCallState}
              class={voiceCallState.isScreenShareEnabled
                ? activeControlButtonClass
                : controlButtonClass}
              testId="call-screen-share-toggle"
              iconClass="text-lg"
            />

            {#if onExitFullscreen}
              <VoiceCallControlButton
                class={controlButtonClass}
                label={m('voice.exit_fullscreen_call')}
                testId="call-exit-fullscreen-button"
                icon="icon-[mdi--fullscreen-exit]"
                iconClass="text-lg"
                onclick={onExitFullscreen}
              />
            {/if}

            <VoiceCallControlButton
              class={dangerControlButtonClass}
              onclick={() => voiceCallState.leave()}
              label={m('voice.leave')}
              testId="call-leave-button"
              icon="icon-[uil--phone-slash]"
              iconClass="text-lg"
            />
          </PillButtonGroup>
        </div>
      {:else}
        <div class={['col-start-1 row-start-1 w-full', isStageLayout && 'mx-auto max-w-sm']}>
          <button
            type="button"
            class="shell-action w-full"
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
        </div>
      {/if}
    {/snippet}
  </WipeReveal>
{/snippet}

<div
  class="flex min-h-0 min-w-0 flex-1 flex-col"
  data-testid={isInThisCall ? 'call-participant-panel' : 'call-observer-panel'}
>
  <div
    class={[
      'flex min-h-0 flex-1 flex-col gap-5',
      isStageLayout ? 'p-4' : 'px-2 py-3',
      isStageLayout ? 'overflow-hidden' : 'overflow-y-auto'
    ]}
  >
    {#if !isIdle}
      {#if isStageLayout && featuredStageTile}
        <section
          class={['flex min-h-0 flex-1 gap-3', stageFilmstripBeside ? 'flex-row' : 'flex-col']}
          aria-label={m('voice.participants')}
          data-testid="call-stage-layout"
          data-filmstrip-placement={stageFilmstripBeside ? 'side' : 'bottom'}
          bind:clientWidth={stageWidth}
          bind:clientHeight={stageHeight}
        >
          <div
            class={[
              '[container-type:size] flex min-h-0 min-w-0 justify-center',
              stageFilmstripBeside ? 'shrink-0 items-start' : 'flex-1 items-center'
            ]}
            style:width={stageFilmstripBeside ? `${featuredWidthBeside}px` : undefined}
            data-testid="call-featured-stage"
          >
            {@render featuredStageCard(featuredStageTile)}
          </div>

          {#if secondaryStageTiles.length > 0}
            <div
              class={[
                'flex',
                stageFilmstripBeside
                  ? 'min-w-0 flex-1 flex-col overflow-y-auto'
                  : 'shrink-0 overflow-x-auto'
              ]}
              data-testid="call-secondary-stage-strip"
            >
              <div
                class={['flex gap-3', stageFilmstripBeside ? 'flex-wrap' : 'mx-auto w-max']}
                style:max-width={stageFilmstripBeside ? `${sideTileGridMaxWidth}px` : undefined}
                data-testid="call-secondary-stage-list"
              >
                {#each secondaryStageTiles as tile (tile.key)}
                  <div class="w-56 shrink-0">
                    {@render stageTileCard(tile, 'pin')}
                  </div>
                {/each}
              </div>
            </div>
          {/if}
        </section>
      {:else if isStageLayout}
        <section
          class="flex min-h-0 flex-1 overflow-y-auto"
          aria-label={m('voice.participants')}
          data-testid="call-stage-grid"
        >
          <div
            class="m-auto flex w-full flex-wrap justify-center gap-3"
            style:max-width="{voiceGridColumns * 24}rem"
          >
            {#each participantTiles as tile (tile.key)}
              <div
                class="min-w-0"
                style:width="calc((100% - {voiceGridColumns - 1} * 0.75rem) / {voiceGridColumns})"
              >
                {@render stageTileCard(tile, 'menu')}
              </div>
            {/each}
          </div>
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

{#if userMenu.target}
  <UserMenu
    state={userMenu}
    audioSource={userMenu.target.screen ? 'streamVolume' : 'voiceVolume'}
    user={userMenu.target.participant.avatarUser}
    canSendMessage={canStartDMs}
    viewerSettings={serverScope.store.currentUser.user?.settings}
    onSendMessage={() => startDMWith(activeServerId, userMenu.target!.participant.avatarUser.id)}
    {onOpenProfile}
  />
{/if}
