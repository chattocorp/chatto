import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { RoomType } from '$lib/gql/graphql';
import { serverStorageKey } from '$lib/storage/serverStorage';
import {
  consumePendingRoomSidebarPanel,
  roomSidebarPanelStorageSuffix
} from '$lib/storage/roomSidebarPanel';
import type { RoomsListGroup } from '$lib/state/server/rooms.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    activeCallRoomIds: new Set<string>(),
    callParticipants: new Map<string, unknown[]>(),
    pushState: vi.fn(),
    goto: vi.fn(),
    store: {
      currentUser: { user: { id: 'me' } },
      notifications: {
        hasDMRoomNotification: vi.fn().mockReturnValue(false),
        hasRoomNotification: vi.fn().mockReturnValue(false),
        getDMRoomNotification: vi.fn().mockReturnValue(null),
        getRoomNotification: vi.fn().mockReturnValue(null),
        fetchRoomNotification: vi.fn().mockResolvedValue({
          ok: true,
          totalCount: 0,
          notification: null
        }),
        resolveRoomNotification: vi.fn().mockResolvedValue({
          ok: true,
          totalCount: 0,
          notification: null
        }),
        dismiss: vi.fn(),
        getCleanPath: vi.fn().mockReturnValue('/chat/-/room')
      },
      notificationLevels: {
        isRoomMuted: vi.fn().mockReturnValue(false)
      },
      activeCallRooms: {
        load: vi.fn().mockResolvedValue(undefined),
        has: vi.fn((roomId: string) => mocks.activeCallRoomIds.has(roomId)),
        getParticipants: vi.fn((roomId: string) => mocks.callParticipants.get(roomId) ?? []),
        handleJoin: vi.fn(),
        handleLeave: vi.fn(),
        handleEnd: vi.fn()
      },
      voiceCall: {
        join: vi.fn().mockResolvedValue(undefined),
        handleParticipantLeftEvent: vi.fn(),
        handleCallEndedEvent: vi.fn()
      },
      serverInfo: {
        livekitUrl: null
      },
      rooms: {
        rooms: [],
        roomGroups: null as RoomsListGroup[] | null,
        isInitialLoading: false,
        currentUserId: 'me',
        markRead: vi.fn(),
        bumpRoom: vi.fn(),
        setUnread: vi.fn(),
        clearUnreadNotifications: vi.fn(),
        decrementUnreadNotification: vi.fn(),
        incrementUnreadNotification: vi.fn(),
        refreshNotificationCounts: vi.fn().mockResolvedValue(undefined)
      },
      pendingHighlights: {
        set: vi.fn()
      },
      handleVoiceCallJoinFailed: vi.fn()
    }
  }
}));

