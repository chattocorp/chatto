import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ThreadPane from './ThreadPane.svelte';

const { mocks } = vi.hoisted(() => {
  return {
    mocks: {
      markThreadAsRead: vi.fn(),
      followThread: vi.fn(),
      unfollowThread: vi.fn(),
      setThread: vi.fn(),
      disposeMessagesStore: vi.fn(),
      ingestServerEvent: vi.fn(),
      ingestEvent: vi.fn(),
      refreshCurrentWindow: vi.fn(),
      loadMore: vi.fn(),
      applyLocalMessageDeletion: vi.fn(),
      refreshAnchorForMessageMutation: vi.fn(),
      removeTypingUser: vi.fn(),
      sendTypingIndicator: vi.fn(),
      resetTypingDebounce: vi.fn(),
      onClose: vi.fn(),
      notifications: {
        dismissThreadNotifications: vi.fn().mockResolvedValue({ byRoom: {} })
      },
      rooms: {
        decrementUnreadNotification: vi.fn()
      },
      appState: {
        isPresent: true
      }
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
  useEvent: vi.fn(),
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
      rooms: mocks.rooms
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
      jumpToMessage: vi.fn()
    }
  }),
  MessagesStore: class {
    threadEvents = [];
    isInitialLoading = false;
    isLoadingMore = false;
    hasReachedStart = true;
    setThread = mocks.setThread;
    dispose = mocks.disposeMessagesStore;
    ingestServerEvent = mocks.ingestServerEvent;
    ingestEvent = mocks.ingestEvent;
    refreshCurrentWindow = mocks.refreshCurrentWindow;
    loadMore = mocks.loadMore;
    applyLocalMessageDeletion = mocks.applyLocalMessageDeletion;
    refreshAnchorForMessageMutation = mocks.refreshAnchorForMessageMutation;
  }
}));

vi.mock('$lib/state/room/messageMutationEvents', () => ({
  onRoomMessageMutated: vi.fn(() => vi.fn())
}));

vi.mock('$lib/eventBus.svelte', () => ({
  onThreadFollowChanged: vi.fn(() => vi.fn())
}));

vi.mock('./EventList.svelte', async () => {
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
    mocks.appState.isPresent = true;
    mocks.markThreadAsRead.mockResolvedValue({
      previousReadAt: null,
      lastReadAt: '2026-07-04T13:00:00Z'
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
});
