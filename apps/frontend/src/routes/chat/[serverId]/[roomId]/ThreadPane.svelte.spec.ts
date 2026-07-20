import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { RoomEventKind } from '$lib/render/eventKinds';
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
      notifications: {
        dismissThreadNotifications: vi.fn().mockResolvedValue({ byRoom: {} })
      },
      rooms: {
        decrementUnreadNotification: vi.fn()
      },
      appState: {
        isPresent: true
      },
      threadStore: null as ThreadPaneTestStore | null
    }
  };
});

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
    options: { markAsRead: (targetId: string, upToEventId?: string) => unknown }
  ) => {
    void options.markAsRead(getTargetId());
    return {
      unreadMarkerEventId: null,
      unreadMarkerWindow: null,
      markAsRead: options.markAsRead,
      setUnreadMarkerEventId: vi.fn(),
      clearUnreadMarker: vi.fn()
    };
  },
  createTypingIndicator: () => ({
    userIds: [],
    removeTypingUser: mocks.removeTypingUser,
    sendTypingIndicator: mocks.sendTypingIndicator,
    resetDebounce: mocks.resetTypingDebounce
  })
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    serverId: 'server-1',
    connectBaseUrl: 'http://localhost/api/connect',
    bearerToken: null
  })
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      currentUser: { user: { id: 'test-user', login: 'testuser' }, loading: false },
      notifications: mocks.notifications,
      rooms: mocks.rooms,
      retainMessagesForThread: mocks.retainMessagesForThread,
      releaseMessagesForThread: mocks.releaseMessagesForThread,
      messagesForThread: () =>
        Object.assign(mocks.threadStore!, {
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

vi.mock('./TimelineEventsPane.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

vi.mock('$lib/components/composer/MessageComposer.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

describe('ThreadPane', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.threadStore = new ThreadPaneTestStore();
    mocks.appState.isPresent = true;
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

  it('marks the thread as read without directly dismissing thread notifications', async () => {
    render(ThreadPane, {
      props: {
        roomId: 'room-1',
        roomName: 'General',
        threadRootEventId: 'thread-root',
        onClose: mocks.onClose
      }
    });

    await vi.waitFor(() =>
      expect(mocks.markThreadAsRead).toHaveBeenCalledWith({
        roomId: 'room-1',
        threadRootEventId: 'thread-root',
        upToEventId: undefined
      })
    );

    expect(mocks.setThread).toHaveBeenCalledWith('room-1', 'thread-root');
    expect(mocks.notifications.dismissThreadNotifications).not.toHaveBeenCalled();
    expect(mocks.rooms.decrementUnreadNotification).not.toHaveBeenCalled();
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
          kind: RoomEventKind.MessagePosted,
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
