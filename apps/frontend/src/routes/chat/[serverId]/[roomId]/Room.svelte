<script lang="ts">
  import RoomNoticeBanner from './RoomNoticeBanner.svelte';
  import type { MarkdownMentionOptions } from '$lib/components/composer/mentionOptions';
  import { timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { conversationPath, userConversationRoomId } from '$lib/navigation/conversationPath';
  import DirectMessageName from '$lib/components/users/DirectMessageName.svelte';
  import { serverUi } from '$lib/state/server/serverUi';
  import { onDestroy, untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { MediaQuery } from 'svelte/reactivity';
  import { beforeNavigate, goto, pushState, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { useRoomData } from '$lib/hooks';
  import { m } from '$lib/i18n/messages';
  import { directMessageLabels } from '$lib/render/directMessageLabels';
  import {
    createMentionRoles,
    setRoomMembersStore,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import {
    getAppUiState,
    getRoomSidebarPresentation,
    type RoomSidebarPresentation
  } from '$lib/state/appUi.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { MessageSearchState } from '$lib/state/server/messageSearch';
  import { userPreferences, type ThreadPanePresentation } from '$lib/state/userPreferences.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { clearLastRoom, setLastRoom } from '$lib/storage/lastRoom';
  import type { RoomSidebarPanel } from '$lib/storage/roomSidebarPanel';
  import { PageTitle, PaneHeader, HeaderIconButton } from '$lib/ui';
  import { tick } from 'svelte';
  import ConversationPane from './ConversationPane.svelte';
  import ThreadPane from './ThreadPane.svelte';
  import RoomWindowLifecycle from './RoomWindowLifecycle.svelte';
  import RoomRouteHighlight from './RoomRouteHighlight.svelte';
  import RoomSidebarPane from './RoomSidebarPane.svelte';
  import RoomSidebarToggle from './RoomSidebarToggle.svelte';
  import {
    canBanMembersFromRoomSidebar,
    roomSidebarPanelForRoom,
    roomSidebarPanelsForRoom,
    visibleRoomSidebarPanel
  } from './roomSidebarBehavior';
  import { RoomNavigationState } from './roomNavigationState.svelte';
  import { RoomResources } from './roomResources';
  import { buildRoomPresentation } from './roomPresentation';
  import type { ThreadOpenOptions } from './threadOpenOptions';
  import { RoomThreadingMode } from '@chatto/client/util/roomThreading';
  import { recentThreadRootCandidate } from './recentThreadRoot';

  let {
    roomId,
    threadId,
    routeMessageId
  }: { roomId: string; threadId?: string; routeMessageId?: string } = $props();

  const serverScope = useServerScope();
  const roomMembersStore = $derived(serverScope.store.rooms.members(roomId));
  setRoomMembersStore(() => roomMembersStore);
  const activeServerId = serverScope.serverId;
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const stores = serverScope.store;
  const roomFilesStore = $derived(stores.rooms.files(roomId));
  const roomMessageSearchStore = $derived(serverUi(stores).roomSearch(roomId));
  const serverInfo = $derived(stores.serverInfo);
  const appUi = getAppUiState();
  const desktopRoomLayout = new MediaQuery('(min-width: 1024px)', false);
  const THREAD_PANE_SPLIT_MIN_WIDTH = 768;
  let roomViewWidth = $state(0);
  let splitThreadLayout = $derived(
    userPreferences.threadPanePresentation === 'split' &&
      roomViewWidth >= THREAD_PANE_SPLIT_MIN_WIDTH
  );
  let threadPanePresentation: ThreadPanePresentation = $derived(
    splitThreadLayout ? 'split' : 'overlay'
  );

  const observeThreadLayout: Attachment<HTMLElement> = (element) => {
    // Overlay presentation does not depend on the room width. The preference
    // page remounts the room, so an opted-in split layout starts a fresh
    // observer when the user returns.
    if (userPreferences.threadPanePresentation !== 'split') {
      roomViewWidth = 0;
      return;
    }

    const update = (width: number) => {
      roomViewWidth = width;
    };
    update(element.getBoundingClientRect().width);

    let frame = 0;
    const observer = new ResizeObserver(([entry]) => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => update(entry.contentRect.width));
    });
    observer.observe(element);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  };

  const navigation = new RoomNavigationState(serverUi(stores).pendingHighlights);

  function openThread(threadRootEventId: string, options: ThreadOpenOptions = {}) {
    navigation.prepareThreadOpen(roomId, threadRootEventId, options);
    goto(conversationPath(activeServerId, roomId, { threadId: threadRootEventId, store: stores }));
  }

  function closeThread() {
    goto(conversationPath(activeServerId, roomId, { store: stores }));
  }

  // Create context-based state (must be synchronous, before children render)
  createMentionRoles(() => stores.mentionRoles.roles);
  const roomMessageStore = $derived(stores.rooms.messages(roomId));

  const room = useRoomData(() => ({ roomId }));
  const canReadMessages = $derived(room.roomData?.canReadMessages !== false);
  const shouldHydrateRoom = $derived(
    stores.realtimeSync.isRecoveringSnapshot || (Boolean(room.roomData) && canReadMessages)
  );

  // --- Extracted hooks ---
  const roomPinsStore = $derived(
    room.roomData && canReadMessages && !room.isDM ? stores.rooms.pins(roomId) : null
  );
  const roomNoticeStore = $derived(
    room.roomData && !room.isDM && stores.projection.rooms.get(roomId)?.viewerState?.isMember
      ? stores.rooms.notice(roomId)
      : null
  );
  const noticeMentions: MarkdownMentionOptions = $derived({
    members: roomMembersStore.members,
    roles: stores.mentionRoles.roles,
    viewerLogin: stores.currentUser.user?.login,
    searchMembers: (query: string) => roomMembersStore.searchMembers(query),
    onMentionClick: (userId: string) => appUi.openMemberProfile(userId)
  });

  // Room permissions — derived reactively, no $effect needed
  let permissions = $derived({
    ...(room.roomData ?? DEFAULT_ROOM_PERMISSIONS),
    canViewPinnedMessages: Boolean(roomPinsStore),
    canPinMessages:
      Boolean(room.roomData) &&
      !room.isDM &&
      !room.roomData?.room.archived &&
      Boolean(roomPinsStore) &&
      Boolean(room.roomData?.canPinMessages)
  });
  let composerCanAttach = $derived(room.roomData === undefined ? true : permissions.canAttach);
  let threadingMode = $derived(room.roomData?.room.threadingMode ?? RoomThreadingMode.ENABLED);
  let composerCanCreateThread = $derived(
    permissions.canPostMessage &&
      (threadingMode === RoomThreadingMode.REQUIRED ||
        (permissions.canPostInThread &&
          (threadingMode === RoomThreadingMode.ENABLED ||
            threadingMode === RoomThreadingMode.ENCOURAGED)))
  );
  let composerRequiresThread = $derived(
    permissions.canPostMessage && threadingMode === RoomThreadingMode.REQUIRED
  );

  function getRecentThreadRootCandidate() {
    const currentUserId = stores.viewerId;
    if (
      !currentUserId ||
      !permissions.canPostInThread ||
      threadingMode === RoomThreadingMode.DISABLED
    ) {
      return null;
    }
    return recentThreadRootCandidate(
      roomMessageStore.rootEvents,
      roomId,
      currentUserId,
      Date.now()
    );
  }

  createRoomPermissions(() => permissions);

  // roomData === null means the ready projection contains no visible room
  // (deleted, archived, or no access), so reaching this branch is genuine — clear
  // lastRoom so [spaceId]/+page.svelte's onMount doesn't redirect us right
  // back here in an infinite loop.
  $effect.pre(() => {
    if (room.roomData === null) {
      clearLastRoom(activeServerId);
      goto(resolve('/chat/[serverId]', { serverId: serverSegment }), { replaceState: true });
    }
  });

  const presentation = $derived(
    buildRoomPresentation({
      roomData: room.roomData,
      isDM: room.isDM,
      dmData: room.dmData,
      directMessageLabel: m('room.title.direct_message'),
      participantLabels: directMessageLabels(),
      getDisplayName: getLiveDisplayName
    })
  );

  // Remember this room as the last visited (for the chat-root → last-room
  // auto-redirect). Room.svelte is reused across roomId changes, so wait for
  // the loaded room data to catch up to the current route before writing.
  $effect(() => {
    if (room.roomData?.room.id === roomId) {
      setLastRoom(activeServerId, roomId);
    }
  });

  const highlightParam = $derived(page.url.searchParams.get('highlight'));
  const routeHighlightKey = $derived(
    JSON.stringify([roomId, threadId ?? null, routeMessageId ?? null, highlightParam])
  );

  /**
   * Remove a consumed `?highlight=` parameter so a refresh does not repeat the
   * jump. On a cold load this runs before SvelteKit's router starts, and the
   * router rejects history updates until then, so wait one tick first. The
   * highlight itself does not depend on this update.
   */
  async function removeHighlightParam(
    targetRoomId: string,
    targetThreadId: string | undefined,
    eventId: string
  ): Promise<void> {
    await tick();
    if (
      roomId !== targetRoomId ||
      threadId !== targetThreadId ||
      page.url.searchParams.get('highlight') !== eventId
    ) {
      return;
    }

    const path = conversationPath(activeServerId, targetRoomId, {
      threadId: targetThreadId,
      store: stores
    });
    try {
      replaceState(path, page.state);
    } catch (error) {
      console.warn('Failed to remove the highlight parameter:', error);
    }
  }

  // Header action visibility — flat derivations keep the template clean
  let showVoiceCall = $derived(!!room.roomData && !!serverInfo.livekitUrl);
  const messageSearchAvailable = $derived(
    serverUi(stores).messageSearch.statusError ||
      (serverUi(stores).messageSearch.statusLoaded &&
        serverUi(stores).messageSearch.status.state !== MessageSearchState.DISABLED)
  );
  // Channel rooms can be left unless membership is granted by Universal policy.
  let showLeaveRoom = $derived(!!room.roomData && !room.isDM && !room.roomData.room.isUniversal);
  const defaultDesktopRoomSidebarPanel = $derived(room.roomData && !room.isDM ? 'members' : null);
  const activeRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(
      room.isDM,
      appUi.desktopRoomSidebarPanel(defaultDesktopRoomSidebarPanel),
      showVoiceCall,
      messageSearchAvailable
    )
  );
  const mobileRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(
      room.isDM,
      appUi.mobileRoomSidebarPanel,
      showVoiceCall,
      messageSearchAvailable
    )
  );
  const directMessageProfileUserId = $derived.by(() => {
    const participantIds = room.dmData?.participantIds ?? [];
    const otherParticipantIds = participantIds.filter(
      (participantId) => participantId !== room.dmData?.currentUserId
    );
    if (otherParticipantIds.length === 1) {
      const [otherId] = otherParticipantIds;
      // A deleted partner has no profile to open.
      const other = room.dmData?.participants.find((participant) => participant.id === otherId);
      return other?.deleted ? null : otherId;
    }
    return participantIds.length === 1 && participantIds[0] === room.dmData?.currentUserId
      ? participantIds[0]
      : null;
  });
  const activeRoomSidebarProfileUserId = $derived(
    desktopRoomLayout.current
      ? appUi.desktopRoomSidebarProfileUserId(room.isDM ? directMessageProfileUserId : null)
      : appUi.activeRoomSidebarProfileUserId
  );
  const activeDesktopRoomSidebarProfileUserId = $derived(
    desktopRoomLayout.current ? activeRoomSidebarProfileUserId : null
  );
  const activeMobileRoomSidebarProfileUserId = $derived(
    desktopRoomLayout.current ? null : activeRoomSidebarProfileUserId
  );
  // The mobile overlay exists only below the desktop breakpoint. Its panel stays
  // selected in app UI state, so it reopens when the viewport narrows again.
  const hasMobileRoomSidebar = $derived(
    !desktopRoomLayout.current &&
      (mobileRoomSidebarPanel !== null || activeMobileRoomSidebarProfileUserId !== null)
  );
  const roomFilesPanelActive = $derived(
    visibleRoomSidebarPanel(
      desktopRoomLayout.current,
      activeRoomSidebarPanel,
      mobileRoomSidebarPanel
    ) === 'files'
  );
  const roomPinsPanelActive = $derived(
    visibleRoomSidebarPanel(
      desktopRoomLayout.current,
      activeRoomSidebarPanel,
      mobileRoomSidebarPanel
    ) === 'pins'
  );
  const roomSidebarTogglePanels = $derived(
    roomSidebarPanelsForRoom(room.isDM, showVoiceCall, messageSearchAvailable)
  );
  const hasActiveRoomCall = $derived(
    serverUi(stores).activeCallRooms.has(roomId) || serverUi(stores).voiceCall.isInCall(roomId)
  );
  const isDesktopCallMaximized = $derived(
    activeRoomSidebarPanel === 'call' &&
      hasActiveRoomCall &&
      appUi.isRoomCallWideFor(activeServerId, roomId)
  );
  const sharedRoomSidebarProps = $derived({
    roomId,
    roomName: room.roomData?.room.name ?? '',
    roomDescription: room.roomData?.room.description ?? '',
    roomCreatedAt: room.roomData?.room.createdAt ?? null,
    hasActiveCall: hasActiveRoomCall,
    loading: room.isRoomLoading,
    searchStore: roomMessageSearchStore,
    filesStore: roomFilesStore,
    pinsStore: roomPinsStore ?? undefined,
    noticeStore: roomNoticeStore ?? undefined,
    canManageNotice: room.roomData?.canManageNotice ?? false,
    noticeMentions,
    livekitUrl: serverInfo.livekitUrl ?? undefined,
    canBanRoomMembers: canBanMembersFromRoomSidebar(room.isDM, room.roomData?.canBanRoomMembers),
    isUniversal: room.roomData?.room.isUniversal ?? false,
    currentUserId: stores.viewerId,
    membersStore: roomMembersStore,
    onOpenProfile: (userId: string) => appUi.openMemberProfile(userId),
    onBackToMembers: appUi.isMemberProfileOpen ? () => appUi.backToRoomMembers() : undefined
  });

  const resources = new RoomResources();
  onDestroy(() => resources.dispose());
  // These leases belong to the selected room and panel, not to a DOM element.
  // Reconcile them together so a panel change cannot release unrelated caches.
  $effect(() => {
    const visible = room.roomData !== null;
    const selection = {
      files: visible && roomFilesPanelActive ? roomFilesStore : null,
      pins: visible ? roomPinsStore : null,
      notice: visible && stores.viewerId ? roomNoticeStore : null
    };
    // A DM already has projected members. A reset clears these flags and
    // permits a fresh read; an ordinary failure still requires explicit retry.
    const members = roomMembersStore;
    const membersToLoad =
      visible && !members.hasFirstPage && !members.isInitialLoading && !members.loadError
        ? members
        : null;
    // Client cache reads made while acquiring a lease are not selection inputs.
    untrack(() => resources.update(selection, membersToLoad));
  });

  $effect(() => {
    if (roomPinsPanelActive) roomPinsStore?.markSeen();
  });

  const syncRoomCallWide: Attachment = () => {
    const active = hasActiveRoomCall;
    const serverId = activeServerId;
    const selectedRoomId = roomId;
    if (!active) {
      untrack(() => appUi.disableRoomCallWideFor(serverId, selectedRoomId));
    }
  };

  let leavingRoom = $state(false);
  // Only an explicit open requests focus. Saved panels have no pending request.
  let focusSearchOnOpen = $state<RoomSidebarPresentation | null>(null);

  beforeNavigate(({ to }) => {
    focusSearchOnOpen = null;
    const highlight = navigation.highlight;
    // A click can queue the next room's request before navigation starts.
    // Cancel only a request for the room that this view currently owns.
    // The main timeline stays mounted when a same-room thread opens or changes.
    if (
      highlight?.roomId === roomId &&
      (to?.params?.serverId !== serverSegment ||
        (to?.params?.roomId ?? userConversationRoomId(stores, to?.params?.userId)) !==
          highlight.roomId ||
        (highlight.threadRootEventId !== null &&
          (to?.params?.threadId ?? null) !== highlight.threadRootEventId))
    ) {
      navigation.clearHighlight(highlight);
    }
  });

  function searchFocused(presentation: RoomSidebarPresentation): void {
    if (focusSearchOnOpen === presentation) focusSearchOnOpen = null;
  }

  function toggleDesktopRoomSidebarPanel(panel: RoomSidebarPanel): void {
    const wasSearchOpen =
      activeRoomSidebarPanel === 'search' && !activeDesktopRoomSidebarProfileUserId;
    focusSearchOnOpen = panel === 'search' && !wasSearchOpen ? 'desktop' : null;
    appUi.toggleDesktopRoomSidebarPanel(panel, defaultDesktopRoomSidebarPanel);
  }

  function toggleMobileRoomSidebarPanel(panel: RoomSidebarPanel): void {
    const wasSearchOpen =
      mobileRoomSidebarPanel === 'search' && !activeMobileRoomSidebarProfileUserId;
    focusSearchOnOpen = panel === 'search' && !wasSearchOpen ? 'mobile' : null;
    appUi.toggleMobileRoomSidebarPanel(panel);
  }

  function openDirectMessageProfile(userId: string): void {
    appUi.openRoomSidebarProfile(userId);
  }

  function openRoomCall(): void {
    appUi.requestRoomSidebarPanel(activeServerId, roomId, 'call', getRoomSidebarPresentation());
  }

  function closeDesktopRoomSidebarPanel(): void {
    focusSearchOnOpen = null;
    appUi.closeDesktopRoomSidebarPanel();
  }

  function closeDesktopRoomSidebar(): void {
    const wasMemberProfile = appUi.isMemberProfileOpen;
    if (activeRoomSidebarProfileUserId) {
      appUi.closeRoomSidebarProfile('desktop');
      if (!wasMemberProfile) return;
    }
    closeDesktopRoomSidebarPanel();
  }

  function closeMobileRoomSidebar(): void {
    focusSearchOnOpen = null;
    const wasMemberProfile = appUi.isMemberProfileOpen;
    if (activeRoomSidebarProfileUserId) {
      appUi.closeRoomSidebarProfile('mobile');
      if (!wasMemberProfile) return;
    }
    appUi.closeMobileRoomSidebarPanel();
  }

  function handleWindowKeydown(event: KeyboardEvent): void {
    if (
      event.defaultPrevented ||
      event.altKey ||
      event.shiftKey ||
      (!event.metaKey && !event.ctrlKey) ||
      event.key !== '/' ||
      !messageSearchAvailable
    ) {
      return;
    }

    event.preventDefault();
    if (desktopRoomLayout.current) {
      if (activeRoomSidebarPanel !== 'search' || activeDesktopRoomSidebarProfileUserId) {
        focusSearchOnOpen = 'desktop';
      }
      appUi.openDesktopRoomSidebarPanel('search');
    } else {
      if (mobileRoomSidebarPanel !== 'search' || activeMobileRoomSidebarProfileUserId) {
        focusSearchOnOpen = 'mobile';
      }
      appUi.openMobileRoomSidebarPanel('search');
    }
  }

  function toggleDesktopCallWide(): void {
    if (activeRoomSidebarPanel !== 'call' || !hasActiveRoomCall) return;
    appUi.toggleRoomCallWide(activeServerId, roomId);
  }

  /** Show a message from a sidebar panel in its thread or in the room timeline. */
  function openMessage(
    messageEventId: string,
    threadRootEventId: string | null,
    closeMobile = false
  ): void {
    if (threadRootEventId) {
      openThread(threadRootEventId, { highlightEventId: messageEventId });
    } else {
      navigation.beginHighlight(roomId, null, messageEventId);
    }
    if (closeMobile) {
      appUi.closeMobileRoomSidebarPanel();
    }
  }