vi.mock('$app/state', () => ({
  page: {
    params: {
      serverId: '-',
      roomId: undefined
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: mocks.pushState
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path.replace('[serverId]', params?.serverId ?? '').replace('[roomId]', params?.roomId ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: () => '-',
  segmentToServerId: () => 'origin'
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: vi.fn(() => mocks.store),
    isOriginServer: vi.fn(() => true),
    getServer: vi.fn(() => ({ id: 'origin', url: 'https://chat.example.test' })),
    originServer: { id: 'origin' },
    servers: [{ id: 'origin', url: 'https://chat.example.test' }]
  }
}));

vi.mock('$lib/hooks', () => ({
  useEvent: vi.fn(),
  useRoomMarkedAsRead: vi.fn(),
  useTabResumeCallback: vi.fn()
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_userId: string, fallback: string) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback
}));

import RoomList from './RoomList.svelte';

function notification(id: string, roomId: string, isDM = false) {
  if (isDM) {
    return {
      __typename: 'DMMessageNotificationItem',
      id,
      createdAt: '2026-06-18T10:00:00Z',
      actor: null,
      summary: 'new direct message',
      room: { id: roomId }
    };
  }

  return {
    __typename: 'MentionNotificationItem',
    id,
    createdAt: '2026-06-18T10:00:00Z',
    actor: null,
    summary: 'mentioned you',
    mentionRoom: { id: roomId, name: 'general' },
    mentionEventId: 'event-1',
    mentionInThread: 'thread-1'
  };
}

function user(id: string, login: string, displayName: string) {
  return {
    id,
    login,
    displayName,
    avatarUrl: null,
    presenceStatus: 'ONLINE'
  };
}

function setRooms() {
  mocks.store.rooms.rooms = [
    {
      id: 'channel-1',
      name: 'general',
      type: RoomType.Channel,
      isUniversal: false,
      hasUnread: false,
      viewerIsMember: true,
      viewerCanJoinRoom: true,
      viewerNotificationCount: 0,
      members: []
    },
    {
      id: 'joinable-channel',
      name: 'joinable',
      type: RoomType.Channel,
      isUniversal: false,
      hasUnread: false,
      viewerIsMember: false,
      viewerCanJoinRoom: true,
      viewerNotificationCount: 0,
      members: []
    },
    {
      id: 'restricted-channel',
      name: 'restricted',
      type: RoomType.Channel,
      isUniversal: false,
      hasUnread: false,
      viewerIsMember: false,
      viewerCanJoinRoom: false,
      viewerNotificationCount: 0,
      members: []
    },
    {
      id: 'dm-with-participants',
      name: '',
      type: RoomType.Dm,
      isUniversal: false,
      hasUnread: false,
      viewerIsMember: true,
      viewerCanJoinRoom: true,
      viewerNotificationCount: 0,
      members: [user('me', 'me', 'Me'), user('teal', 'teal', 'Teal')]
    },
    {
      id: 'dm-phone-only',
      name: '',
      type: RoomType.Dm,
      isUniversal: false,
      hasUnread: false,
      viewerIsMember: true,
      viewerCanJoinRoom: true,
      viewerNotificationCount: 0,
      members: [user('me', 'me', 'Me'), user('river', 'river', 'River')]
    }
  ] as never;
}

function setRoomNotificationCount(roomId: string, count: number) {
  const rooms = mocks.store.rooms.rooms as Array<{
    id: string;
    viewerNotificationCount: number;
  }>;
  const room = rooms.find((item) => item.id === roomId);
  if (!room) throw new Error(`Missing mocked room ${roomId}`);
  room.viewerNotificationCount = count;
}

function setRoomUnread(roomId: string, hasUnread: boolean) {
  const rooms = mocks.store.rooms.rooms as Array<{
    id: string;
    hasUnread: boolean;
  }>;
  const room = rooms.find((item) => item.id === roomId);
  if (!room) throw new Error(`Missing mocked room ${roomId}`);
  room.hasUnread = hasUnread;
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  mocks.activeCallRoomIds = new Set();
  mocks.callParticipants = new Map();
  mocks.store.rooms.roomGroups = null;
  mocks.store.rooms.isInitialLoading = false;
  mocks.store.rooms.currentUserId = 'me';
  setRooms();
  vi.clearAllMocks();
  mocks.store.notifications.fetchRoomNotification.mockResolvedValue({
    ok: true,
    totalCount: 0,
    notification: null
  });
  mocks.store.notifications.resolveRoomNotification.mockResolvedValue({
    ok: true,
    totalCount: 0,
    notification: null
  });
  mocks.store.notifications.getCleanPath.mockReturnValue('/chat/-/room');
  mocks.store.rooms.refreshNotificationCounts.mockResolvedValue(undefined);
});

