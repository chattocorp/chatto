import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import {
  consumePendingRoomSidebarPanel,
  setPendingRoomSidebarPanel
} from '$lib/storage/roomSidebarPanel';

const { mocks } = vi.hoisted(() => {
  const queryData = {
    server: { roles: [] },
    room: {
      events: {
        events: [],
        startCursor: null,
        endCursor: null,
        hasOlder: false,
        hasNewer: false
      },
      members: {
        users: [],
        totalCount: 0,
        hasMore: false
      }
    }
  };

  return {
    mocks: {
      goto: vi.fn(),
      pushState: vi.fn(),
      replaceState: vi.fn(),
      noteReadCursor: vi.fn(),
      noteAwayEvent: vi.fn(),
      markRoomAsRead: vi.fn(),
      resetTypingDebounce: vi.fn(),
      query: vi.fn(() => ({
        toPromise: vi.fn().mockResolvedValue({ data: queryData, error: null })
      })),
      mutation: vi.fn(() => ({
        toPromise: vi.fn().mockResolvedValue({ data: {}, error: null })
      })),
      subscription: vi.fn(),
      livekitUrl: null as string | null,
      notifications: {
        dismissDMNotifications: vi.fn().mockResolvedValue({ byRoom: {} }),
        dismissMentionNotifications: vi.fn().mockResolvedValue({ byRoom: {} }),
        dismissRoomReplyNotifications: vi.fn().mockResolvedValue({ byRoom: {} }),
        dismissRoomMessageNotifications: vi.fn().mockResolvedValue({ byRoom: {} })
      },
      rooms: {
        decrementUnreadNotification: vi.fn(),
        refreshNotificationCounts: vi.fn().mockResolvedValue(undefined)
      }
    }
  };
});

vi.mock('$app/state', () => ({
  page: {
    params: { serverId: '-', roomId: 'room-1' },
    state: {},
    url: new URL('https://chat.example.test/chat/-/room-1')
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: mocks.pushState,
  replaceState: mocks.replaceState
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
      .replace('[threadId]', params?.threadId ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: () => '-',
  segmentToServerId: () => 'server-1'
}));

vi.mock('$lib/hooks', () => ({
  useRoomData: () => ({
    roomData: {
      room: {
        id: 'room-1',
        name: 'general',
        description: 'Room description',
        type: 'CHANNEL',
        isUniversal: false
      },
      spaceName: 'Test Space',
      canPostMessage: true,
      canPostInThread: true,
      canAttach: false,
      canReact: true,
      canManageOthersMessage: false,
      canEchoMessage: true,
      canManageRoom: false,
      canBanRoomMembers: false
    },
    dmData: null,
    isDM: false,
    isRoomLoading: false
  }),
  useRoomUnread: () => ({
    unreadAfterTime: null,
    unreadBeforeTime: null,
    markRoomAsRead: mocks.markRoomAsRead,
    noteReadCursor: mocks.noteReadCursor,
    noteAwayEvent: mocks.noteAwayEvent
  }),
  useEvent: vi.fn(),
  usePresenceChange: vi.fn(),
  createTypingIndicator: () => ({
    userIds: [],
    sendTypingIndicator: vi.fn(),
    resetDebounce: mocks.resetTypingDebounce,
    removeTypingUser: vi.fn()
  })
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    isConnected: true,
    showConnectionLostBanner: false,
    client: {
      query: mocks.query,
      mutation: mocks.mutation,
      subscription: mocks.subscription
    }
  })
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      currentUser: { user: { id: 'test-user', login: 'testuser' }, loading: false },
      serverInfo: {
        livekitUrl: mocks.livekitUrl,
        videoProcessingEnabled: false,
        maxUploadSize: 25 * 1024 * 1024,
        maxVideoUploadSize: 25 * 1024 * 1024
      },
      notifications: mocks.notifications,
      pendingHighlights: {
        consume: vi.fn(() => null)
      },
      activeCallRooms: {
        has: vi.fn(() => false)
      },
      rooms: mocks.rooms
    }),
    originServer: { id: 'server-1', url: 'https://chat.example.test' },
    getServer: () => ({ id: 'server-1', url: 'https://chat.example.test' })
  }
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server-1'
}));

