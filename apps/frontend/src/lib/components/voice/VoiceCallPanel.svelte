<!--
@component

Room sidebar panel for voice/video calls.

**Two modes:**
- **Observer mode**: Call is active but user hasn't joined. Shows participants
  from server state and a Join button.
- **Participant mode**: User is connected to LiveKit. Shows live audio levels,
  mute toggle, camera/screen-share controls, preferences shortcut, and hang-up button.

**Props:**
- `roomId` - The room ID
- `livekitUrl` - The LiveKit server WebSocket URL (needed for joining)
-->
<script lang="ts">
  import UserCard from '$lib/ui/UserCard.svelte';
  import WipeReveal from '$lib/ui/WipeReveal.svelte';
  import CompactActionButton from '$lib/ui/CompactActionButton.svelte';
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
  import MicrophoneSilenceHint from './MicrophoneSilenceHint.svelte';
  import ConnectionQualityHint from './ConnectionQualityHint.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import VoiceCallControlButton from './VoiceCallControlButton.svelte';
  import ScreenShareControlButton from './ScreenShareControlButton.svelte';
  import {
    contextMenuTrigger,
    type ContextMenuTriggerDetails
  } from '$lib/ui/contextMenuTrigger.svelte';
  import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
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
    'participant-card group/media relative flex w-full min-w-0 flex-col overflow-hidden shell-surface text-start text-text';
  const callTileMediaButtonClass =
    'flex w-full flex-1 cursor-pointer flex-col overflow-hidden rounded-sm text-left text-text outline-none focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-neutral-action';

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

  // User context menu popover
  let popoverParticipant = $state<DisplayParticipant | null>(null);
  let popoverScreen = $state(false);
  let popoverAnchorRect = $state<{ top: number; bottom: number; left: number } | null>(null);
  let popoverPosition = $state<{ x: number; y: number } | undefined>();
  let popoverPresentation = $state<'auto' | 'sheet'>('auto');

  function showUserMenu(participant: DisplayParticipant, e: MouseEvent, screen = false) {
    const button = e.currentTarget as HTMLElement;
    const rect = button?.getBoundingClientRect();
    if (!rect) return;
    popoverParticipant = participant;
    popoverScreen = screen;
    popoverPosition = undefined;
    popoverPresentation = 'auto';
    popoverAnchorRect = { top: rect.top, bottom: rect.bottom, left: rect.left };
  }

  function showParticipantContextMenu(
    participant: DisplayParticipant,
    details: ContextMenuTriggerDetails,
    screen = false
  ) {
    popoverParticipant = participant;
    popoverScreen = screen;
    popoverAnchorRect = null;
    popoverPosition = details.position;
    popoverPresentation = details.presentation;
  }

  function closeUserMenu() {
    popoverParticipant = null;
    popoverAnchorRect = null;
    popoverPosition = undefined;
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
</script>

{#snippet localMuteButton(participant: DisplayParticipant)}
  {@const isMutedForViewer = participant.isLocal
    ? voiceCallState.isMuted
    : participant.isLocallyMuted}
  {#if !participant.isLocal || !isMutedForViewer || voiceCallState.canUseVoice}
    <CompactActionButton
      class={isMutedForViewer ? 'bg-surface-emphasized text-text' : undefined}
      label={participant.isLocal
        ? isMutedForViewer
          ? m('voice.unmute')
          : m('voice.mute')
        : isMutedForViewer
          ? m('voice.locally_unmute_participant')
          : m('voice.locally_mute_participant')}
      data-testid="call-feed-local-mute-button"
      onclick={(event) => toggleFeedMute(participant, event)}
    >
      <span
        class={['iconify', isMutedForViewer ? 'icon-[uil--volume-mute]' : 'icon-[uil--volume-up]']}
        aria-hidden="true"
      ></span>
    </CompactActionButton>
  {/if}
{/snippet}

{#snippet mediaTileActions()}
  <CompactActionButton
    label={m('voice.fullscreen_feed')}
    data-testid="call-feed-fullscreen-button"
    onclick={toggleClosestMediaFullscreen}
  >
    <span class="iconify icon-[mdi--fullscreen]" aria-hidden="true"></span>
  </CompactActionButton>
{/snippet}

{#snippet participantIndicators(participant: DisplayParticipant)}
  {#if participant.isMuted}
    <span class="inline-flex h-5 min-w-5 shrink-0 items-center justify-end gap-1.5 text-sm">
      <span
        class="iconify icon-[uil--microphone-slash] text-danger"
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
  screen = false
)}
  <UserCard
    name={label}
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
      label: m('room.sidebar.view_profile', { name: participant.displayName }),
      onclick: (event) => showUserMenu(participant, event, screen),
      expanded: popoverParticipant?.key === participant.key && popoverScreen === screen,
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
      {#if isInThisCall && participant.isLocal && !screen}
        <MicrophoneSilenceHint />
      {/if}
      {#if headerActions === 'media'}
        {@render mediaTileActions()}
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
    title={participant.displayName}
    data-testid="call-participant-card"
    {@attach contextMenuTrigger((details) => showParticipantContextMenu(participant, details))}
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
    title={m('voice.screen_title', { name: participant.displayName })}
    data-testid="call-screen-share-card"
    {@attach contextMenuTrigger((details) =>
      showParticipantContextMenu(participant, details, true)
    )}
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

{#snippet featuredStageCard(tile: StageTile)}
  {@const participant = tile.participant}
  {@const isScreen = tile.kind === 'screen'}
  {@const isVideo = tile.kind === 'video'}
  <div
    class={[callTileCardClass, 'participant-card-video h-full min-h-0']}
    title={isScreen
      ? m('voice.screen_title', { name: participant.displayName })
      : participant.displayName}
    data-testid="call-featured-stage-card"
    {@attach contextMenuTrigger((details) =>
      showParticipantContextMenu(participant, details, isScreen)
    )}
    data-call-media-card={isScreen || isVideo ? true : undefined}
  >
    {@render participantHeader(
      participant,
      isScreen
        ? m('voice.screen_title', { name: participant.displayName })
        : participant.displayName,
      isScreen || isVideo ? 'media' : 'none',
      !isScreen,
      isScreen
    )}
    <button
      type="button"
      class={[
        callTileMediaButtonClass,
        'min-h-0 items-center justify-center',
        !isScreen && !isVideo && 'p-6'
      ]}
      onclick={(e) => showUserMenu(participant, e, isScreen)}
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
              label={voiceCallState.isMuted ? m('voice.unmute') : m('voice.mute')}
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
            <span class="icon-[uil--phone] text-lg" aria-hidden="true"></span>
            {joinLabel}
          </button>
        </div>
      {/if}
    {/snippet}
  </WipeReveal>
{/snippet}

<div
  class="flex min-h-0 flex-1 flex-col"
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

{#if popoverParticipant && (popoverAnchorRect || popoverPosition)}
  <UserContextMenu
    audioSource={popoverScreen ? 'streamVolume' : 'voiceVolume'}
    user={popoverParticipant.avatarUser}
    anchorRect={popoverAnchorRect}
    position={popoverPosition}
    presentation={popoverPresentation}
    canSendMessage={canStartDMs}
    viewerSettings={serverScope.store.currentUser.user?.settings}
    onSendMessage={() => startDMWith(activeServerId, popoverParticipant!.avatarUser.id)}
    {onOpenProfile}
    onClose={closeUserMenu}
  />
{/if}
