<!--
@component

Room sidebar panel for voice/video calls.

**Two modes:**
- **Observer mode**: Call is active but user hasn't joined. Shows participants
  from server state and a Join button.
- **Participant mode**: User is connected to LiveKit. Shows live audio levels,
  mute toggle, camera/screen-share controls, preferences shortcut, and hang-up button.

**Layouts:**
- **Sidebar**: A featured source above a separate scrolling participant list.
  Participant cards use one or two columns for the narrow pane.
- **Stage**: For the maximized or fullscreen pane. One featured source in a
  16:9 card, with equal tiles below it, or beside it on wide screens. The
  viewer can pin a tile to the featured area. Without a pin, a call without
  screen shares or cameras shows an equal grid instead.

**Props:**
- `roomId` - The room ID
- `livekitUrl` - The LiveKit server WebSocket URL (needed for joining)
- `layout` - `sidebar` (default) or `stage`
-->
<script lang="ts">
  import { Button } from '$lib/ui/form';
  import { serverUi } from '$lib/state/server/serverUi';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import {
    UserCard,
    WipeReveal,
    CompactActionButton,
    PillButtonGroup,
    MenuItem,
    MenuSection
  } from '$lib/ui';
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
  import CallCard, { type CallCardControls } from './CallCard.svelte';
  import ConnectionQualityHint from './ConnectionQualityHint.svelte';
  import { tick, untrack } from 'svelte';
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

  /** Options for a participant card header. */
  type HeaderOptions = {
    /** Show the picture-in-picture button for the card's video. */
    media?: boolean;
    /** Show the muted indicator. Defaults to `true`. */
    indicators?: boolean;
    /** The card shows a screen share, so audio levels and menus use the stream. */
    screen?: boolean;
    /** The card is pinned to the stage and shows an unpin button. */
    pinned?: boolean;
  };

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

  /** Viewer-local pin shared by all layouts; never persisted or sent to LiveKit. */
  let pinnedStageTileKey = $state<string | null>(null);
  let pinnedStageTileKind: StageTile['kind'] | null = null;
  let pinnedStageTile = $derived(stageTiles.find((tile) => tile.key === pinnedStageTileKey));

  // Observe share starts, not the absence of a pin. Unpinning must not acquire
  // an existing share again. A stopped and restarted share is a new start.
  let previousScreenKeys = new Set<string>();
  $effect(() => {
    const joined = isInThisCall;
    const tiles = stageTiles;
    const screens = screenShareTiles;
    untrack(() => {
      const pinned = tiles.find((tile) => tile.key === pinnedStageTileKey);
      if (!joined || !pinned || (pinnedStageTileKind === 'video' && pinned.kind !== 'video')) {
        pinnedStageTileKey = null;
        pinnedStageTileKind = null;
      }
      const newScreens = screens.filter((tile) => !previousScreenKeys.has(tile.key));
      if (joined && pinnedStageTileKey === null) {
        const next = newScreens.find((tile) => !tile.participant.isLocal) ?? newScreens[0];
        if (next) {
          pinnedStageTileKey = next.key;
          pinnedStageTileKind = next.kind;
        }
      }
      previousScreenKeys = new Set(joined ? screens.map((tile) => tile.key) : []);
    });
  });
  /**
   * The remote active speaker, with or without a camera, while any camera is
   * on. A voice-only call keeps its grid, where the speaking glow shows who
   * talks, so the stage does not move without video to gain.
   */
  let activeSpeakerStageTile = $derived(
    isInThisCall && participantTiles.some((tile) => tile.kind === 'video')
      ? participantTiles.find(
          (tile) =>
            !tile.participant.isLocal &&
            tile.participant.key === voiceCallState.activeSpeakerIdentity
        )
      : undefined
  );
  /**
   * Without a pin, follow the active speaker, then fall back to a camera.
   * Existing screen shares do not regain a pin after the viewer unpins them.
   */
  let automaticStageTile = $derived(
    activeSpeakerStageTile ??
      participantTiles.find((tile) => tile.kind === 'video' && !tile.participant.isLocal) ??
      participantTiles.find((tile) => tile.kind === 'video')
  );
  /** Without a pin or camera, the stage keeps every source in an equal grid. */
  let featuredStageTile = $derived(pinnedStageTile ?? automaticStageTile);
  let secondaryStageTiles = $derived(
    featuredStageTile ? stageTiles.filter((tile) => tile.key !== featuredStageTile.key) : []
  );

  /** Near-square fallback grid, including screen shares the viewer unpinned. */
  let stageGridColumns = $derived(Math.max(1, Math.ceil(Math.sqrt(stageTiles.length))));

  /**
   * Largest card whose 16:9 media area fits the featured stage container. The
   * 3rem term is the card header height. This keeps wide and tall screens from
   * cropping camera feeds or adding wide black bars to screen shares. The
   * 12rem floor keeps the header readable on a very short stage.
   */
  const featuredStageCardWidth = 'max(12rem, min(100cqw, calc((100cqh - 3rem) * 16 / 9)))';

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

  let panelElement = $state<HTMLElement | null>(null);

  // Pinning and unpinning remove the focused button, so move focus to keep
  // keyboard users in place: to the control that reverses the action, or to
  // the formerly pinned source's menu button when it has no pin button, for
  // example when it stays featured or the stage switches to the grid.
  async function pinStageTile(tile: StageTile, event: MouseEvent): Promise<void> {
    event.stopPropagation();
    userMenu.close();
    pinnedStageTileKey = tile.key;
    pinnedStageTileKind = tile.kind;
    await tick();
    (
      panelElement?.querySelector<HTMLElement>('[data-testid="call-stage-unpin-button"]') ??
      panelElement?.querySelector<HTMLElement>(
        '[data-testid="call-featured-stage-card"] [data-testid="call-participant-menu-button"]'
      )
    )?.focus();
  }

  async function unpinStageTile(event: MouseEvent): Promise<void> {
    event.stopPropagation();
    const key = pinnedStageTileKey;
    userMenu.close();
    pinnedStageTileKey = null;
    pinnedStageTileKind = null;
    await tick();
    if (key === null || !panelElement) return;
    const tile = `[data-stage-tile-key="${CSS.escape(key)}"]`;
    (
      panelElement.querySelector<HTMLElement>(`${tile} [data-testid="call-stage-pin-button"]`) ??
      panelElement.querySelector<HTMLElement>(
        `${tile} [data-testid="call-participant-menu-button"]`
      )
    )?.focus();
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

  const userMenu = new UserMenuState<{
    participant: DisplayParticipant;
    screen: boolean;
    controls: CallCardControls;
  }>(undefined, () => {
    void userMenu.target?.controls.actions.restoreFocus();
  });
  const menuParticipant = $derived(
    participants.find((participant) => participant.key === userMenu.target?.participant.key)
  );

  /** Choose the next available call control after a card disappears. */
  function cardFocusFallback(): HTMLElement | null | undefined {
    return panelElement?.querySelector<HTMLElement>(
      '[data-testid="call-participant-menu-button"], [data-testid="call-leave-button"], [data-testid="call-join-button"]'
    );
  }

  function cardMenuTarget(identity: string, screen: boolean) {
    const participant = participants.find((participant) => participant.key === identity);
    return participant ? { participant, screen } : null;
  }

  function showUserMenu(
    participant: DisplayParticipant,
    controls: CallCardControls,
    e: MouseEvent,
    screen = false
  ) {
    userMenu.open({ participant, screen, controls }, e);
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

{#snippet mediaTileActions(controls: CallCardControls)}
  <CallPictureInPictureButton {controls} />
{/snippet}

{#snippet participantIndicators(
  participant: DisplayParticipant,
  compact: boolean,
  microphone: boolean
)}
  {@const isMuted = participant.isLocal ? voiceCallState.isMuted : participant.isMuted}
  {#if microphone && isMuted && (compact || !(participant.isLocal && isInThisCall && canShowMuteButton(participant)))}
    <span class="inline-flex h-5 min-w-5 shrink-0 items-center justify-end gap-1.5 text-sm">
      <span
        class="iconify icon-[uil--microphone-slash] text-danger"
        role="img"
        aria-label={m('voice.muted')}
        data-testid="call-muted-indicator"
      ></span>
    </span>
  {/if}
  {#if compact && !participant.isLocal && participant.isLocallyMuted}
    <span
      class="iconify icon-[uil--volume-mute] shrink-0"
      role="img"
      aria-label={m('voice.locally_muted')}
      data-testid="call-locally-muted-indicator"
    ></span>
  {/if}
{/snippet}

{#snippet participantHeader(
  participant: DisplayParticipant,
  label: string,
  controls: CallCardControls,
  {
    media = false,
    indicators: showIndicators = true,
    screen = false,
    pinned = false
  }: HeaderOptions
)}
  {@const compact = controls.actions.compact}
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
    identityAttributes={{ onclick: (e) => showUserMenu(participant, controls, e, screen) }}
    menu={{
      label: m('voice.participant_actions', {
        name: formatAccountName(participant.displayName, participant.avatarUser)
      }),
      onclick: (event) => showUserMenu(participant, controls, event, screen),
      expanded:
        userMenu.target?.participant.key === participant.key && userMenu.target.screen === screen,
      testId: 'call-participant-menu-button',
      attachment: controls.actions.trigger
    }}
  >
    {#snippet avatar()}
      <UserAvatar user={participant.avatarUser} size="sm" />
    {/snippet}
    {#snippet indicators()}
      {@render participantIndicators(
        participant,
        compact,
        showIndicators || (compact && participant.isLocal && isInThisCall)
      )}
    {/snippet}
    {#snippet actions()}
      {#if isInThisCall && !screen}
        <ConnectionQualityHint quality={participant.connectionQuality} />
      {/if}
      <div class="contents" {@attach controls.actions.inline}>
        {#if pinned && !compact}
          <CompactActionButton
            label={m('voice.unpin_from_stage')}
            data-testid="call-stage-unpin-button"
            onclick={unpinStageTile}
          >
            <span class="iconify icon-[mdi--pin-off-outline]" aria-hidden="true"></span>
          </CompactActionButton>
        {/if}
        {#if media && controls}
          {@render mediaTileActions(controls)}
        {/if}
        {#if isInThisCall && !compact}
          {@render localMuteButton(participant)}
        {/if}
      </div>
    {/snippet}
  </UserCard>
{/snippet}

{#snippet participantCard(participant: DisplayParticipant, mode: 'compact' | 'video')}
  {@const showVideo = mode === 'video' && hasVideo(participant)}
  <CallCard
    menu={userMenu}
    target={() => cardMenuTarget(participant.key, false)}
    focusFallback={cardFocusFallback}
    focusScope={() => panelElement}
    class={[
      callTileCardClass,
      mode === 'video' ? 'participant-card-video' : 'participant-card-compact'
    ]}
    title={formatAccountName(participant.displayName, participant.avatarUser)}
    data-testid="call-participant-card"
    data-stage-tile-key={`${participant.key}:person`}
    data-call-tile
    data-call-media-card={showVideo ? true : undefined}
  >
    {#snippet children(controls)}
      {@render participantHeader(participant, participant.displayName, controls, {
        media: isInThisCall && !!showVideo,
        indicators: isInThisCall
      })}

      {#if showVideo}
        <button
          type="button"
          class={callTileMediaButtonClass}
          aria-label={m('voice.pin_to_stage', { name: participant.displayName })}
          data-testid="call-stage-pin-button"
          onclick={(e) =>
            pinStageTile({ key: `${participant.key}:person`, kind: 'video', participant }, e)}
        >
          <VideoThumbnail
            track={participant.videoTrack!}
            videoAttachment={controls.media.observeVideo}
            name={participant.displayName}
            user={participant.avatarUser}
            showIdentityOverlay={false}
          />
        </button>
      {/if}
    {/snippet}
  </CallCard>
{/snippet}

{#snippet screenShareCard(participant: DisplayParticipant)}
  <CallCard
    menu={userMenu}
    target={() => cardMenuTarget(participant.key, true)}
    focusFallback={cardFocusFallback}
    focusScope={() => panelElement}
    class={[callTileCardClass, 'participant-card-video col-span-full']}
    title={m('voice.screen_title', {
      name: formatAccountName(participant.displayName, participant.avatarUser)
    })}
    data-testid="call-screen-share-card"
    data-stage-tile-key={`${participant.key}:screen`}
    data-call-tile
    data-call-media-card
  >
    {#snippet children(controls)}
      {@render participantHeader(
        participant,
        m('voice.screen_title', { name: participant.displayName }),
        controls,
        { media: true, indicators: false, screen: true }
      )}
      <button
        type="button"
        class={callTileMediaButtonClass}
        aria-label={m('voice.pin_to_stage', {
          name: m('voice.screen_title', { name: participant.displayName })
        })}
        data-testid="call-stage-pin-button"
        onclick={(e) =>
          pinStageTile({ key: `${participant.key}:screen`, kind: 'screen', participant }, e)}
      >
        <VideoThumbnail
          track={participant.screenShareTrack!}
          videoAttachment={controls.media.observeVideo}
          name={m('voice.screen_title', { name: participant.displayName })}
          user={participant.avatarUser}
          showIdentityOverlay={false}
          fit="contain"
        />
      </button>
    {/snippet}
  </CallCard>
{/snippet}

{#snippet stageMedia(tile: StageTile, fill: boolean, controls?: CallCardControls)}
  {@const participant = tile.participant}
  {#if tile.kind === 'screen'}
    <VideoThumbnail
      track={participant.screenShareTrack!}
      videoAttachment={controls?.media.observeVideo}
      name={stageTileLabel(tile)}
      user={participant.avatarUser}
      showIdentityOverlay={false}
      fit="contain"
      {fill}
    />
  {:else if tile.kind === 'video'}
    <VideoThumbnail
      track={participant.videoTrack!}
      videoAttachment={controls?.media.observeVideo}
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
  <CallCard
    menu={userMenu}
    target={() => cardMenuTarget(participant.key, isScreen)}
    focusFallback={cardFocusFallback}
    focusScope={() => panelElement}
    class={[callTileCardClass, 'participant-card-video min-h-0']}
    style={isStageLayout ? `width: ${featuredStageCardWidth}` : undefined}
    title={stageTileTitle(tile)}
    data-testid="call-featured-stage-card"
    data-call-tile
    data-stage-tile-key={tile.key}
    data-call-media-card={hasMedia ? true : undefined}
  >
    {#snippet children(controls)}
      {@render participantHeader(participant, stageTileLabel(tile), controls, {
        media: hasMedia,
        indicators: !isScreen,
        screen: isScreen,
        pinned: tile.key === pinnedStageTileKey
      })}
      <button
        type="button"
        class={[
          callTileMediaButtonClass,
          'relative aspect-video min-h-0 items-center justify-center',
          !hasMedia && 'p-6'
        ]}
        aria-label={m('voice.pin_to_stage', { name: stageTileLabel(tile) })}
        aria-pressed={tile.key === pinnedStageTileKey}
        data-testid="call-stage-pin-button"
        onclick={(e) =>
          tile.key === pinnedStageTileKey ? unpinStageTile(e) : pinStageTile(tile, e)}
      >
        {#if hasMedia}
          <div class="absolute inset-0">
            {@render stageMedia(tile, true, controls)}
          </div>
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
    {/snippet}
  </CallCard>
{/snippet}

<!--
  Equal-size stage tile for the filmstrip and the voice grid. Filmstrip tiles
  pin their source to the featured area; grid tiles open the user menu.
-->
{#snippet stageTileCard(tile: StageTile, selectAction: 'pin' | 'menu')}
  {@const participant = tile.participant}
  {@const isScreen = tile.kind === 'screen'}
  <CallCard
    menu={userMenu}
    target={() => cardMenuTarget(participant.key, isScreen)}
    focusFallback={cardFocusFallback}
    focusScope={() => panelElement}
    class={[callTileCardClass, 'participant-card-video']}
    title={stageTileTitle(tile)}
    data-testid="call-stage-tile"
    data-call-tile
    data-stage-tile-key={tile.key}
    data-stage-tile-kind={tile.kind}
  >
    {#snippet children(controls)}
      {@render participantHeader(participant, stageTileLabel(tile), controls, {
        indicators: !isScreen,
        screen: isScreen
      })}
      <button
        type="button"
        class={[callTileMediaButtonClass, 'group/pin relative']}
        aria-label={selectAction === 'pin'
          ? m('voice.pin_to_stage', { name: stageTileLabel(tile) })
          : undefined}
        data-testid={selectAction === 'pin' ? 'call-stage-pin-button' : undefined}
        onclick={(e) =>
          selectAction === 'pin'
            ? pinStageTile(tile, e)
            : showUserMenu(participant, controls, e, isScreen)}
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
          {@render stageMedia(tile, false, controls)}
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
    {/snippet}
  </CallCard>
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
  bind:this={panelElement}
  class="flex min-h-0 min-w-0 flex-1 flex-col"
  data-testid={isInThisCall ? 'call-participant-panel' : 'call-observer-panel'}
>
  <div
    class={[
      'flex min-h-0 flex-1 flex-col gap-5',
      isStageLayout ? 'p-4' : 'px-2 py-3',
      'overflow-hidden'
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
                  <!-- Side tiles may shrink, so a vertical scrollbar cannot force a horizontal one. -->
                  <div class={['w-56', stageFilmstripBeside ? 'min-w-0' : 'shrink-0']}>
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
            style:max-width="{stageGridColumns * 24}rem"
          >
            {#each stageTiles as tile (tile.key)}
              <div
                class="min-w-0"
                style:width="calc((100% - {stageGridColumns - 1} * 0.75rem) / {stageGridColumns})"
              >
                {@render stageTileCard(tile, tile.kind === 'voice' ? 'menu' : 'pin')}
              </div>
            {/each}
          </div>
        </section>
      {:else}
        {#if isInThisCall && featuredStageTile}
          <div
            class="flex max-h-[60%] min-h-0 shrink-0 flex-col"
            data-testid="call-sidebar-featured"
          >
            {@render featuredStageCard(featuredStageTile)}
          </div>
        {/if}
        <section
          class="@container min-h-0 flex-1 overflow-y-auto"
          aria-label={m('voice.participants')}
          data-testid="call-sidebar-participants"
        >
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
              {#if hasScreenShare(participant) && featuredStageTile?.key !== `${participant.key}:screen`}
                {@render screenShareCard(participant)}
              {/if}
            {/each}
            {#each sortedParticipants as participant (participant.key)}
              {@render participantCard(
                participant,
                isInThisCall &&
                  hasVideo(participant) &&
                  featuredStageTile?.key !== `${participant.key}:person`
                  ? 'video'
                  : 'compact'
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

{#if userMenu.target && menuParticipant}
  {@const selection = userMenu.target}
  {@const selectedParticipant = menuParticipant}
  <UserMenu
    state={userMenu}
    audioSource={userMenu.target.screen ? 'streamVolume' : 'voiceVolume'}
    user={selectedParticipant.avatarUser}
    canSendMessage={canStartDMs}
    viewerSettings={serverScope.store.currentUser.user?.settings}
    onSendMessage={() => startDMWith(activeServerId, userMenu.target!.participant.avatarUser.id)}
    {onOpenProfile}
  >
    {#snippet extraActions()}
      {#key selection}
        <CallPictureInPictureButton
          controls={selection.controls}
          presentation="menu"
          onToggle={() => {
            if (userMenu.target === selection) userMenu.close();
          }}
        />
      {/key}
      {#if userMenu.target?.controls.actions.compact && isInThisCall && menuParticipant}
        {@const controls = userMenu.target.controls}
        {@const isPinned = controls.actions.element?.dataset.stageTileKey === pinnedStageTileKey}
        {@const isMuted = menuParticipant.isLocal
          ? voiceCallState.isMuted
          : menuParticipant.isLocallyMuted}
        <MenuSection>
          {#if isPinned}
            <MenuItem
              icon="icon-[mdi--pin-off-outline]"
              dataTestid="call-stage-unpin-button"
              onclick={(event) => {
                userMenu.close();
                void unpinStageTile(event);
              }}
            >
              {m('voice.unpin_from_stage')}
            </MenuItem>
          {/if}
          {#if canShowMuteButton(menuParticipant)}
            <MenuItem
              icon={menuParticipant.isLocal
                ? isMuted
                  ? 'icon-[uil--microphone-slash]'
                  : 'icon-[uil--microphone]'
                : isMuted
                  ? 'icon-[uil--volume-mute]'
                  : 'icon-[uil--volume-up]'}
              pressed={isMuted}
              dataTestid="call-feed-local-mute-button"
              onclick={(event) => {
                if (menuParticipant) toggleFeedMute(menuParticipant, event);
              }}
            >
              {menuParticipant.isLocal ? m('voice.mute') : m('voice.locally_mute_participant')}
            </MenuItem>
          {/if}
        </MenuSection>
      {/if}
    {/snippet}
  </UserMenu>
{/if}