describe('RoomList', () => {
  it('renders active-call DM rows with the pulse icon and participant avatars', async () => {
    mocks.activeCallRoomIds.add('dm-with-participants');
    mocks.callParticipants.set('dm-with-participants', [
      {
        userId: 'teal',
        login: 'teal',
        displayName: 'Teal',
        avatarUrl: null
      }
    ]);

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/dm-with-participants"]')).toBeInTheDocument();
    const dmRow = q(container, '[href="/chat/-/dm-with-participants"]');
    const icon = dmRow?.querySelector('[data-testid="room-call-icon"]');
    const pulseIcon = icon?.querySelector('[data-testid="active-call-pulse-icon"]');
    const children = Array.from(dmRow?.children ?? []);
    expect(icon).not.toBeNull();
    expect(icon?.classList.contains('text-accent')).toBe(true);
    expect(icon?.querySelector('.uil--phone')).not.toBeNull();
    expect(pulseIcon).not.toBeNull();
    expect(pulseIcon?.classList.contains('animate-ping')).toBe(true);
    expect(dmRow?.querySelector('[data-testid="room-call-participants"]')).not.toBeNull();
    expect(dmRow?.querySelectorAll('[data-testid="room-call-participant-avatar"]')).toHaveLength(1);
    expect(children.indexOf(dmRow!.querySelector('[data-testid="room-call-participants"]')!)).toBe(
      children.indexOf(icon!) - 1
    );
    expect(children[0]?.querySelector('[data-testid="room-call-icon"]')).toBeNull();
  });

  it('renders the active-call phone icon when participants are not loaded', async () => {
    mocks.activeCallRoomIds.add('dm-phone-only');

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/dm-phone-only"]')).toBeInTheDocument();
    const dmRow = q(container, '[href="/chat/-/dm-phone-only"]');
    const icon = dmRow?.querySelector('[data-testid="room-call-icon"]');
    expect(icon).not.toBeNull();
    expect(icon?.querySelector('.uil--phone')).not.toBeNull();
    expect(icon?.querySelector('[data-testid="active-call-pulse-icon"]')).not.toBeNull();
    expect(dmRow?.querySelector('[data-testid="room-call-participants"]')).toBeNull();
  });

  it('renders active-call channel rows with the pulse icon and participant avatars', async () => {
    mocks.activeCallRoomIds.add('channel-1');
    mocks.callParticipants.set('channel-1', [
      {
        userId: 'teal',
        login: 'teal',
        displayName: 'Teal',
        avatarUrl: null
      }
    ]);

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/channel-1"]')).toBeInTheDocument();
    const channelRow = q(container, '[href="/chat/-/channel-1"]');
    const icon = channelRow?.querySelector('[data-testid="room-call-icon"]');
    const pulseIcon = icon?.querySelector('[data-testid="active-call-pulse-icon"]');
    const leadingIcon = channelRow?.querySelector('.sidebar-icon');
    const children = Array.from(channelRow?.children ?? []);
    expect(icon).not.toBeNull();
    expect(icon?.querySelector('.uil--phone')).not.toBeNull();
    expect(pulseIcon).not.toBeNull();
    expect(pulseIcon?.classList.contains('animate-ping')).toBe(true);
    expect(leadingIcon?.textContent).toBe('#');
    expect(leadingIcon).not.toBe(icon);
    expect(channelRow?.querySelector('[data-testid="room-call-participants"]')).not.toBeNull();
    expect(
      channelRow?.querySelectorAll('[data-testid="room-call-participant-avatar"]')
    ).toHaveLength(1);
    expect(
      children.indexOf(channelRow!.querySelector('[data-testid="room-call-participants"]')!)
    ).toBe(children.indexOf(icon!) - 1);
  });

  it('renders a compact overflow count for larger active calls', async () => {
    mocks.activeCallRoomIds.add('channel-1');
    mocks.callParticipants.set('channel-1', [
      { userId: 'teal', login: 'teal', displayName: 'Teal', avatarUrl: null },
      { userId: 'river', login: 'river', displayName: 'River', avatarUrl: null },
      { userId: 'sage', login: 'sage', displayName: 'Sage', avatarUrl: null },
      { userId: 'ash', login: 'ash', displayName: 'Ash', avatarUrl: null },
      { userId: 'sol', login: 'sol', displayName: 'Sol', avatarUrl: null },
      { userId: 'moon', login: 'moon', displayName: 'Moon', avatarUrl: null }
    ]);

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/channel-1"]')).toBeInTheDocument();
    const channelRow = q(container, '[href="/chat/-/channel-1"]');
    expect(
      channelRow?.querySelectorAll('[data-testid="room-call-participant-avatar"]')
    ).toHaveLength(4);
    await expect
      .element(q(channelRow!, '[data-testid="room-call-overflow"]'))
      .toHaveTextContent('+2');
  });

  it('opens the call panel when an active-call room icon is clicked', async () => {
    mocks.activeCallRoomIds.add('channel-1');

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/channel-1"]')).toBeInTheDocument();
    const channelRow = q(container, '[href="/chat/-/channel-1"]');
    const icon = channelRow?.querySelector('[data-testid="room-call-icon"]') as HTMLElement | null;
    expect(icon).not.toBeNull();

    icon!.click();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/channel-1');
    });
    expect(
      localStorage.getItem(serverStorageKey('origin', roomSidebarPanelStorageSuffix('channel-1')))
    ).toBe('call');
    expect(consumePendingRoomSidebarPanel('origin', 'channel-1')).toBe('call');
  });

  it('opens the call panel when an active-call DM icon is clicked', async () => {
    mocks.activeCallRoomIds.add('dm-with-participants');

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/dm-with-participants"]')).toBeInTheDocument();
    const dmRow = q(container, '[href="/chat/-/dm-with-participants"]');
    const icon = dmRow?.querySelector('[data-testid="room-call-icon"]') as HTMLElement | null;
    expect(icon).not.toBeNull();

    icon!.click();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-with-participants');
    });
    expect(
      localStorage.getItem(
        serverStorageKey('origin', roomSidebarPanelStorageSuffix('dm-with-participants'))
      )
    ).toBe('call');
    expect(consumePendingRoomSidebarPanel('origin', 'dm-with-participants')).toBe('call');
  });

  it.each([
    ['Enter', 'Enter'],
    ['Space', ' ']
  ])(
    'opens the call panel on %s when an active-call row has keyboard focus',
    async (_label, key) => {
      mocks.activeCallRoomIds.add('channel-1');

      const { container } = render(RoomList);

      await expect.element(q(container, '[href="/chat/-/channel-1"]')).toBeInTheDocument();
      const channelRow = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;

      const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true });
      const wasNotCanceled = channelRow.dispatchEvent(event);

      expect(wasNotCanceled).toBe(false);
      await vi.waitFor(() => {
        expect(mocks.goto).toHaveBeenCalledWith('/chat/-/channel-1');
      });
      expect(
        localStorage.getItem(serverStorageKey('origin', roomSidebarPanelStorageSuffix('channel-1')))
      ).toBe('call');
      expect(consumePendingRoomSidebarPanel('origin', 'channel-1')).toBe('call');
    }
  );

  it('opens a join modal for a faded joinable non-member channel row', async () => {
    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/joinable-channel"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    expect(row.className).toContain('opacity-60');

    row.click();

    expect(mocks.pushState).toHaveBeenCalledWith('', {
      modal: {
        type: 'joinRoom',
        roomId: 'joinable-channel',
        roomName: 'joinable',
        viewerCanJoinRoom: true
      }
    });
  });

  it('opens an access-info modal for a faded non-joinable channel row', async () => {
    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/restricted-channel"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    expect(row.className).toContain('opacity-60');
    const icon = row.querySelector('.sidebar-icon');
    expect(icon?.classList.contains('uil--lock')).toBe(true);
    expect(row.querySelectorAll('.uil--lock')).toHaveLength(1);

    row.click();

    expect(mocks.pushState).toHaveBeenCalledWith('', {
      modal: {
        type: 'joinRoom',
        roomId: 'restricted-channel',
        roomName: 'restricted',
        viewerCanJoinRoom: false
      }
    });
  });

  it('renders unread channel rows and icons in full-contrast text', async () => {
    setRoomUnread('channel-1', true);

    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    const icon = row.querySelector('.sidebar-icon');
    expect(row.classList.contains('font-semibold')).toBe(true);
    expect(row.classList.contains('text-text-top')).toBe(true);
    expect(row.classList.contains('hover:!text-text-top')).toBe(true);
    expect(icon?.classList.contains('text-text-top')).toBe(true);
    expect(icon?.classList.contains('text-muted')).toBe(false);
  });

  it('renders server-local sidebar links as same-tab anchors resolved against the active server', async () => {
    mocks.store.rooms.roomGroups = [
      {
        id: 'g1',
        name: 'Links',
        roomIds: [],
        items: [
          {
            id: 'link:docs',
            type: 'link',
            link: { id: 'docs', label: 'Docs', url: '/docs' }
          }
        ]
      }
    ];

    const { container } = render(RoomList);

    const link = q(container, '[href="https://chat.example.test/docs"]') as HTMLAnchorElement;
    await expect.element(link).toBeInTheDocument();
    expect(link.textContent).toContain('Docs');
    expect(link.getAttribute('target')).toBeNull();
    expect(link.getAttribute('rel')).toBeNull();
  });

  it('renders active-server host sidebar links as same-tab anchors', async () => {
    mocks.store.rooms.roomGroups = [
      {
        id: 'g1',
        name: 'Links',
        roomIds: [],
        items: [
          {
            id: 'link:admin',
            type: 'link',
            link: {
              id: 'admin',
              label: 'Admin',
              url: 'https://chat.example.test/admin'
            }
          }
        ]
      }
    ];

    const { container } = render(RoomList);

    const link = q(container, '[href="https://chat.example.test/admin"]') as HTMLAnchorElement;
    await expect.element(link).toBeInTheDocument();
    expect(link.getAttribute('target')).toBeNull();
    expect(link.getAttribute('rel')).toBeNull();
  });

  it('renders external sidebar links as new-tab anchors', async () => {
    mocks.store.rooms.roomGroups = [
      {
        id: 'g1',
        name: 'Links',
        roomIds: [],
        items: [
          {
            id: 'link:external',
            type: 'link',
            link: {
              id: 'external',
              label: 'External Docs',
              url: 'https://docs.example.test'
            }
          }
        ]
      }
    ];

    const { container } = render(RoomList);

    const link = q(container, '[href="https://docs.example.test/"]') as HTMLAnchorElement;
    await expect.element(link).toBeInTheDocument();
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  it('resolves a stale channel badge through the room-scoped notification query', async () => {
    setRoomNotificationCount('channel-1', 1);
    const roomNotification = notification('mention-1', 'channel-1');
    mocks.store.notifications.resolveRoomNotification.mockResolvedValue({
      ok: true,
      totalCount: 1,
      notification: roomNotification
    });
    mocks.store.notifications.getCleanPath.mockReturnValue('/chat/-/channel-1/thread-1');
    mocks.store.notifications.dismiss.mockResolvedValue(true);

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="room-notification-badge"]');
    await expect.element(badge).toBeInTheDocument();
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.resolveRoomNotification).toHaveBeenCalledWith('channel-1', {
        isDM: false
      });
      expect(mocks.store.pendingHighlights.set).toHaveBeenCalledWith(
        'channel-1',
        'thread-1',
        'event-1'
      );
      expect(mocks.store.rooms.decrementUnreadNotification).toHaveBeenCalledWith('channel-1');
      expect(mocks.store.notifications.dismiss).toHaveBeenCalledWith('mention-1');
      expect(mocks.store.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/channel-1/thread-1');
    });
  });

  it('resolves a stale DM badge through the room-scoped notification query', async () => {
    setRoomNotificationCount('dm-with-participants', 1);
    const dmNotification = notification('dm-1', 'dm-with-participants', true);
    mocks.store.notifications.resolveRoomNotification.mockResolvedValue({
      ok: true,
      totalCount: 1,
      notification: dmNotification
    });
    mocks.store.notifications.getCleanPath.mockReturnValue('/chat/-/dm-with-participants');
    mocks.store.notifications.dismiss.mockResolvedValue(true);

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="dm-notification-badge"]');
    await expect.element(badge).toBeInTheDocument();
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.resolveRoomNotification).toHaveBeenCalledWith(
        'dm-with-participants',
        { isDM: true }
      );
      expect(mocks.store.rooms.decrementUnreadNotification).toHaveBeenCalledWith(
        'dm-with-participants'
      );
      expect(mocks.store.notifications.dismiss).toHaveBeenCalledWith('dm-1');
      expect(mocks.store.rooms.refreshNotificationCounts).toHaveBeenCalledOnce();
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-with-participants');
    });
  });

  it('clears a stale room badge when the room-scoped query returns no notifications', async () => {
    setRoomNotificationCount('channel-1', 1);
    mocks.store.notifications.resolveRoomNotification.mockResolvedValue({
      ok: true,
      totalCount: 0,
      notification: null
    });

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="room-notification-badge"]');
    await expect.element(badge).toBeInTheDocument();
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.resolveRoomNotification).toHaveBeenCalledWith('channel-1', {
        isDM: false
      });
      expect(mocks.store.rooms.clearUnreadNotifications).toHaveBeenCalledWith('channel-1');
      expect(mocks.goto).not.toHaveBeenCalled();
      expect(mocks.store.notifications.dismiss).not.toHaveBeenCalled();
    });
  });
});
