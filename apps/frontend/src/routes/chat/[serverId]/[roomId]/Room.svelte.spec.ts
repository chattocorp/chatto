import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
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
      timeline: {
        getRoomEvents: vi.fn(),
        getRoomEventsAround: vi.fn(),
        resolveMessageLinkTarget: vi.fn(),
        getThreadEvents: vi.fn(),
        getThreadEventsAround: vi.fn()
      },
      livekitUrl: null as string | null,
      roomKind: 1,
      activeCallRoomIds: new Set<string>(),
      joinedCallRoomIds: new Set<string>(),
      notifications: {
        notifications: [] as Array<{ id: string }>,
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
        type: mocks.roomKind,
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
    isDM: mocks.roomKind === RoomKind.DM,
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
    serverId: 'server-1',
    connectBaseUrl: 'http://localhost/api/connect',
    bearerToken: null,
    client: {
      query: mocks.query,
      mutation: mocks.mutation,
      subscription: mocks.subscription
    }
  })
}));

vi.mock('@chatto/api-client/roomTimeline', () => ({
  createRoomTimelineAPI: () => mocks.timeline
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
        has: vi.fn((roomId: string) => mocks.activeCallRoomIds.has(roomId))
      },
      voiceCall: {
        isInCall: vi.fn((roomId: string) => mocks.joinedCallRoomIds.has(roomId))
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
  const { default: MessageComposerMock } =
    await import('./RoomLocalEchoMessageComposerMock.svelte');
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
  const { default: RoomSidebarMock } = await import('./RoomLocalEchoRoomSidebarMock.svelte');
  return { default: RoomSidebarMock };
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

function emptyTimelinePage() {
  return {
    events: [],
    startCursor: null,
    endCursor: null,
    hasOlder: false,
    hasNewer: false
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  sessionStorage.clear();
  mocks.timeline.getRoomEvents.mockResolvedValue(emptyTimelinePage());
  mocks.timeline.getRoomEventsAround.mockResolvedValue(emptyTimelinePage());
  mocks.timeline.resolveMessageLinkTarget.mockResolvedValue({
    event: null,
    threadRootEventId: null
  });
  mocks.timeline.getThreadEvents.mockResolvedValue(emptyTimelinePage());
  mocks.timeline.getThreadEventsAround.mockResolvedValue(emptyTimelinePage());
  mocks.livekitUrl = null;
  mocks.roomKind = RoomKind.CHANNEL;
  mocks.activeCallRoomIds.clear();
  mocks.joinedCallRoomIds.clear();
  mocks.notifications.notifications = [];
  mocks.notifications.dismissDMNotifications.mockResolvedValue({ byRoom: {} });
  mocks.notifications.dismissMentionNotifications.mockResolvedValue({ byRoom: {} });
  mocks.notifications.dismissRoomReplyNotifications.mockResolvedValue({ byRoom: {} });
  mocks.notifications.dismissRoomMessageNotifications.mockResolvedValue({ byRoom: {} });
  mocks.rooms.refreshNotificationCounts.mockResolvedValue(undefined);
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({ matches: true }))
  );
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

  it('clears pending in-room reply state when the room changes', async () => {
    const rendered = render(Room, { props: { roomId: 'room-1' } });
    const { container } = rendered;

    await expect
      .element(q(container, '[data-testid="composer-in-reply-to"]'))
      .toHaveTextContent('');

    (q(container, '[data-testid="start-composer-reply"]') as HTMLButtonElement).click();
    await expect
      .element(q(container, '[data-testid="composer-in-reply-to"]'))
      .toHaveTextContent('reply-target');

    await rendered.rerender({ roomId: 'room-2' });

    await expect
      .element(q(container, '[data-testid="composer-in-reply-to"]'))
      .toHaveTextContent('');
  });

  it('opens a pending call panel request as a mobile sidebar after navigation', async () => {
    mocks.livekitUrl = 'wss://livekit.example.test';
    vi.stubGlobal(
      'matchMedia',
      vi.fn(() => ({ matches: false }))
    );
    setPendingRoomSidebarPanel('server-1', 'room-1', 'call');

    const { container } = render(Room, { props: { roomId: 'room-1' } });

    await expect
      .element(q(container, '[data-testid="room-sidebar-mobile-pane"]'))
      .toBeInTheDocument();
    expect(consumePendingRoomSidebarPanel('server-1', 'room-1')).toBeNull();
  });

  it('lets a maximized desktop call sidebar fill the room route content area', async () => {
    mocks.livekitUrl = 'wss://livekit.example.test';
    mocks.activeCallRoomIds.add('room-1');
    setPendingRoomSidebarPanel('server-1', 'room-1', 'call');

    const { container } = render(Room, { props: { roomId: 'room-1' } });

    const roomRegion = q(container, '[data-testid="room-view-region"]')!;
    const desktopSidebarPane = q(container, '[data-testid="room-sidebar-desktop-pane"]')!;
    const maximizeButton = q(container, '[data-testid="toggle-maximized-call"]') as HTMLButtonElement;

    await expect.element(desktopSidebarPane).toBeInTheDocument();
    expect(roomRegion.className).not.toContain('lg:hidden');
    expect(desktopSidebarPane.className).toContain('shrink-0');

    maximizeButton.click();

    await expect.element(maximizeButton).toHaveAttribute('data-maximized', 'true');
    expect(roomRegion.className).toContain('lg:hidden');
    expect(desktopSidebarPane.className).toContain('flex-1');
    expect(desktopSidebarPane.className).not.toContain('shrink-0');
  });

  it('restores the room view when a maximized desktop call ends', async () => {
    mocks.livekitUrl = 'wss://livekit.example.test';
    mocks.activeCallRoomIds.add('room-1');
    setPendingRoomSidebarPanel('server-1', 'room-1', 'call');

    const rendered = render(Room, { props: { roomId: 'room-1' } });
    const { container } = rendered;

    const roomRegion = q(container, '[data-testid="room-view-region"]')!;
    const desktopSidebarPane = q(container, '[data-testid="room-sidebar-desktop-pane"]')!;
    const maximizeButton = q(container, '[data-testid="toggle-maximized-call"]') as HTMLButtonElement;

    maximizeButton.click();

    await expect.element(maximizeButton).toHaveAttribute('data-maximized', 'true');
    expect(roomRegion.className).toContain('lg:hidden');
    expect(desktopSidebarPane.className).toContain('flex-1');

    mocks.activeCallRoomIds.clear();
    await rendered.rerender({ roomId: 'room-1' });

    await expect.element(maximizeButton).toHaveAttribute('data-maximized', 'false');
    expect(roomRegion.className).not.toContain('lg:hidden');
    expect(desktopSidebarPane.className).toContain('shrink-0');
    expect(desktopSidebarPane.className).not.toContain('flex-1');
  });

  it('refreshes room notification counts after active-room notifications auto-dismiss', async () => {
    mocks.notifications.dismissMentionNotifications.mockResolvedValue({ byRoom: { 'room-1': 1 } });

    render(Room, { props: { roomId: 'room-1' } });

    await vi.waitFor(() => {
      expect(mocks.rooms.decrementUnreadNotification).toHaveBeenCalledWith('room-1', 1);
      expect(mocks.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
    });
  });

  it('dismisses active DM notifications and refreshes room notification counts', async () => {
    mocks.roomKind = RoomKind.DM;
    mocks.notifications.dismissDMNotifications.mockResolvedValue({ byRoom: { 'room-1': 2 } });

    render(Room, { props: { roomId: 'room-1' } });

    await vi.waitFor(() => {
      expect(mocks.notifications.dismissDMNotifications).toHaveBeenCalledWith('room-1');
      expect(mocks.notifications.dismissMentionNotifications).not.toHaveBeenCalled();
      expect(mocks.rooms.decrementUnreadNotification).toHaveBeenCalledWith('room-1', 2);
      expect(mocks.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
    });
  });

  it('refreshes the visible room window after a local link-preview deletion succeeds', async () => {
    const { container } = render(Room, { props: { roomId: 'room-1' } });

    await expect.element(q(container, '[data-testid="room-event-ids"]')).toHaveTextContent('');
    (q(container, '[data-testid="emit-returned-post"]') as HTMLButtonElement).click();
    await expect
      .element(q(container, '[data-testid="room-event-ids"]'))
      .toHaveTextContent('msg-local');
    await vi.waitFor(() => expect(mocks.timeline.getRoomEvents).toHaveBeenCalled());
    mocks.timeline.getRoomEventsAround.mockClear();

    window.dispatchEvent(
      new CustomEvent('chatto:room-message-mutated', {
        detail: {
          roomId: 'room-1',
          eventId: 'msg-local',
          reason: 'link-preview-deleted'
        }
      })
    );

    await vi.waitFor(() => {
      expect(mocks.timeline.getRoomEventsAround).toHaveBeenCalledWith({
        roomId: 'room-1',
        eventId: 'msg-local',
        limit: 50
      });
    });
  });

  it('refreshes a visible channel echo when a local mutation references the original message', async () => {
    const { container } = render(Room, { props: { roomId: 'room-1' } });

    await expect.element(q(container, '[data-testid="room-event-ids"]')).toHaveTextContent('');
    (q(container, '[data-testid="emit-returned-echo"]') as HTMLButtonElement).click();
    await expect
      .element(q(container, '[data-testid="room-event-ids"]'))
      .toHaveTextContent('echo-local');
    await vi.waitFor(() => expect(mocks.timeline.getRoomEvents).toHaveBeenCalled());
    mocks.timeline.getRoomEventsAround.mockClear();

    window.dispatchEvent(
      new CustomEvent('chatto:room-message-mutated', {
        detail: {
          roomId: 'room-1',
          eventId: 'original-reply',
          reason: 'attachment-deleted'
        }
      })
    );

    await vi.waitFor(() => {
      expect(mocks.timeline.getRoomEventsAround).toHaveBeenCalledWith({
        roomId: 'room-1',
        eventId: 'echo-local',
        limit: 50
      });
    });
  });

  it('removes a deleted visible channel echo without refreshing around the hidden echo', async () => {
    const { container } = render(Room, { props: { roomId: 'room-1' } });

    await expect.element(q(container, '[data-testid="room-event-ids"]')).toHaveTextContent('');
    (q(container, '[data-testid="emit-returned-echo"]') as HTMLButtonElement).click();
    await expect
      .element(q(container, '[data-testid="room-event-ids"]'))
      .toHaveTextContent('echo-local');
    await vi.waitFor(() => expect(mocks.timeline.getRoomEvents).toHaveBeenCalled());
    mocks.timeline.getRoomEventsAround.mockClear();

    window.dispatchEvent(
      new CustomEvent('chatto:room-message-mutated', {
        detail: {
          roomId: 'room-1',
          eventId: 'echo-local',
          reason: 'message-deleted'
        }
      })
    );

    await expect.element(q(container, '[data-testid="room-event-ids"]')).toHaveTextContent('');
    expect(mocks.timeline.getRoomEventsAround).not.toHaveBeenCalled();
  });
});
