import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { SvelteMap } from 'svelte/reactivity';
import { q } from '$lib/test-utils';
import { TimelineEventKind } from '$lib/render/timelineEvents';
import { threadPaneWidth } from '$lib/state/threadPaneWidth.svelte';
import { THREAD_PANE_MAX_WIDTH } from '$lib/storage/threadPaneWidth';
import ThreadPane from './ThreadPane.svelte';
import { ThreadPaneTestStore } from './ThreadPaneTestStore.svelte';

const { mocks } = vi.hoisted(() => {
  return {
    mocks: {
      markThreadAsRead: vi.fn(),
      followThread: vi.fn(),
      unfollowThread: vi.fn(),
      setThread: vi.fn(),
      retainMessagesForThread: vi.fn(),
      releaseMessagesForThread: vi.fn(),
      nextServerRetainMessagesForThread: vi.fn(),
      nextServerReleaseMessagesForThread: vi.fn(),
      disposeMessagesStore: vi.fn(),
      ingestEvent: vi.fn(),
      refreshCurrentWindow: vi.fn(),
      setThreadRootFollowState: vi.fn(),
      loadMore: vi.fn(),
      applyLocalMessageDeletion: vi.fn(),
      refreshAnchorForMessageMutation: vi.fn(),
      removeTypingUser: vi.fn(),
      sendTypingIndicator: vi.fn(),
      resetTypingDebounce: vi.fn(),
      jumpToMessage: vi.fn(),
      onClose: vi.fn(),
      clearUnreadMarker: vi.fn(),
      unreadMarkerEventId: null as string | null,
      appState: {
        isPresent: true
      },
      threadStore: null as ThreadPaneTestStore | null,
      nextServerThreadStore: null as ThreadPaneTestStore | null
    }
  };
});

const scopeState = new SvelteMap([['serverId', 'server-1']]);

vi.mock('$lib/api-client/readState', () => ({
  createReadStateAPI: () => ({
    markThreadAsRead: mocks.markThreadAsRead
  })
}));

vi.mock('$lib/api-client/threads', () => ({
  createThreadAPI: () => ({
    followThread: mocks.followThread,
    unfollowThread: mocks.unfollowThread
  })
}));

vi.mock('$lib/hooks', () => ({
  useProjectionEvent: vi.fn(),
  useUnreadMarker: (
    getTargetId: () => string,
    options: {
      markAsRead: (
        targetId: string,
        upToEventId: string | undefined,
        signal: AbortSignal
      ) => unknown;
    }
  ) => {
    void options.markAsRead(getTargetId(), undefined, new AbortController().signal);
    return {
      unreadMarkerEventId: mocks.unreadMarkerEventId,
      markAsRead: options.markAsRead,
      clearUnreadMarker: mocks.clearUnreadMarker
    };
  },
  createTypingIndicator: () => ({
    userIds: [],
    removeTypingUser: mocks.removeTypingUser,
    sendTypingIndicator: mocks.sendTypingIndicator,
    resetDebounce: mocks.resetTypingDebounce
  })
}));

vi.mock('$lib/state/server/scope.svelte', async () => {
  const { serverRegistry } = await import('$lib/state/server/registry.svelte');
  return {
    useServerScope: () => ({
      get serverId() {
        return scopeState.get('serverId')!;
      },
      connection: {
        serverId: 'server-1',
        connectBaseUrl: 'http://localhost/api/connect',
        bearerToken: null,
        getAPI: (factory: (config: never) => unknown) => factory({} as never)
      },
      get store() {
        return serverRegistry.getStore(scopeState.get('serverId')!);
      }
    })
  };
});

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: (serverId: string) => ({
      currentUser: { user: { id: 'test-user', login: 'testuser' }, loading: false },
      retainMessagesForThread:
        serverId === 'server-2'
          ? mocks.nextServerRetainMessagesForThread
          : mocks.retainMessagesForThread,
      releaseMessagesForThread:
        serverId === 'server-2'
          ? mocks.nextServerReleaseMessagesForThread
          : mocks.releaseMessagesForThread,
      messagesForThread: () =>
        Object.assign(serverId === 'server-2' ? mocks.nextServerThreadStore! : mocks.threadStore!, {
          isLoadingMore: false,
          hasReachedStart: true,
          setThread: mocks.setThread,
          dispose: mocks.disposeMessagesStore,
          ingestEvent: mocks.ingestEvent,
          refreshCurrentWindow: mocks.refreshCurrentWindow,
          setThreadRootFollowState: mocks.setThreadRootFollowState,
          loadMore: mocks.loadMore,
          applyLocalMessageDeletion: mocks.applyLocalMessageDeletion,
          refreshAnchorForMessageMutation: mocks.refreshAnchorForMessageMutation
        })
    })
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server-1'
}));

vi.mock('$lib/state/globals.svelte', () => ({
  appState: mocks.appState
}));