vi.mock('$lib/state/globals.svelte', () => ({
  appState: {
    isFocused: true,
    isPresent: true
  }
}));

vi.mock('$lib/storage/lastRoom', () => ({
  clearLastRoom: vi.fn(),
  setLastRoom: vi.fn()
}));

vi.mock('$lib/attachments/dropZone.svelte', () => ({
  dropZone: vi.fn()
}));

vi.mock('$lib/components/composer/MessageComposer.svelte', async () => {
  const { default: MessageComposerMock } = await import('./RoomLocalEchoMessageComposerMock.svelte');
  return { default: MessageComposerMock };
});

vi.mock('./RoomEventsPane.svelte', async () => {
  const { default: RoomEventsPaneMock } = await import('./RoomLocalEchoRoomEventsPaneMock.svelte');
  return { default: RoomEventsPaneMock };
});

vi.mock('./ThreadPane.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

vi.mock('./RoomSidebar.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

vi.mock('./RoomSidebarToggle.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

vi.mock('$lib/attachments/DropZoneOverlay.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

vi.mock('$lib/components/voice/VoiceCallButton.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

vi.mock('$lib/components/voice/VoiceCallPanel.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

vi.mock('$lib/ui/PageTitle.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

vi.mock('$lib/ui/PaneHeader.svelte', async () => {
  const { default: EmptyMock } = await import('./RoomLocalEchoEmptyMock.svelte');
  return { default: EmptyMock };
});

import Room from './Room.svelte';

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  mocks.livekitUrl = null;
  mocks.notifications.dismissDMNotifications.mockResolvedValue({ byRoom: {} });
  mocks.notifications.dismissMentionNotifications.mockResolvedValue({ byRoom: {} });
  mocks.notifications.dismissRoomReplyNotifications.mockResolvedValue({ byRoom: {} });
  mocks.notifications.dismissRoomMessageNotifications.mockResolvedValue({ byRoom: {} });
  mocks.rooms.refreshNotificationCounts.mockResolvedValue(undefined);
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true })));
});

describe('Room local message echo', () => {
  it('inserts a returned main-room post into the same store rendered by the room timeline', async () => {
    const { container } = render(Room, { props: { roomId: 'room-1' } });

    await expect.element(q(container, '[data-testid="room-event-ids"]')).toHaveTextContent('');

    (q(container, '[data-testid="emit-returned-post"]') as HTMLButtonElement).click();

    await expect
      .element(q(container, '[data-testid="room-event-ids"]'))
      .toHaveTextContent('msg-local');
    expect(mocks.resetTypingDebounce).toHaveBeenCalledOnce();
    expect(mocks.noteReadCursor).toHaveBeenCalledWith('2026-06-17T10:47:00Z');
  });

  it('does not advance the current room read cursor for a stale returned post from another room', async () => {
    const { container } = render(Room, { props: { roomId: 'room-2' } });

    await expect.element(q(container, '[data-testid="room-event-ids"]')).toHaveTextContent('');

    (q(container, '[data-testid="emit-returned-post"]') as HTMLButtonElement).click();

    await expect.element(q(container, '[data-testid="room-event-ids"]')).toHaveTextContent('');
    expect(mocks.resetTypingDebounce).toHaveBeenCalledOnce();
    expect(mocks.noteReadCursor).not.toHaveBeenCalled();
  });

  it('opens a pending call panel request as a mobile sidebar after navigation', async () => {
    mocks.livekitUrl = 'wss://livekit.example.test';
    vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false })));
    setPendingRoomSidebarPanel('server-1', 'room-1', 'call');

    const { container } = render(Room, { props: { roomId: 'room-1' } });

    await expect.element(q(container, '[data-testid="room-sidebar-mobile-pane"]')).toBeInTheDocument();
    expect(consumePendingRoomSidebarPanel('server-1', 'room-1')).toBeNull();
  });

  it('refreshes room notification counts after active-room notifications auto-dismiss', async () => {
    mocks.notifications.dismissMentionNotifications.mockResolvedValue({ byRoom: { 'room-1': 1 } });

    render(Room, { props: { roomId: 'room-1' } });

    await vi.waitFor(() => {
      expect(mocks.rooms.decrementUnreadNotification).toHaveBeenCalledWith('room-1', 1);
      expect(mocks.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
    });
  });
});