</script>

<svelte:window
  onkeydown={(e) => {
    // The modal owns keyboard actions while the room remains visible behind it.
    if (page.state.modal) return;
    // An open menu or sheet consumes Escape before the underlying room panels.
    if (e.key === 'Escape' && document.querySelector(':popover-open, dialog[open]')) return;
    handleWindowKeydown(e);
    if (e.defaultPrevented) return;

    if (e.key === 'Escape' && hasMobileRoomSidebar && !e.defaultPrevented) {
      e.preventDefault();
      closeMobileRoomSidebar();
      return;
    }

    if (e.key === 'Escape' && threadId && !e.defaultPrevented) {
      e.preventDefault();
      closeThread();
    }
  }}
  onpointerdowncapture={(e) => {
    // Check before the menu's outside-pointer handler removes it. One press
    // dismisses only the menu, not the room overlay underneath it.
    if (document.querySelector(':popover-open')) return;

    if (hasMobileRoomSidebar && e.button === 0) {
      const target = e.target as HTMLElement;
      if (
        target.closest(
          '[data-testid="room-sidebar-mobile-pane"], [data-testid="room-sidebar-toggle"], dialog'
        )
      ) {
        return;
      }
      closeMobileRoomSidebar();
      return;
    }

    if (!threadId || splitThreadLayout || e.button !== 0) return;
    const target = e.target as HTMLElement;
    if (target.closest('[data-testid="thread-pane"], dialog')) return;
    // The dimmed room, left sidebar, and server gutter dismiss the overlay.
    // The room extras sidebar keeps its independent controls.
    if (!target.closest('[data-thread-dismiss-surface], [data-app-sidebar]')) return;
    closeThread();
  }}