vi.mock('$lib/state/room', () => ({
  getRoomMembers: () => [],
  createComposerContext: () => ({
    replyState: {
      messageEventId: null,
      actorDisplayName: '',
      excerpt: '',
      startReply: vi.fn(),
      cancelReply: vi.fn()
    },
    quoteInsertionState: {
      requestInsertQuote: vi.fn()
    },
    jumpState: {
      scrollToEventId: null,
      setJumpHandler: vi.fn(),
      jumpToMessage: mocks.jumpToMessage
    }
  }),
  MessagesStore: class {
    threadEvents = [];
    isInitialLoading = false;
    isLoadingMore = false;
    hasReachedStart = true;
    setThread = mocks.setThread;
    dispose = mocks.disposeMessagesStore;
    ingestEvent = mocks.ingestEvent;
    refreshCurrentWindow = mocks.refreshCurrentWindow;
    setThreadRootFollowState = mocks.setThreadRootFollowState;
    loadMore = mocks.loadMore;
    applyLocalMessageDeletion = mocks.applyLocalMessageDeletion;
    refreshAnchorForMessageMutation = mocks.refreshAnchorForMessageMutation;
  }
}));

vi.mock('$lib/state/room/messageMutationEvents', () => ({
  onRoomMessageMutated: vi.fn(() => vi.fn())
}));

vi.mock('./EventList.svelte', async () => {
  const { default: EventListContractMock } = await import('./EventListContractMock.svelte');
  return { default: EventListContractMock };
});

