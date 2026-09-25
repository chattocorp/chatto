<script lang="ts">
  import DirectMessageName from '$lib/components/users/DirectMessageName.svelte';
  import { untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { MediaQuery } from 'svelte/reactivity';
  import { beforeNavigate, goto, pushState, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { dropZone } from '$lib/attachments/dropZone.svelte';
  import DropZoneOverlay from '$lib/attachments/DropZoneOverlay.svelte';
  import MessageComposer, {
    type MessageComposerApi
  } from '$lib/components/composer/MessageComposer.svelte';
  import {
    useRoomData,
    useRoomUnread,
    useProjectionEvent,
    createTypingIndicator
  } from '$lib/hooks';
  import { appState } from '$lib/state/globals.svelte';
  import { m } from '$lib/i18n/messages';
  import {
    createComposerContext,
    createMentionRoles,
    getRoomMembers,
    setRoomMembersStore,
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS
  } from '$lib/state/room';
  import { useTimelineMutations } from '$lib/hooks/useTimelineMutations.svelte';
  import {
    getAppUiState,
    getRoomSidebarPresentation,
    type RoomSidebarPresentation
  } from '$lib/state/appUi.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { MessageSearchState } from '$lib/state/server/messageSearch.svelte';
  import { threadPaneWidth } from '$lib/state/threadPaneWidth.svelte';
  import { userPreferences, type ThreadPanePresentation } from '$lib/state/userPreferences.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { clearLastRoom, setLastRoom } from '$lib/storage/lastRoom';
  import type { RoomSidebarPanel } from '$lib/storage/roomSidebarPanel';
  import { toast } from '$lib/ui/toast';
  import { EmptyState, Hint } from '$lib/ui';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import HeaderIconButton from '$lib/ui/HeaderIconButton.svelte';
  import { tick } from 'svelte';
  import RoomEventsPane from './RoomEventsPane.svelte';
  import RoomSidebarPane from './RoomSidebarPane.svelte';
  import RoomSidebarToggle from './RoomSidebarToggle.svelte';
  import {
    canBanMembersFromRoomSidebar,
    roomSidebarPanelForRoom,
    roomSidebarPanelsForRoom,
    visibleRoomSidebarPanel
  } from './roomSidebarBehavior';
  import { RoomNavigationState } from './roomNavigationState.svelte';
  import { buildRoomPresentation } from './roomPresentation';
  import type { ThreadOpenOptions } from './threadOpenOptions';
  import { RoomThreadingMode } from '$lib/roomThreading';
  import { recentThreadRootCandidate } from './recentThreadRoot';

  let threadPaneModule: Promise<typeof import('./ThreadPane.svelte')> | null = null;
  let threadPaneLoadAttempt = $state(0);

  function loadThreadPane(_attempt: number) {
    threadPaneModule ??= import('./ThreadPane.svelte').catch((error: unknown) => {
      threadPaneModule = null;
      throw error;
    });
    return threadPaneModule;
  }

  let {
    roomId,
    threadId,
    routeMessageId
  }: { roomId: string; threadId?: string; routeMessageId?: string } = $props();

  const serverScope = useServerScope();
  const roomMembersStore = $derived(serverScope.store.membersForRoom(roomId));
  setRoomMembersStore(() => roomMembersStore);
  const activeServerId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const stores = $derived(serverScope.store);
  const roomFilesStore = $derived(stores.filesForRoom(roomId));
  const roomMessageSearchStore = $derived(stores.messageSearchForRoom(roomId));
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

  const navigation = new RoomNavigationState();

  function openThread(threadRootEventId: string, options: ThreadOpenOptions = {}) {
    navigation.prepareThreadOpen(threadRootEventId, options);
    goto(
      resolve('/chat/[serverId]/[roomId]/[threadId]', {
        serverId: serverSegment,
        roomId,
        threadId: threadRootEventId
      })
    );
  }

  function closeThread() {
    goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  // Create context-based state (must be synchronous, before children render)
  const composerContext = createComposerContext({ scroll: true });
  createMentionRoles(() => stores.mentionRoles.roles);
  const replyState = composerContext.replyState;
  let replyStateRoomId: string | null = null;
  const jumpState = composerContext.jumpState;
  const currentUser = $derived(stores.currentUser);
  const roomMessageStore = $derived(stores.messagesForRoom(roomId));

  const room = useRoomData(() => ({ roomId }));
  const canReadMessages = $derived(room.roomData?.canReadMessages !== false);
  const shouldHydrateRoom = $derived(
    stores.realtimeSync.isRecoveringSnapshot || (Boolean(room.roomData) && canReadMessages)
  );

  $effect(() => {
    const mountedStores = stores;
    const selectedRoomId = roomId;
    const hydrateRoom = shouldHydrateRoom;
    if (hydrateRoom) untrack(() => mountedStores.restoreProjectedRoomWindow(selectedRoomId));
    return () => {
      // Invalidate any historical-window request before this room becomes
      // inactive. Its late response must not replace the retained latest
      // projection while another room is being rendered.
      navigation.clearMainHighlight();
      if (hydrateRoom) untrack(() => mountedStores.restoreProjectedRoomWindow(selectedRoomId));
    };
  });

  useTimelineMutations(() => ({
    serverId: activeServerId,
    roomId,
    timeline: roomMessageStore
  }));

  // --- Extracted hooks ---
  const supportsPinnedMessages = $derived(serverInfo.supportsFeature('pinnedMessages'));
  const roomPinsStore = $derived(
    room.roomData && canReadMessages && !room.isDM && supportsPinnedMessages
      ? stores.pinsForRoom(roomId)
      : null
  );

  $effect(() => {
    const currentRoomId = roomId;
    if (replyStateRoomId === null) {
      replyStateRoomId = currentRoomId;
      return;
    }
    if (replyStateRoomId === currentRoomId) return;
    replyStateRoomId = currentRoomId;
    replyState.cancelReply();
  });

  $effect(() => {
    if (!stores.isAuthenticated) return;
    void stores.mentionRoles.refresh();
  });

  const unread = useRoomUnread(() => ({
    roomId,
    events: roomMessageStore.rootEvents,
    canReadMessages
  }));

  // Room permissions — derived reactively, no $effect needed
  let permissions = $derived({
    ...(room.roomData ?? DEFAULT_ROOM_PERMISSIONS),
    canViewPinnedMessages: Boolean(roomPinsStore),
    canPinMessages:
      Boolean(room.roomData) &&
      !room.isDM &&
      !room.roomData?.room.archived &&
      Boolean(roomPinsStore) &&
      Boolean(room.roomData?.canManageRoom)
  });
  let composerCanAttach = $derived(room.roomData === undefined ? true : permissions.canAttach);
  let threadingMode = $derived(room.roomData?.room.threadingMode ?? RoomThreadingMode.ENABLED);
  const postingNotice = $derived.by(() => {
    if (room.roomData?.canPostMessage !== false) return null;
    if (
      canReadMessages &&
      !room.roomData.room.archived &&
      threadingMode !== RoomThreadingMode.DISABLED
    ) {
      if (room.roomData.canPostInThread) return m('room.timeline.post_threads_only');
      if (room.roomData.canPostInteractions) {
        return m(
          room.isDM
            ? 'room.timeline.post_interactions_only'
            : 'room.timeline.post_interactions_only_channel'
        );
      }
    }
    return m('room.timeline.post_denied');
  });
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
    const currentUserId = currentUser.user?.id;
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
      currentUserLabel: m('common.you'),
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

  // Resolve the pending highlight once room data has loaded for the
  // current roomId. Two sources, in priority order:
  //   1. PendingHighlightStore — set by in-app navigations (notification
  //      clicks, message-link redirects). One-shot, consumed-on-success.
  //   2. ?highlight= URL param — for shareable permalinks. Stripped after
  //      consumption so a refresh doesn't re-fire it.
  $effect(() => {
    if (!room.roomData) return;
    // Room.svelte lives in +layout and is reused across roomId changes; bail
    // until the new room's data has actually loaded.
    if (room.roomData.room.id !== roomId) return;

    const threadMessageTarget = navigation.consumeThreadMessageRoute(
      roomId,
      threadId,
      routeMessageId
    );
    if (threadMessageTarget !== undefined) {
      if (threadMessageTarget) applyHighlight(threadMessageTarget);
      return;
    }

    const pending = stores.pendingHighlights.consume(roomId, threadId ?? null);
    if (pending) {
      applyHighlight(pending.eventId, pending.notificationId);
      return;
    }

    const fromUrl = navigation.consumeHighlightParam(
      roomId,
      threadId,
      page.url.searchParams.get('highlight')
    );
    if (!fromUrl) return;

    applyHighlight(fromUrl);
    void removeHighlightParam(roomId, threadId, fromUrl);
  });

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

    const path = targetThreadId
      ? resolve('/chat/[serverId]/[roomId]/[threadId]', {
          serverId: serverSegment,
          roomId: targetRoomId,
          threadId: targetThreadId
        })
      : resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId: targetRoomId });
    try {
      replaceState(path, page.state);
    } catch (error) {
      console.warn('Failed to remove the highlight parameter:', error);
    }
  }

  function applyHighlight(eventId: string, notificationId: string | null = null): void {
    const requestId = navigation.beginHighlight(eventId, !!threadId);
    if (requestId === null) return;
    const targetRoomId = roomId;

    tick().then(async () => {
      const jumped = await jumpState.jumpToMessage(eventId);
      if (!serverScope.isCurrent() || targetRoomId !== roomId) return;
      if (jumped && notificationId) {
        void stores.notifications.markOccurrenceRead(notificationId).catch((error) => {
          console.error('Failed to mark displayed notification read:', error);
        });
      }
      if (!jumped && navigation.failMainHighlight(requestId, eventId)) {
        toast.error(m('room.jump_failed'));
      }
    });
  }

  // Canonical facts invalidate the explicit reads that own this room's data.
  useProjectionEvent((event) => {
    const semantic = event.event?.event;
    if (!semantic) return;
    if (semantic.case === 'messagePosted' && semantic.value.roomId === roomId) {
      if (semantic.value.threadRootEventId) return;
      const actorId = event.event?.actorId;
      if (actorId) typingIndicator.removeTypingUser(actorId);
      if (currentUser.user && actorId !== currentUser.user.id && appState.isPresent) {
        unread.markRoomAsRead(roomId, event.event?.id ?? '');
      }
      return;
    }
  });

  // Header action visibility — flat derivations keep the template clean
  let showVoiceCall = $derived(!!room.roomData && !!serverInfo.livekitUrl);
  const supportsMessageSearch = $derived(serverInfo.supportsFeature('messageSearch'));
  const messageSearchAvailable = $derived(
    supportsMessageSearch &&
      (stores.messageSearch.statusError ||
        (stores.messageSearch.statusLoaded &&
          stores.messageSearch.status.state !== MessageSearchState.DISABLED))
  );
  $effect(() => {
    if (supportsMessageSearch && stores.isAuthenticated) void stores.messageSearch.ensureStatus();
  });
  // Channel rooms can be left unless membership is granted by Universal policy.
  let showLeaveRoom = $derived(!!room.roomData && !room.isDM && !room.roomData.room.isUniversal);
  const defaultDesktopRoomSidebarPanel = $derived(room.roomData && !room.isDM ? 'members' : null);
  const activeRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(
      room.isDM,
      appUi.desktopRoomSidebarPanel(defaultDesktopRoomSidebarPanel),
      showVoiceCall,
      messageSearchAvailable,
      supportsPinnedMessages
    )
  );
  const mobileRoomSidebarPanel = $derived(
    roomSidebarPanelForRoom(
      room.isDM,
      appUi.mobileRoomSidebarPanel,
      showVoiceCall,
      messageSearchAvailable,
      supportsPinnedMessages
    )
  );
  const directMessageProfileUserId = $derived.by(() => {
    const participantIds = room.dmData?.participantIds ?? [];
    const otherParticipantIds = participantIds.filter(
      (participantId) => participantId !== room.dmData?.currentUserId
    );
    if (otherParticipantIds.length === 1) return otherParticipantIds[0];
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
  const hasMobileRoomSidebar = $derived(
    mobileRoomSidebarPanel !== null || activeMobileRoomSidebarProfileUserId !== null
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
    roomSidebarPanelsForRoom(
      room.isDM,
      showVoiceCall,
      messageSearchAvailable,
      supportsPinnedMessages
    )
  );
  const hasActiveRoomCall = $derived(
    stores.activeCallRooms.has(roomId) || stores.voiceCall.isInCall(roomId)
  );
  const isDesktopCallMaximized = $derived(
    activeRoomSidebarPanel === 'call' &&
      hasActiveRoomCall &&
      appUi.isRoomCallWideFor(activeServerId, roomId)
  );
  const sharedRoomSidebarProps = $derived({
    roomId,
    hasActiveCall: hasActiveRoomCall,
    loading: room.isRoomLoading,
    searchStore: roomMessageSearchStore,
    filesStore: roomFilesStore,
    pinsStore: roomPinsStore ?? undefined,
    livekitUrl: serverInfo.livekitUrl ?? undefined,
    canBanRoomMembers: canBanMembersFromRoomSidebar(room.isDM, room.roomData?.canBanRoomMembers),
    isUniversal: room.roomData?.room.isUniversal ?? false,
    currentUserId: stores.viewerId,
    membersStore: roomMembersStore,
    onOpenProfile: (userId: string) => appUi.openMemberProfile(userId),
    onBackToMembers: appUi.isMemberProfileOpen ? () => appUi.backToRoomMembers() : undefined
  });

  const syncRoomMembers: Attachment = () => {
    const selectedRoomId = roomId;
    const hasFirstPage = roomMembersStore.hasFirstPage;
    const hasCompleteMembership = stores.hasCompleteProjectedRoomMembership(selectedRoomId);
    const projectedMemberIds = hasCompleteMembership
      ? stores.projectedMemberIdsForRoom(selectedRoomId)
      : [];
    untrack(() => {
      roomMembersStore.setRoom(selectedRoomId);
      if (hasCompleteMembership) {
        roomMembersStore.replaceProjection(selectedRoomId, projectedMemberIds);
      } else {
        if (!hasFirstPage) roomMembersStore.ensureLoaded();
      }
    });
  };

  const syncRoomFiles: Attachment = () => {
    const store = roomFilesStore;
    const active = roomFilesPanelActive;
    if (active) return untrack(() => store.retain());
  };

  const syncRoomPins: Attachment = () => {
    const store = roomPinsStore;
    if (!store) return;
    return untrack(() => store.retain());
  };

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

  beforeNavigate(() => {
    focusSearchOnOpen = null;
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

  function openFileMessage(
    messageEventId: string,
    threadRootEventId: string | null,
    closeMobile = false
  ): void {
    if (threadRootEventId) {
      openThread(threadRootEventId, { highlightEventId: messageEventId });
    } else {
      void jumpState.jumpToMessage(messageEventId);
    }
    if (closeMobile) {
      appUi.closeMobileRoomSidebarPanel();
    }
  }

  function openSearchResult(
    messageEventId: string,
    threadRootEventId: string | null,
    closeMobile = false
  ): void {
    if (threadRootEventId) {
      openThread(threadRootEventId, { highlightEventId: messageEventId });
    } else {
      void jumpState.jumpToMessage(messageEventId);
    }
    if (closeMobile) appUi.closeMobileRoomSidebarPanel();
  }

  function openPinnedMessage(
    messageEventId: string,
    threadRootEventId: string | null,
    closeMobile = false
  ): void {
    openFileMessage(messageEventId, threadRootEventId, closeMobile);
  }

  // Drop zone state for drag-and-drop uploads
  let isDraggingFiles = $state(false);
  let composerApi = $state<MessageComposerApi | null>(null);

  // Drop zone attachment - only active when user can post and attach files.
  const roomDropZone = $derived(
    room.roomData?.canPostMessage && room.roomData?.canAttach
      ? dropZone({
          onDrop: (files) => composerApi?.addFiles(files),
          onDragStateChange: (dragging) => (isDraggingFiles = dragging)
        })
      : undefined
  );

  // Typing indicator for main room (not thread)
  const typingIndicator = createTypingIndicator(() => ({
    roomId,
    threadRootEventId: null,
    currentUserId: stores.viewerId
  }));
</script>

<svelte:window
  onkeydown={(e) => {
    // The modal owns keyboard actions while the room remains visible behind it.
    if (page.state.modal) return;
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
  onpointerdown={(e) => {
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
    // A thread is an overlay over the room view, so only the dimmed room
    // surface behind it should behave as a click-outside dismissal target.
    // Controls elsewhere in the app (such as the room extras sidebar) manage
    // their own state and must not close the thread as a side effect.
    if (!target.closest('[data-thread-dismiss-surface]')) return;
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
{#if room.roomData !== null}
  {#if presentation.pageTitle}
    <PageTitle title={presentation.pageTitle} />
  {/if}

  <div
    class="flex min-h-0 min-w-0 flex-1"
    {@attach syncRoomMembers}
    {@attach syncRoomFiles}
    {@attach syncRoomPins}
    {@attach syncRoomCallWide}
  >
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
      <div
        class={[
          'relative flex min-h-0 min-w-0 flex-1 flex-col transition-opacity duration-200',
          threadId && canReadMessages && !splitThreadLayout ? 'opacity-30' : '',
          hasMobileRoomSidebar ? 'max-lg:opacity-30' : ''
        ]}
        data-testid="room-main-pane"
        inert={(threadId && canReadMessages && !splitThreadLayout) || hasMobileRoomSidebar
          ? true
          : undefined}
        {@attach roomDropZone}
      >
        <DropZoneOverlay visible={isDraggingFiles} />

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
                icon="icon-[uil--sign-out-alt]"
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

        {#if postingNotice || room.roomData?.hasLimitedMessageAccess}
          <div class="flex shrink-0 flex-col gap-2 p-2" data-testid="room-permission-notices">
            {#if postingNotice}
              <div data-testid="room-post-denied">
                <Hint>{postingNotice}</Hint>
              </div>
            {/if}
            {#if room.roomData?.hasLimitedMessageAccess}
              <div data-testid="limited-message-access">
                <Hint>
                  {m(
                    room.isDM ? 'room.timeline.limited_access_dm' : 'room.timeline.limited_access'
                  )}
                </Hint>
              </div>
            {/if}
          </div>
        {/if}

        {#if canReadMessages}
          <RoomEventsPane
            hasLimitedMessageAccess={room.roomData?.hasLimitedMessageAccess ?? false}
            {roomId}
            messageStore={roomMessageStore}
            unreadMarkerEventId={unread.unreadMarkerEventId}
            onUnreadMarkerCleared={() => unread.clearUnreadMarker()}
            onOpenThread={openThread}
            onOpenCall={openRoomCall}
            pendingHighlightId={navigation.pendingMainHighlightId}
            onOpenProfile={(userId) => appUi.openMemberProfile(userId)}
            onHighlightComplete={() => navigation.clearMainHighlight()}
            typingUserIds={typingIndicator.userIds}
            typingMembers={getRoomMembers()}
            {threadingMode}
          />
        {:else}
          <EmptyState icon="icon-[uil--eye-slash]">
            {m('room.timeline.read_denied')}
          </EmptyState>
        {/if}

        <MessageComposer
          {roomId}
          echoToConversation={room.isDM}
          canPost={permissions.canPostMessage}
          canAttach={composerCanAttach}
          slowModeSeconds={room.roomData?.room.slowModeSeconds ?? 0}
          slowModeNextPostAt={room.roomData?.slowModeNextPostAt ?? null}
          slowModeBypassed={permissions.canManageRoom || permissions.canManageOthersMessage}
          showCreateThread={composerCanCreateThread}
          createThreadRequired={composerRequiresThread}
          createThreadDefault={threadingMode === RoomThreadingMode.ENCOURAGED}
          threadsEncouraged={threadingMode === RoomThreadingMode.ENCOURAGED}
          {getRecentThreadRootCandidate}
          inReplyTo={replyState.messageEventId ?? undefined}
          replyDisplayName={replyState.actorDisplayName || undefined}
          replyIdentity={replyState.actorIdentity}
          replyExcerpt={replyState.excerpt || undefined}
          onCancelReply={() => replyState.cancelReply()}
          autoFocus={!threadId && !hasMobileRoomSidebar}
          onReady={(api) => (composerApi = api)}
          onTyping={() => typingIndicator?.sendTypingIndicator()}
          onMessageSent={(event) => {
            typingIndicator?.resetDebounce();
            if (event) {
              roomMessageStore.ingestEvent(event);
            } else {
              void roomMessageStore.refreshCurrentWindow(null);
            }
          }}
          onThreadMessageSent={(threadRootEventId, event) => {
            typingIndicator?.resetDebounce();
            openThread(threadRootEventId, { highlightEventId: event?.id });
          }}
        />
      </div>

      {#if threadId && (room.roomData || stores.realtimeSync.isRecoveringSnapshot) && canReadMessages}
        {#await loadThreadPane(threadPaneLoadAttempt)}
          <div
            class={[
              'flex min-h-0 min-w-0 flex-col overflow-hidden border-s border-border bg-background p-4',
              splitThreadLayout
                ? 'relative w-[var(--thread-pane-width)] shrink-0'
                : 'absolute inset-y-0 end-0 z-10 w-full inline-end-overlay-shadow lg:w-[90%]'
            ]}
            data-testid="thread-pane"
            style:--thread-pane-width={`${threadPaneWidth.value}px`}
          >
            <LoadingFog class="min-h-0 w-full flex-1" />
          </div>
        {:then { default: ThreadPane }}
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
            slowModeSeconds={room.roomData?.room.slowModeSeconds ?? 0}
            slowModeNextPostAt={room.roomData?.slowModeNextPostAt ?? null}
            slowModeBypassed={!!room.roomData?.canManageRoom ||
              !!room.roomData?.canManageOthersMessage}
            highlightEventId={navigation.pendingThreadHighlight}
            pendingQuote={navigation.pendingThreadQuote}
            pendingReply={navigation.pendingThreadReply}
            presentation={threadPanePresentation}
            {threadingMode}
            onOpenProfile={(userId) => appUi.openMemberProfile(userId)}
            onHighlightComplete={() => navigation.clearThreadHighlight()}
            onQuoteConsumed={() => navigation.clearThreadQuote()}
            onReplyConsumed={() => navigation.clearThreadReply()}
          />
        {:catch}
          <div
            class={[
              'flex min-h-0 min-w-0 flex-col items-center justify-center gap-3 overflow-hidden border-s border-border bg-background p-4 text-center',
              splitThreadLayout
                ? 'relative w-[var(--thread-pane-width)] shrink-0'
                : 'absolute inset-y-0 end-0 z-10 w-full inline-end-overlay-shadow lg:w-[90%]'
            ]}
            data-testid="thread-pane"
            style:--thread-pane-width={`${threadPaneWidth.value}px`}
          >
            <p class="text-sm text-muted">{m('common.error.network')}</p>
            <div class="flex gap-2">
              <button
                type="button"
                class="btn-secondary"
                onclick={() => (threadPaneLoadAttempt += 1)}
              >
                {m('common.retry')}
              </button>
              <button type="button" class="btn-secondary" onclick={closeThread}>
                {m('room.thread.close')}
              </button>
            </div>
          </div>
        {/await}
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
                openFileMessage(messageEventId, threadRootEventId, true),
              onOpenSearchResult: (messageEventId, threadRootEventId) =>
                openSearchResult(messageEventId, threadRootEventId, true),
              onOpenPin: (messageEventId, threadRootEventId) =>
                openPinnedMessage(messageEventId, threadRootEventId, true),
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
          onOpenFileMessage: openFileMessage,
          onOpenSearchResult: openSearchResult,
          onOpenPin: openPinnedMessage,
          onToggleMaximized: toggleDesktopCallWide,
          onClose: closeDesktopRoomSidebar
        }}
      />
    {/if}
  </div>
{/if}