/>

<!--
  Render the layout shell whether or not roomData has loaded. EventList stays
  mounted across roomId changes, so scroll and virtualization state can settle
  without remounting the whole room body.

  roomData === null triggers a redirect via $effect.pre above, so we skip
  rendering in that case to avoid a flash of the previous room's UI under
  the new (empty) data.
-->
<PageTitle title={presentation.pageTitle} />

{#if room.roomData?.room.id === roomId}
  {#key routeHighlightKey}
    <RoomRouteHighlight
      {roomId}
      {threadId}
      messageId={routeMessageId}
      {highlightParam}
      {navigation}
      onQueryConsumed={(targetRoomId, targetThreadId, eventId) =>
        void removeHighlightParam(targetRoomId, targetThreadId, eventId)}
    />
  {/key}
{/if}

{#key roomId}
  {#if shouldHydrateRoom}
    <RoomWindowLifecycle
      {roomId}
      store={stores}
      {navigation}
      isActive={(selectedRoomId) => selectedRoomId === roomId}
    />
  {/if}
{/key}

{#if room.roomData !== null}
  <div class="flex min-h-0 min-w-0 flex-1" {@attach syncRoomCallWide}>
    <div
      class={[
        '@container relative flex min-h-0 min-w-0 flex-1 overflow-hidden',
        isDesktopCallMaximized ? 'lg:hidden' : ''
      ]}
      data-testid="room-view-region"
      data-thread-presentation={splitThreadLayout ? 'split' : 'overlay'}
      data-thread-dismiss-surface
      {@attach observeThreadLayout}
    >
      <ConversationPane
        class={[
          'transition-opacity duration-200',
          threadId && canReadMessages && !splitThreadLayout ? 'opacity-30' : '',
          hasMobileRoomSidebar ? 'max-lg:opacity-30' : ''
        ]}
        data-testid="room-main-pane"
        data-navigation-content
        inert={(threadId && canReadMessages && !splitThreadLayout) || hasMobileRoomSidebar
          ? true
          : undefined}
        {roomId}
        messageStore={roomMessageStore}
        {threadingMode}
        {canReadMessages}
        hasLimitedMessageAccess={room.roomData?.hasLimitedMessageAccess ?? false}
        canPost={permissions.canPostMessage}
        canAttach={composerCanAttach}
        composer={{
          echoToConversation: room.isDM,
          slowModeSeconds: room.roomData?.room.slowModeSeconds ?? 0,
          slowModeNextPostAt: room.roomData?.slowModeNextPostAt ?? null,
          slowModeBypassed: permissions.canManageRoom || permissions.canManageOthersMessage,
          showCreateThread: composerCanCreateThread,
          createThreadRequired: composerRequiresThread,
          createThreadDefault: threadingMode === RoomThreadingMode.ENCOURAGED,
          threadsEncouraged: threadingMode === RoomThreadingMode.ENCOURAGED,
          getRecentThreadRootCandidate,
          autoFocus: !threadId && !hasMobileRoomSidebar,
          onThreadMessageSent: (threadRootEventId, event) =>
            openThread(threadRootEventId, { highlightEventId: event?.id })
        }}
        highlight={room.roomData?.room.id === roomId ? navigation.highlightFor(roomId, null) : null}
        onHighlightComplete={(highlight) => navigation.clearHighlight(highlight)}
        onOpenThread={openThread}
        onOpenCall={openRoomCall}
        onOpenProfile={(userId) => appUi.openMemberProfile(userId)}
      >
        {#snippet header()}
          {#snippet roomPanelActions(panels: RoomSidebarPanel[])}
            <RoomSidebarToggle
              mode="mobile"
              activePanel={activeMobileRoomSidebarProfileUserId ? null : mobileRoomSidebarPanel}
              {panels}
              hasActiveCall={hasActiveRoomCall}
              hasUnseenPins={roomPinsStore?.hasUnseen ?? false}
              onToggle={toggleMobileRoomSidebarPanel}
            />
            <RoomSidebarToggle
              mode="desktop"
              activePanel={activeRoomSidebarPanel}
              {panels}
              hasActiveCall={hasActiveRoomCall}
              hasUnseenPins={roomPinsStore?.hasUnseen ?? false}
              onToggle={toggleDesktopRoomSidebarPanel}
            />
          {/snippet}

          {#snippet directMessageTitle()}
            {#if room.dmData}<DirectMessageName
                participants={room.dmData.participants}
                currentUserId={room.dmData.currentUserId}
                getDisplayName={getLiveDisplayName}
              />{/if}
          {/snippet}
          <PaneHeader
            title={presentation.title}
            titleContent={room.isDM && room.dmData?.participants.length
              ? directMessageTitle
              : undefined}
            subtitle={presentation.description}
            collapseActions
            hideOnKeyboard
            actionsLabel={m('room_list.room_actions', { room: room.roomData?.room.name ?? '' })}
          >
            {#snippet actions()}
              {@render roomPanelActions(roomSidebarTogglePanels)}
              {#if room.isDM && directMessageProfileUserId}
                <HeaderIconButton
                  icon="icon-[uil--info-circle]"
                  label={m('chat.profile.title')}
                  tone={activeRoomSidebarProfileUserId ? 'active' : 'default'}
                  onclick={() => openDirectMessageProfile(directMessageProfileUserId)}
                />
              {/if}
              {#if showLeaveRoom}
                <HeaderIconButton
                  icon="icon-[uil--sign-out-alt] rtl:-scale-x-100"
                  label={m('room.leave.title')}
                  disabled={leavingRoom}
                  onclick={() =>
                    pushState('', {
                      modal: {
                        type: 'leaveRoom',
                        serverId: activeServerId,
                        roomId,
                        roomName: room.roomData!.room.name
                      }
                    })}
                />
              {/if}
            {/snippet}
            {#snippet collapsedActions()}
              {#if hasActiveRoomCall && roomSidebarTogglePanels.includes('call')}
                {@render roomPanelActions(['call'])}
              {/if}
            {/snippet}
          </PaneHeader>
          {#if roomNoticeStore && stores.viewerId}
            <div class="flex min-h-0 flex-col">
              <RoomNoticeBanner
                store={roomNoticeStore}
                serverId={activeServerId}
                viewerId={stores.viewerId}
                canManage={room.roomData?.canManageNotice ?? false}
                mentions={noticeMentions}
                timeSettings={timeFormatSettingsFor(stores.currentUser.user?.settings)}
              />
            </div>
          {/if}
        {/snippet}
      </ConversationPane>

      {#if threadId && (room.roomData || stores.realtimeSync.isRecoveringSnapshot) && canReadMessages}
        <ThreadPane
          {roomId}
          roomName={room.isDM ? presentation.title : (room.roomData?.room.name ?? '')}
          isDirectMessage={room.isDM}
          threadRootEventId={threadId}
          onClose={closeThread}
          canPostInThread={!!room.roomData?.canPostInThread &&
            threadingMode !== RoomThreadingMode.DISABLED}
          canAttach={!!room.roomData?.canAttach && threadingMode !== RoomThreadingMode.DISABLED}
          canEchoMessage={!!room.roomData?.canEchoMessage &&
            !!room.roomData?.canPostMessage &&
            threadingMode !== RoomThreadingMode.DISABLED}
          slowModeSeconds={room.roomData?.room.threadSlowModeSeconds ??
            room.roomData?.room.slowModeSeconds ??
            0}
          independentSlowMode={room.roomData?.room.threadSlowModeSeconds !== undefined}
          slowModeNextPostAt={room.roomData?.slowModeNextPostAt ?? null}
          slowModeBypassed={!!room.roomData?.canManageRoom ||
            !!room.roomData?.canManageOthersMessage}
          highlight={room.roomData?.room.id === roomId
            ? navigation.highlightFor(roomId, threadId)
            : null}
          composerInput={navigation.composerInputFor(roomId, threadId)}
          presentation={threadPanePresentation}
          {threadingMode}
          onOpenProfile={(userId) => appUi.openMemberProfile(userId)}
          onHighlightComplete={(highlight) => navigation.clearHighlight(highlight)}
          onComposerInputConsumed={(input) => navigation.clearComposerInput(input)}
        />
      {/if}

      <RoomSidebarPane
        presentation="mobile"
        sidebarProps={hasMobileRoomSidebar
          ? {
              ...sharedRoomSidebarProps,
              activePanel: mobileRoomSidebarPanel ?? 'members',
              focusSearchOnMount: focusSearchOnOpen === 'mobile',
              onSearchFocused: () => searchFocused('mobile'),
              activeProfileUserId: activeMobileRoomSidebarProfileUserId,
              onOpenFileMessage: (messageEventId, threadRootEventId) =>
                openMessage(messageEventId, threadRootEventId, true),
              onOpenSearchResult: (messageEventId, threadRootEventId) =>
                openMessage(messageEventId, threadRootEventId, true),
              onOpenPin: (messageEventId, threadRootEventId) =>
                openMessage(messageEventId, threadRootEventId, true),
              onClose: closeMobileRoomSidebar
            }
          : null}
      />
    </div>

    {#if activeRoomSidebarPanel || activeDesktopRoomSidebarProfileUserId}
      <RoomSidebarPane
        presentation="desktop"
        sidebarProps={{
          ...sharedRoomSidebarProps,
          activePanel: activeRoomSidebarPanel ?? 'members',
          focusSearchOnMount: focusSearchOnOpen === 'desktop',
          onSearchFocused: () => searchFocused('desktop'),
          activeProfileUserId: activeDesktopRoomSidebarProfileUserId,
          maximized: isDesktopCallMaximized,
          onOpenFileMessage: openMessage,
          onOpenSearchResult: openMessage,
          onOpenPin: openMessage,
          onToggleMaximized: toggleDesktopCallWide,
          onClose: closeDesktopRoomSidebar
        }}
      />
    {/if}
  </div>
{/if}