vi.mock('$lib/components/composer/MessageComposer.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

describe('ThreadPane', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    threadPaneWidth.reset();
    mocks.threadStore = new ThreadPaneTestStore();
    mocks.nextServerThreadStore = new ThreadPaneTestStore();
    scopeState.set('serverId', 'server-1');
    mocks.appState.isPresent = true;
    mocks.unreadMarkerEventId = null;
    mocks.markThreadAsRead.mockResolvedValue({
      previousReadAt: null,
      lastReadAt: '2026-07-04T13:00:00Z'
    });
    mocks.followThread.mockResolvedValue({
      following: true,
      state: { roomId: 'room-1', threadRootEventId: 'thread-root', following: true }
    });
    mocks.unfollowThread.mockResolvedValue({
      following: false,
      state: { roomId: 'room-1', threadRootEventId: 'thread-root', following: false }
    });
  });

  it('uses the persisted width and accessible resize handle in split layouts', async () => {
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose,
        presentation: 'split'
      }
    });

    const pane = q(container, '[data-testid="thread-pane"]') as HTMLElement;
    const handle = q(container, '[role="slider"][aria-label^="Resize:"]') as HTMLElement;

    expect(pane.className).toContain('relative');
    expect(pane.style.getPropertyValue('--thread-pane-width')).toBe('420px');

    handle.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true }));

    await vi.waitFor(() => {
      expect(pane.style.getPropertyValue('--thread-pane-width')).toBe(`${THREAD_PANE_MAX_WIDTH}px`);
      expect(handle.getAttribute('aria-valuenow')).toBe(String(THREAD_PANE_MAX_WIDTH));
    });
    expect(localStorage.getItem('chatto:threadPaneWidth')).toBe(String(THREAD_PANE_MAX_WIDTH));
  });

  it('uses the overlay by default and hides the resize handle', () => {
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    const pane = q(container, '[data-testid="thread-pane"]') as HTMLElement;
    expect(pane.className).toContain('absolute');
    expect(pane.className).toContain('inline-end-overlay-shadow');
    expect(pane.className).toContain('lg:w-[90%]');
    expect(pane.className).not.toContain('sm:w-[90%]');
    expect(container.querySelector('[role="slider"]')).toBeNull();
  });

  it('marks the thread as read through its content cursor', async () => {
    render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    await vi.waitFor(() =>
      expect(mocks.markThreadAsRead).toHaveBeenCalledWith(
        {
          roomId: 'room-1',
          threadRootEventId: 'thread-root',
          upToEventId: undefined
        },
        { signal: expect.any(AbortSignal) }
      )
    );

    expect(mocks.setThread).toHaveBeenCalledWith('room-1', 'thread-root');
  });

  it('forwards unread marker state and bottom arrival to EventList', () => {
    mocks.unreadMarkerEventId = 'thread-unread';
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    expect(
      (q(container, '[data-testid="event-list-unread-after"]') as HTMLOutputElement).textContent
    ).toBe('thread-unread');

    (q(container, '[data-testid="event-list-reached-bottom"]') as HTMLButtonElement).click();
    expect(mocks.clearUnreadMarker).toHaveBeenCalledOnce();
  });

  it('retains decrypted thread history only for the mounted pane lifetime', async () => {
    const rendered = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    await vi.waitFor(() => expect(mocks.retainMessagesForThread).toHaveBeenCalledOnce());
    const mountedStore = mocks.threadStore;
    expect(mocks.retainMessagesForThread).toHaveBeenCalledWith(
      'room-1',
      'thread-root',
      mountedStore
    );

    rendered.unmount();
    expect(mocks.releaseMessagesForThread).toHaveBeenCalledWith(
      'room-1',
      'thread-root',
      mountedStore
    );
  });

  it('releases decrypted thread history through its owning server store', async () => {
    const rendered = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    await vi.waitFor(() => expect(mocks.retainMessagesForThread).toHaveBeenCalledOnce());
    const firstServerStore = mocks.threadStore;

    scopeState.set('serverId', 'server-2');

    await vi.waitFor(() => expect(mocks.nextServerRetainMessagesForThread).toHaveBeenCalledOnce());
    expect(mocks.releaseMessagesForThread).toHaveBeenCalledWith(
      'room-1',
      'thread-root',
      firstServerStore
    );
    expect(mocks.nextServerReleaseMessagesForThread).not.toHaveBeenCalled();

    rendered.unmount();
    expect(mocks.nextServerReleaseMessagesForThread).toHaveBeenCalledWith(
      'room-1',
      'thread-root',
      mocks.nextServerThreadStore
    );
  });

  it('loads a highlighted reply outside the latest thread page before jumping to it', async () => {
    let resolveRefresh!: (result: {
      hasOlder: boolean;
      hasNewer: boolean;
      refreshed: boolean;
      changed: boolean;
    }) => void;
    mocks.refreshCurrentWindow.mockReturnValue(
      new Promise((resolve) => {
        resolveRefresh = resolve;
      })
    );

    render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        highlightEventId: 'older-reply',
        onClose: mocks.onClose
      }
    });

    await vi.waitFor(() => expect(mocks.refreshCurrentWindow).toHaveBeenCalledWith('older-reply'));
    expect(mocks.jumpToMessage).not.toHaveBeenCalled();

    resolveRefresh({
      hasOlder: true,
      hasNewer: true,
      refreshed: true,
      changed: true
    });

    await vi.waitFor(() => {
      expect(mocks.jumpToMessage).toHaveBeenCalledWith('older-reply');
    });
  });

  it('updates the thread follow button optimistically while the RPC is pending', async () => {
    let resolveFollow!: (value: {
      following: boolean;
      state: { roomId: string; threadRootEventId: string; following: boolean };
    }) => void;
    mocks.followThread.mockReturnValue(
      new Promise((resolve) => {
        resolveFollow = resolve;
      })
    );

    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    (q(container, 'button[aria-label="Follow thread"]') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Unfollow thread"]')).toBeTruthy();
    });
    expect(
      (q(container, 'button[aria-label="Unfollow thread"]') as HTMLButtonElement).disabled
    ).toBe(true);
    expect(mocks.followThread).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 'thread-root'
    });

    resolveFollow({
      following: true,
      state: { roomId: 'room-1', threadRootEventId: 'thread-root', following: true }
    });

    await vi.waitFor(() => {
      expect(
        (q(container, 'button[aria-label="Unfollow thread"]') as HTMLButtonElement).disabled
      ).toBe(false);
    });
  });

  it('seeds follow state when the lazy thread root arrives after mount', async () => {
    mocks.threadStore!.isInitialLoading = true;
    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    expect(q(container, 'button[aria-label="Follow thread"]')).toBeTruthy();

    mocks.threadStore!.threadEvents = [
      {
        id: 'thread-root',
        createdAt: '2026-07-17T12:00:00Z',
        actorId: 'test-user',
        actor: null,
        event: {
          kind: TimelineEventKind.MessagePosted,
          roomId: 'room-1',
          body: 'Thread root',
          attachments: [],
          linkPreview: null,
          updatedAt: null,
          inReplyTo: null,
          threadRootEventId: null,
          echoOfEventId: null,
          echoFromThreadRootEventId: null,
          channelEchoEventId: null,
          replyCount: 1,
          lastReplyAt: '2026-07-17T12:01:00Z',
          threadParticipants: [],
          viewerIsFollowingThread: true,
          reactions: []
        }
      }
    ];
    mocks.threadStore!.isInitialLoading = false;

    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Unfollow thread"]')).toBeTruthy();
    });
  });

  it('ignores another follow toggle while the first request is pending', async () => {
    let rejectFollow!: (error: Error) => void;
    mocks.followThread.mockReturnValue(
      new Promise((_, reject) => {
        rejectFollow = reject;
      })
    );

    const { container } = render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    (q(container, 'button[aria-label="Follow thread"]') as HTMLButtonElement).click();
    await vi.waitFor(() => {
      expect(q(container, 'button[aria-label="Unfollow thread"]')).toBeTruthy();
    });
    const pendingButton = q(container, 'button[aria-label="Unfollow thread"]') as HTMLButtonElement;
    pendingButton.click();

    expect(pendingButton.disabled).toBe(true);
    expect(mocks.followThread).toHaveBeenCalledOnce();
    expect(mocks.unfollowThread).not.toHaveBeenCalled();

    rejectFollow(new Error('request failed'));

    await vi.waitFor(() => {
      expect(
        (q(container, 'button[aria-label="Follow thread"]') as HTMLButtonElement).disabled
      ).toBe(false);
    });
  });
});
