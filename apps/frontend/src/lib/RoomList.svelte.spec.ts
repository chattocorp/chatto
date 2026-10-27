import { ServerProjectionStore } from '@chatto/client/server/projection';
import { NavigationStore } from '$lib/state/server/navigation';
import { RoomListView } from '@chatto/client/server/rooms';
import { RoomGroup, RoomGroupViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { PermissionGrant } from '@chatto/api-types/api/v1/permissions_pb';
import { Room, RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { RoomWithViewerState, RoomViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { Timestamp } from '@bufbuild/protobuf';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { NotificationAttentionLevel } from '@chatto/client/api/notifications';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page as browserPage, userEvent } from 'vitest/browser';
import { SvelteMap } from 'svelte/reactivity';
import { tick } from 'svelte';
import { q } from '$lib/test-utils';
import {
  deletedDirectMessageParticipant,
  type UserAvatarUserView
} from '@chatto/client/timeline/users';
import { sidebarNav } from '$lib/state/globals.svelte';
import '../app.css';

import { NotificationSignalKind } from '@chatto/client/api/notifications';
import type { RoomsListGroup, RoomsListItem } from '$lib/state/server/navigation';
import { getToasts, toast } from '$lib/ui/toast';
import { TOUCH_ONLY_QUERY } from '$lib/utils/inputMediaQueries';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    notificationPath: vi.fn().mockReturnValue('/chat/-/room'),
    activeRoomId: undefined as string | undefined,
    activeCallRoomIds: new Set<string>(),
    projectedCallParticipants: new Map<string, unknown[]>(),
    unreadRoomIds: new Set<string>(),
    writeClipboardText: vi.fn(),
    markNavigationRoomAsRead: vi.fn().mockResolvedValue(true),
    pushState: vi.fn(),
    goto: vi.fn(),
    layoutAPI: {
      createRoomGroup: vi.fn().mockResolvedValue(null),
      updateRoomGroup: vi.fn().mockResolvedValue(null),
      deleteRoomGroup: vi.fn().mockResolvedValue(true),
      createSidebarLink: vi.fn().mockResolvedValue(null),
      updateSidebarLink: vi.fn().mockResolvedValue(null),
      deleteSidebarLink: vi.fn().mockResolvedValue(true),
      moveRoomGroup: vi.fn().mockResolvedValue([]),
      moveSidebarItem: vi.fn().mockResolvedValue(null)
    },
    roomCommandAPI: {
      createRoom: vi.fn().mockResolvedValue(null),
      joinRoom: vi.fn().mockResolvedValue(null),
      archiveRoom: vi.fn().mockResolvedValue(null)
    },
    appUi: {
      disableRoomCallWideFor: vi.fn(),
      requestRoomSidebarPanel: vi.fn()
    },
    store: {
      currentUser: { user: { id: 'me' } },
      accountId: 'me',
      permissions: {
        loaded: true,
        canStartDMs: true,
        canManageBots: false,
        canAdminManageAccounts: false,
        canAdminViewUsers: false
      },
      projection: { rooms: new Map<string, { memberUserIds: string[] }>() },
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
        markRead: vi.fn()
      },
      /** Notification attention; the notification mock also serves it. */
      get attention() {
        return this.notifications;
      },
      roomUnread: {
        roomIsUnread: vi.fn((roomId: string) => mocks.unreadRoomIds.has(roomId)),
        setRoomUnread: vi.fn((roomId: string, unread: boolean) => {
          if (unread) mocks.unreadRoomIds.add(roomId);
          else mocks.unreadRoomIds.delete(roomId);
        })
      },
      activeCallRooms: {
        has: vi.fn((roomId: string) => mocks.activeCallRoomIds.has(roomId)),
        getParticipants: vi.fn(
          (roomId: string) => mocks.projectedCallParticipants.get(roomId) ?? []
        )
      },
      voiceCall: {
        join: vi.fn().mockResolvedValue(undefined),
        handleParticipantLeftEvent: vi.fn(),
        handleCallEndedEvent: vi.fn()
      },
      serverInfo: {
        livekitUrl: null
      },
      navigation: {
        rooms: [],
        roomGroups: [] as RoomsListGroup[],
        isInitialLoading: false,
        currentUserId: 'me'
      },
      get projectionViewerId(): string | null {
        return this.navigation.currentUserId;
      },
      roomDirectory: {
        joinRoom: vi.fn()
      },
      pendingHighlights: {
        set: vi.fn()
      },
      handleVoiceCallJoinFailed: vi.fn()
    }
  }
}));

const activeRoomRoute = new SvelteMap<string, string>();

// The store mock also carries the frontend UI state of its server.
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    tryGetStore: () => mocks.store,
    isOriginServer: vi.fn(() => true),
    getServer: vi.fn(() => ({ id: 'origin', url: 'https://chat.example.test' })),
    originServer: { id: 'origin' },
    servers: [
      { id: 'origin', url: 'https://chat.example.test' },
      { id: 'remote', url: 'https://remote.example.test' }
    ]
  }
}));

vi.mock('$lib/notificationPath', () => ({ notificationPath: mocks.notificationPath }));

vi.mock('$app/state', () => ({
  page: {
    params: {
      serverId: '-',
      get roomId() {
        return activeRoomRoute.get('roomId') ?? mocks.activeRoomId;
      }
    }
  }
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto,
  pushState: mocks.pushState
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
      .replace('[groupId]', params?.groupId ?? '')
      .replace('[botId]', params?.botId ?? '')
      .replace('[userId]', params?.userId ?? '')
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (id: string) => (id === 'remote' ? 'remote.example.test' : '-'),
  segmentToServerId: (segment: string) =>
    segment === 'remote.example.test' ? 'remote' : segment === '-' ? 'origin' : null
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'origin',
    store: mocks.store,
    connection: {
      getAPI: vi.fn((factory: { name?: string }) =>
        factory.name === 'createAdminRoomLayoutAPI' ? mocks.layoutAPI : mocks.roomCommandAPI
      )
    },
    isCurrent: () => true
  })
}));

vi.mock('$lib/state/appUi.svelte', () => ({
  getAppUiState: () => mocks.appUi,
  getRoomSidebarPresentation: () => 'desktop'
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveLogin: (_userId: string, fallback: string) => fallback,
  getLiveBotOwnerUserId: (_userId: string, fallback: string | null) => fallback,
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/navigation/readActions', () => ({
  markNavigationRoomAsRead: mocks.markNavigationRoomAsRead
}));

import RoomList from './RoomList.svelte';

function notification(id: string, roomId: string, isDM = false) {
  return {
    id,
    createdAt: '2026-06-18T10:00:00Z',
    actor: null,
    signalKind: isDM
      ? NotificationSignalKind.DIRECT_MESSAGE
      : NotificationSignalKind.DIRECT_MENTION,
    targetSupported: true,
    room: { id: roomId, name: 'general' },
    eventId: 'event-1',
    threadRootId: isDM ? null : 'thread-1'
  };
}

function user(id: string, login: string, displayName: string) {
  return {
    id,
    login,
    displayName,
    avatarUrl: null,
    presenceStatus: PresenceStatus.ONLINE
  };
}

function setRooms() {
  mocks.store.navigation.rooms = [
    {
      id: 'channel-1',
      name: 'general',
      type: RoomKind.CHANNEL,
      isUniversal: false,
      viewerIsMember: true,
      viewerCanJoinRoom: true,
      viewerCanManageRoom: true,
      viewerNotificationCount: 0,
      viewerImportantNotificationCount: 0,
      members: []
    },
    {
      id: 'joinable-channel',
      name: 'joinable',
      type: RoomKind.CHANNEL,
      isUniversal: false,
      viewerIsMember: false,
      viewerCanJoinRoom: true,
      viewerCanManageRoom: false,
      viewerNotificationCount: 0,
      viewerImportantNotificationCount: 0,
      members: []
    },
    {
      id: 'restricted-channel',
      name: 'restricted',
      type: RoomKind.CHANNEL,
      isUniversal: false,
      viewerIsMember: false,
      viewerCanJoinRoom: false,
      viewerCanManageRoom: false,
      viewerNotificationCount: 0,
      viewerImportantNotificationCount: 0,
      members: []
    },
    {
      id: 'dm-with-participants',
      name: '',
      type: RoomKind.DM,
      isUniversal: false,
      viewerIsMember: true,
      viewerCanJoinRoom: true,
      viewerCanManageRoom: false,
      viewerNotificationCount: 0,
      viewerImportantNotificationCount: 0,
      members: [user('me', 'me', 'Me'), user('teal', 'teal', 'Teal')]
    },
    {
      id: 'dm-phone-only',
      name: '',
      type: RoomKind.DM,
      isUniversal: false,
      viewerIsMember: true,
      viewerCanJoinRoom: true,
      viewerCanManageRoom: false,
      viewerNotificationCount: 0,
      viewerImportantNotificationCount: 0,
      members: [user('me', 'me', 'Me'), user('river', 'river', 'River')]
    }
  ] as never;
  mocks.store.projection.rooms.clear();
  for (const room of mocks.store.navigation.rooms as RoomsListItem[]) {
    mocks.store.projection.rooms.set(room.id, {
      memberUserIds: room.members.map((member) => member.id)
    });
  }
}

/** Add recent conversations so activity tests can hide DMs beyond the minimum. */
function addRecentDMs(): void {
  const rooms = mocks.store.navigation.rooms as RoomsListItem[];
  const template = rooms.find((room) => room.type === RoomKind.DM)!;
  for (let index = 0; index < 2; index++) {
    rooms.push({
      ...template,
      id: `dm-recent-${index}`,
      lastMessageAt: new Date(Date.now() - 86400000).toISOString()
    });
  }
}

function setDMParticipants(participants: UserAvatarUserView[], participantIds?: string[]): void {
  const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
    (room) => room.id === 'dm-with-participants'
  )!;
  room.members = participants;
  mocks.store.projection.rooms.set(room.id, {
    memberUserIds: participantIds ?? participants.map((member) => member.id)
  });
}

async function openDMMenu(container: HTMLElement): Promise<HTMLElement> {
  const row = q(container, '[href="/chat/-/dm-with-participants"]')!;
  row.dispatchEvent(
    new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 40, clientY: 60 })
  );
  await expect
    .element(browserPage.getByRole('dialog', { name: 'User profile', exact: true }))
    .toBeVisible();
  return q(document.body, '[role="dialog"][aria-label="User profile"]')!;
}

function setRoomNotificationCount(roomId: string, count: number, importantCount = count) {
  const rooms = mocks.store.navigation.rooms as Array<{
    id: string;
    viewerNotificationCount: number;
    viewerImportantNotificationCount: number;
  }>;
  const room = rooms.find((item) => item.id === roomId);
  if (!room) throw new Error(`Missing mocked room ${roomId}`);
  room.viewerNotificationCount = count;
  room.viewerImportantNotificationCount = importantCount;
}

function setRoomUnread(roomId: string, hasUnread: boolean) {
  if (hasUnread) mocks.unreadRoomIds.add(roomId);
  else mocks.unreadRoomIds.delete(roomId);
}

beforeEach(() => {
  toast.clear();
  localStorage.clear();
  sessionStorage.clear();
  mocks.activeRoomId = undefined;
  activeRoomRoute.clear();
  sidebarNav.setMobile(false);
  if (!sidebarNav.isOpen) sidebarNav.toggle();
  mocks.activeCallRoomIds = new Set();
  mocks.projectedCallParticipants = new Map();
  mocks.unreadRoomIds = new Set();
  mocks.store.navigation.roomGroups = [];
  mocks.store.navigation.isInitialLoading = false;
  mocks.store.navigation.currentUserId = 'me';
  mocks.store.permissions.canStartDMs = true;
  mocks.store.permissions.canManageBots = false;
  mocks.store.permissions.canAdminViewUsers = false;
  setRooms();
  vi.clearAllMocks();
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: mocks.writeClipboardText },
    configurable: true
  });
  mocks.writeClipboardText.mockResolvedValue(undefined);
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
  mocks.notificationPath.mockReturnValue('/chat/-/room');
  mocks.store.roomDirectory.joinRoom.mockResolvedValue({ ok: true });
  mocks.markNavigationRoomAsRead.mockResolvedValue(true);
});

describe('RoomList', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('hides ages in narrow rows while notification badges stay visible', async () => {
    const rooms = mocks.store.navigation.rooms as RoomsListItem[];
    const room = rooms.find((entry) => entry.id === 'dm-with-participants')!;
    room.lastMessageAt = new Date(Date.now() - 3 * 86400000).toISOString();
    const badgeRoom = rooms.find((entry) => entry.id === 'dm-phone-only')!;
    badgeRoom.viewerNotificationCount = 1;
    badgeRoom.viewerImportantNotificationCount = 1;
    const { container } = render(RoomList);
    const row = container.querySelector<HTMLAnchorElement>(
      '[href="/chat/-/dm-with-participants"]'
    )!;
    const age = row.querySelector('time')!;
    const name = row.querySelector<HTMLElement>('.min-w-0.flex-1')!;
    const badge = q(container, '[data-testid="dm-notification-badge"]')!;
    const nameWidths: number[] = [];
    for (const [width, visible] of [
      [220, false],
      [280, true],
      [400, true],
      [220, false]
    ] as const) {
      container.style.width = `${width}px`;
      await vi.waitFor(() => expect(getComputedStyle(age).display !== 'none').toBe(visible));
      await expect.element(badge).toBeVisible();
      nameWidths.push(name.getBoundingClientRect().width);
    }
    expect(nameWidths.at(-1)).toBe(nameWidths[0]);
    // At the same narrow width, the hidden age takes no space from the name.
    age.style.display = 'inline';
    expect(name.getBoundingClientRect().width).toBeLessThan(nameWidths.at(-1)!);
  });

  it.each([
    [0, 'now'],
    [59 / 60, 'now'],
    [1, '1min'],
    [59, '59min'],
    [60, '1h'],
    [1439, '23h'],
    [1440, '1d'],
    [90 * 1440, '90d'],
    [-1, 'now']
  ])('shows %s elapsed minutes as %s, including older expanded DMs', async (minutes, age) => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-10-22T12:00:00Z'));
    addRecentDMs();
    const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
      (r) => r.id === 'dm-with-participants'
    )!;
    room.lastMessageAt = new Date(Date.now() - Number(minutes) * 60000).toISOString();
    room.lastRootMessageAt = new Date(Date.now() - 120 * 86400000).toISOString();
    const { container } = render(RoomList);
    if (Number(minutes) > 7 * 1440) {
      await browserPage.getByRole('button', { name: 'Show older (1)', exact: true }).click();
    }
    const time = container.querySelector<HTMLTimeElement>(
      '[href="/chat/-/dm-with-participants"] time'
    )!;
    await expect.element(time).toHaveTextContent(String(age));
    expect(time.getAttribute('datetime')).toBe(room.lastMessageAt);
    expect(time.getAttribute('title')).toBeTruthy();
    expect(time.getAttribute('dir')).toBe('ltr');
  });

  it.each([
    [0, 'now', 60000, '1min'],
    [59, '59min', 120000, '1h']
  ])(
    'refreshes %s elapsed minutes from %s on the minute tick without replacing the room',
    async (minutes, initialAge, elapsed, updatedAge) => {
      vi.useFakeTimers({ toFake: ['Date'] });
      vi.setSystemTime(new Date('2026-10-22T12:00:00Z'));
      const now = Date.now();
      const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
        (r) => r.id === 'dm-with-participants'
      )!;
      room.lastMessageAt = new Date(now - Number(minutes) * 60000).toISOString();
      const intervals = vi.spyOn(window, 'setInterval');
      const { container, unmount } = render(RoomList);
      try {
        const time = container.querySelector<HTMLTimeElement>(
          '[href="/chat/-/dm-with-participants"] time'
        )!;
        await expect.element(time).toHaveTextContent(String(initialAge));
        const interval = intervals.mock.calls.find((args) => args[1] === 60000)![0] as () => void;
        const dateNow = vi.spyOn(Date, 'now').mockReturnValue(now + Number(elapsed));
        try {
          interval();
        } finally {
          dateNow.mockRestore();
        }
        await expect.element(time).toHaveTextContent(String(updatedAge));
        expect(
          (mocks.store.navigation.rooms as RoomsListItem[]).find((r) => r.id === room.id)
        ).toBe(room);
      } finally {
        await unmount();
        intervals.mockRestore();
      }
    }
  );

  it.each([undefined, null, 'invalid'])('omits unknown or invalid activity %s', (timestamp) => {
    const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
      (r) => r.id === 'dm-with-participants'
    )!;
    room.lastMessageAt = timestamp;
    const { container } = render(RoomList);
    expect(container.querySelector('[data-testid="dm-activity-age"]')).toBeNull();
  });

  it.each([0, 1])(
    'gives notification badges priority over age with %s important occurrences',
    async (important) => {
      const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
        (r) => r.id === 'dm-with-participants'
      )!;
      room.lastMessageAt = new Date(Date.now() - 3 * 86400000).toISOString();
      room.viewerNotificationCount = 1;
      room.viewerImportantNotificationCount = important;
      const { container } = render(RoomList);
      expect(container.querySelector('[data-testid="dm-activity-age"]')).toBeNull();
      await expect
        .element(
          q(
            container,
            important
              ? '[data-testid="dm-notification-badge"]'
              : '[data-testid="dm-ambient-notification-badge"]'
          )
        )
        .toBeInTheDocument();
    }
  );

  it('keeps the unread dot and call icon beside DM activity while channels have no age', async () => {
    const rooms = mocks.store.navigation.rooms as RoomsListItem[];
    for (const room of rooms)
      room.lastMessageAt = new Date(Date.now() - 3 * 86400000).toISOString();
    mocks.unreadRoomIds.add('dm-with-participants');
    mocks.activeCallRoomIds.add('dm-with-participants');
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/dm-with-participants"]')!;
    await expect
      .element(row.querySelector<HTMLElement>('[data-testid="dm-unread-dot"]'))
      .toBeInTheDocument();
    await expect
      .element(row.querySelector<HTMLElement>('[data-testid="room-call-icon"]'))
      .toBeInTheDocument();
    await expect.element(row.querySelector('time')).toHaveTextContent('3d');
    expect(container.querySelector('[href="/chat/-/channel-1"] time')).toBeNull();
  });

  it.each([0, 1, 2, 3, 5])('shows up to three of %s old DMs by default', async (count) => {
    const template = (mocks.store.navigation.rooms as RoomsListItem[]).find(
      (room) => room.type === RoomKind.DM
    )!;
    // Navigation order differs from activity order; the newest are at the end.
    mocks.store.navigation.rooms = Array.from({ length: count }, (_, index) => ({
      ...template,
      id: `dm-old-${index}`,
      lastMessageAt: new Date(Date.now() - (20 - index) * 86400000).toISOString()
    })) as never;
    const { container } = render(RoomList);
    const visibleIds = () =>
      Array.from(container.querySelectorAll('a[href^="/chat/-/dm-old-"]'), (link) =>
        link.getAttribute('href')!.split('/').at(-1)
      );
    const allIds = Array.from({ length: count }, (_, index) => `dm-old-${index}`);
    expect(visibleIds()).toEqual(allIds.slice(-3).reverse());
    if (count <= 3) {
      expect(container.querySelector('[data-testid="older-conversations"]')).toBeNull();
    } else {
      await browserPage.getByRole('button', { name: 'Show older (2)', exact: true }).click();
      expect(visibleIds()).toEqual(allIds.toReversed());
      await browserPage.getByRole('button', { name: 'Show less', exact: true }).click();
      await vi.waitFor(() => expect(visibleIds()).toEqual(allIds.slice(-3).reverse()));
    }
  });

  it.each([
    { ages: [12, 1, 10, 8], visibleIds: [1, 3, 2], hiddenCount: 1 },
    { ages: [12, 1, 2, 8, 10], visibleIds: [1, 2, 3], hiddenCount: 2 },
    { ages: [12, 1, 1, 1, 8, 10], visibleIds: [1, 2, 3], hiddenCount: 3 },
    { ages: [12, 1, 1, 1, 1, 8, 10], visibleIds: [1, 2, 3, 4], hiddenCount: 3 }
  ])('fills the minimum for DM ages $ages', ({ ages, visibleIds, hiddenCount }) => {
    // Equal ages must produce equal timestamps even when fixture creation crosses a millisecond.
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date('2026-10-27T12:00:00Z'));
    const template = (mocks.store.navigation.rooms as RoomsListItem[]).find(
      (room) => room.type === RoomKind.DM
    )!;
    mocks.store.navigation.rooms = ages.map((days, index) => ({
      ...template,
      id: `dm-mixed-${index}`,
      lastMessageAt: new Date(Date.now() - days * 86400000).toISOString()
    })) as never;
    const { container } = render(RoomList);
    const visible = Array.from(container.querySelectorAll('a[href^="/chat/-/dm-mixed-"]'), (link) =>
      link.getAttribute('href')
    );
    expect(visible).toEqual(visibleIds.map((id) => `/chat/-/dm-mixed-${id}`));
    expect(container.querySelector('[data-testid="older-conversations"]')).toHaveTextContent(
      `Show older (${hiddenCount})`
    );
  });

  it.each([false, true])(
    'reorders a revived DM while mounted with older DMs shown: %s',
    async (expanded) => {
      vi.useFakeTimers({ toFake: ['Date'] });
      vi.setSystemTime(new Date('2026-10-25T12:00:00Z'));
      const originalNavigation = mocks.store.navigation;
      const projection = new ServerProjectionStore();
      const updateRoom = (id: string, hours: number) => {
        projection.rooms.set(
          id,
          new RoomWithViewerState({
            room: new Room({
              id,
              name: id,
              kind: RoomKind.DM
            }),
            lastMessageAt: Timestamp.fromDate(new Date(Date.now() - hours * 3600000)),
            lastRootMessageAt: Timestamp.fromDate(new Date('2026-09-01T12:00:00Z')),
            viewerState: new RoomViewerState({
              isMember: true,
              permissions: [new PermissionGrant({ permission: 'message.read', granted: true })]
            }),
            hasMessageHistory: true
          })
        );
      };
      // Deliberately keep insertion order different from posting order.
      updateRoom('dm-two-days', 48);
      updateRoom('dm-one-day', 24);
      updateRoom('dm-three-days', 72);
      updateRoom('dm-revived', 240);
      mocks.store.navigation = new NavigationStore(
        new RoomListView(projection, { hasUsableProjection: true }),
        () => ({
          unreadNotificationCount: 0,
          importantUnreadNotificationCount: 0,
          roomUnreadCounts: {},
          roomImportantUnreadCounts: {}
        })
      ) as unknown as typeof originalNavigation;
      try {
        const { container } = render(RoomList);
        const ids = () =>
          Array.from(container.querySelectorAll('a[href^="/chat/-/dm-"]'), (link) =>
            link.getAttribute('href')!.split('/').at(-1)
          );
        expect(ids()).toEqual(['dm-one-day', 'dm-two-days', 'dm-three-days']);
        if (expanded) {
          await browserPage.getByRole('button', { name: 'Show older (1)', exact: true }).click();
          expect(ids()).toEqual(['dm-one-day', 'dm-two-days', 'dm-three-days', 'dm-revived']);
        }
        // A reply updates posting activity without changing root-message activity.
        updateRoom('dm-revived', 1);
        await vi.waitFor(() =>
          expect(ids()).toEqual(['dm-revived', 'dm-one-day', 'dm-two-days', 'dm-three-days'])
        );
        await expect
          .element(container.querySelector<HTMLTimeElement>('[href="/chat/-/dm-revived"] time'))
          .toHaveTextContent('1h');
      } finally {
        mocks.store.navigation = originalNavigation;
      }
    }
  );

  it('hides old DMs, reveals them on request, and keeps unknown activity visible', async () => {
    addRecentDMs();
    const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
      (entry) => entry.id === 'dm-with-participants'
    )!;
    room.lastMessageAt = new Date(Date.now() - 8 * 86400000).toISOString();
    const { container } = render(RoomList);
    expect(container.querySelector('[href="/chat/-/dm-with-participants"]')).toBeNull();
    await expect.element(q(container, '[href="/chat/-/dm-phone-only"]')).toBeInTheDocument();
    const toggle = browserPage.getByRole('button', { name: 'Show older (1)', exact: true });
    await expect.element(toggle).toHaveTextContent('Show older (1)');
    await toggle.click();
    await expect.element(q(container, '[href="/chat/-/dm-with-participants"]')).toBeInTheDocument();
    const showLess = browserPage.getByRole('button', { name: 'Show less', exact: true });
    await expect.element(showLess).toHaveAttribute('aria-expanded', 'true');
    await showLess.click();
    await vi.waitFor(() =>
      expect(container.querySelector('[href="/chat/-/dm-with-participants"]')).toBeNull()
    );
  });

  it.each(['recent', 'unread', 'notification', 'current', 'call', 'invalid'])(
    'keeps a DM visible with %s activity or attention',
    async (reason) => {
      addRecentDMs();
      const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
        (entry) => entry.id === 'dm-with-participants'
      )!;
      room.lastMessageAt = new Date(
        Date.now() - (reason === 'recent' ? 6 : 8) * 86400000
      ).toISOString();
      if (reason === 'unread') mocks.unreadRoomIds.add(room.id);
      if (reason === 'notification') room.viewerNotificationCount = 1;
      if (reason === 'current') mocks.activeRoomId = room.id;
      if (reason === 'call') mocks.activeCallRoomIds.add(room.id);
      if (reason === 'invalid') room.lastMessageAt = 'invalid';
      const { container } = render(RoomList);
      await expect
        .element(q(container, '[href="/chat/-/dm-with-participants"]'))
        .toBeInTheDocument();
    }
  );

  it('keeps older-conversation controls out of the collapsed section', async () => {
    addRecentDMs();
    const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
      (entry) => entry.id === 'dm-with-participants'
    )!;
    room.lastMessageAt = new Date(Date.now() - 8 * 86400000).toISOString();
    const { container } = render(RoomList);
    await browserPage.getByRole('button', { name: 'Direct Messages', exact: true }).click();
    await vi.waitFor(() =>
      expect(container.querySelector('[data-testid="older-conversations"]')).toBeNull()
    );
    // Restore the reactive persisted preference as well as browser storage.
    await browserPage.getByRole('button', { name: 'Direct Messages', exact: true }).click();
  });

  it('ages DMs while mounted and clears its interval when removed', async () => {
    addRecentDMs();
    const now = Date.now();
    const room = (mocks.store.navigation.rooms as RoomsListItem[]).find(
      (entry) => entry.id === 'dm-with-participants'
    )!;
    room.lastMessageAt = new Date(now - 7 * 86400000 + 60000).toISOString();
    const intervals = vi.spyOn(window, 'setInterval');
    const clearInterval = vi.spyOn(window, 'clearInterval');
    const { container, unmount } = render(RoomList);
    try {
      await expect
        .element(q(container, '[href="/chat/-/dm-with-participants"]'))
        .toBeInTheDocument();
      await expect
        .element(
          container.querySelector<HTMLTimeElement>('[href="/chat/-/dm-with-participants"] time')
        )
        .toHaveTextContent('6d');
      const timerIndex = intervals.mock.calls.findIndex((args) => args[1] === 60000);
      expect(timerIndex).toBeGreaterThanOrEqual(0);
      const timer = intervals.mock.calls[timerIndex][0] as () => void;
      const dateNow = vi.spyOn(Date, 'now').mockReturnValue(now + 120000);
      try {
        timer();
      } finally {
        dateNow.mockRestore();
      }
      await vi.waitFor(() =>
        expect(container.querySelector('[href="/chat/-/dm-with-participants"]')).toBeNull()
      );
      await browserPage.getByRole('button', { name: 'Show older (1)', exact: true }).click();
      await expect
        .element(
          container.querySelector<HTMLTimeElement>('[href="/chat/-/dm-with-participants"] time')
        )
        .toHaveTextContent('7d');
      const timerId = intervals.mock.results[timerIndex].value;
      await unmount();
      expect(clearInterval).toHaveBeenCalledWith(timerId);
    } finally {
      intervals.mockRestore();
      clearInterval.mockRestore();
    }
  });

  it('hides only DMs explicitly projected without message history', async () => {
    const empty = mocks.store.navigation.rooms.find(
      (room: { id: string }) => room.id === 'dm-with-participants'
    ) as unknown as { hasMessageHistory?: boolean };
    empty.hasMessageHistory = false;

    const { container } = render(RoomList);

    expect(container.querySelector('[href="/chat/-/dm-with-participants"]')).toBeNull();
    await expect.element(q(container, '[href="/chat/-/dm-phone-only"]')).toBeInTheDocument();
  });

  it('renders a lone configured room group and keeps highlighted rooms visible when collapsed', async () => {
    const channel = mocks.store.navigation.rooms.find(
      (room: { id: string }) => room.id === 'channel-1'
    );
    mocks.store.navigation.rooms = [channel] as never;
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: false,
        roomIds: ['channel-1']
      }
    ];
    setRoomUnread('channel-1', true);

    const { container } = render(RoomList);
    const groupHeaders = container.querySelectorAll<HTMLButtonElement>(
      'button[aria-expanded]:not([data-testid="room-group-more"])'
    );
    const groupHeader = groupHeaders[0];

    expect(groupHeaders).toHaveLength(1);
    await expect.element(groupHeader).toHaveTextContent('Projects');
    await expect.element(groupHeader).toHaveAttribute('aria-expanded', 'true');
    expect(groupHeader?.parentElement?.parentElement?.classList.contains('mt-4')).toBe(false);

    groupHeader?.click();

    await expect.element(groupHeader).toHaveAttribute('aria-expanded', 'false');
    await expect.element(q(container, '[href="/chat/-/channel-1"]')).toBeInTheDocument();

    groupHeader?.click();
    await expect.element(groupHeader).toHaveAttribute('aria-expanded', 'true');
  });

  it('renders a DM-only sidebar with the same first-section disclosure treatment', async () => {
    const dm = mocks.store.navigation.rooms.find(
      (room: { id: string }) => room.id === 'dm-with-participants'
    );
    mocks.store.navigation.rooms = [dm] as never;

    const { container } = render(RoomList);
    const groupHeaders = container.querySelectorAll<HTMLButtonElement>(
      'button[aria-expanded]:not([data-testid="room-group-more"])'
    );
    const groupHeader = groupHeaders[0];

    expect(groupHeaders).toHaveLength(1);
    await expect.element(groupHeader).toHaveTextContent('Direct Messages');
    await expect.element(groupHeader).toHaveAttribute('aria-expanded', 'true');
    expect(groupHeader?.parentElement?.parentElement?.classList.contains('mt-4')).toBe(false);

    groupHeader?.click();

    await expect.element(groupHeader).toHaveAttribute('aria-expanded', 'false');
    await vi.waitFor(() =>
      expect(container.querySelector('[href="/chat/-/dm-with-participants"]')).toBeNull()
    );

    groupHeader?.click();
    await expect.element(groupHeader).toHaveAttribute('aria-expanded', 'true');
  });

  it('shows the current account name in a self-DM sidebar row', async () => {
    mocks.store.navigation.rooms = [
      {
        id: 'dm-self',
        name: '',
        type: RoomKind.DM,
        viewerIsMember: true,
        hasMessageHistory: true,
        members: [user('me', 'me', 'My name')]
      }
    ] as never;

    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/dm-self"]')!;
    await expect.element(row).toHaveTextContent('My name');
    expect(row.querySelector('[data-testid="you-badge"]')).not.toBeNull();
  });

  it('shows a deleted partner instead of a self-DM in the sidebar row', async () => {
    mocks.store.navigation.rooms = [
      {
        id: 'dm-deleted',
        name: '',
        type: RoomKind.DM,
        viewerIsMember: true,
        hasMessageHistory: true,
        members: [user('me', 'me', 'My name'), deletedDirectMessageParticipant('gone')]
      }
    ] as never;

    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/dm-deleted"]')!;
    await expect.element(row).toHaveTextContent('[deleted user]');
    expect(row.textContent).not.toContain('My name');
    expect(row.querySelector('[data-testid="you-badge"]')).toBeNull();
    expect(row.querySelector('[role="img"][aria-label="[deleted user]"]')).not.toBeNull();
  });

  it('renders a full-width separator between adjacent room and DM sections', () => {
    const { container } = render(RoomList);
    const roomList = q(container, 'nav.room-list') as HTMLElement;
    const groupHeaders = container.querySelectorAll<HTMLButtonElement>(
      'button[aria-expanded]:not([data-testid="room-group-more"])'
    );
    const sections = roomList.querySelectorAll<HTMLElement>('[data-testid="room-group-section"]');
    const separatedSection = sections[1];

    expect(groupHeaders).toHaveLength(2);
    expect(sections).toHaveLength(2);
    expect(separatedSection?.classList.contains('border-t')).toBe(true);
    expect(separatedSection?.previousElementSibling?.contains(groupHeaders[0])).toBe(true);
    expect(separatedSection?.contains(groupHeaders[1])).toBe(true);
  });

  it('opens room actions on right-click and marks an unread room as read', async () => {
    setRoomUnread('channel-1', true);
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;

    row.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 40, clientY: 60 })
    );
    await vi.waitFor(() => expect(document.body.textContent).toContain('Mark as read'));

    const markRead = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Mark as read'
    );
    const leave = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Leave room'
    );
    await expect.element(markRead ?? null).toBeInTheDocument();
    await expect.element(markRead ?? null).toBeEnabled();
    await expect.element(leave ?? null).toBeInTheDocument();
    expect(markRead?.closest('.menu-section')).not.toBe(leave?.closest('.menu-section'));
    expect(q(document.body, '[role="separator"]')).toBeNull();

    markRead!.click();

    expect(mocks.markNavigationRoomAsRead).toHaveBeenCalledWith('origin', 'channel-1');
  });

  it.each([false, true])('copies the room link and reports clipboard failure=%s', async (fails) => {
    if (fails) mocks.writeClipboardText.mockRejectedValue(new Error('denied'));
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    const action = browserPage.getByTestId('copy-room-link');
    await expect.element(action).toHaveTextContent('Copy room link');
    await action.click();
    expect(mocks.writeClipboardText).toHaveBeenCalledWith(
      'https://chat.example.test/chat/-/channel-1'
    );
    await vi.waitFor(() =>
      expect(getToasts().map((item) => item.message)).toContain(
        fails ? 'Failed to copy link' : 'Link copied'
      )
    );
    await expect.element(action).not.toBeInTheDocument();
  });

  it.each([false, true])(
    'groups room, copy, and exit actions for universal=%s and copies the room ID',
    async (isUniversal) => {
      const rooms = mocks.store.navigation.rooms as RoomsListItem[];
      const room = rooms.find((room) => room.id === 'channel-1')!;
      room.isUniversal = isUniversal;
      room.viewerCanInvite = true;
      const { container } = render(RoomList);
      const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;

      row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
      await vi.waitFor(() =>
        expect(document.querySelector('[data-testid="copy-room-id"]')).not.toBeNull()
      );

      const copyRoomId = q(document.body, '[data-testid="copy-room-id"]') as HTMLButtonElement;
      await expect.element(copyRoomId).toHaveTextContent('Copy room ID');
      const sections = copyRoomId.closest('[role="menu"]')!.querySelectorAll('.menu-section');
      expect(
        Array.from(sections, (section) =>
          Array.from(section.querySelectorAll('[role="menuitem"]'), (item) =>
            item.textContent?.trim()
          )
        )
      ).toEqual([
        ['Invite member', 'Mark as read'],
        ['Copy room link', 'Copy room ID'],
        ['Open room settings', 'Archive room', ...(isUniversal ? [] : ['Leave room'])]
      ]);

      copyRoomId.click();

      await vi.waitFor(() => expect(mocks.writeClipboardText).toHaveBeenCalledWith('channel-1'));
      expect(getToasts().map((item) => item.message)).toContain('Copied to clipboard');
    }
  );

  it('keeps an invited non-member room visible and opens its invitation instead of joining', async () => {
    const rooms = mocks.store.navigation.rooms as RoomsListItem[];
    const invited = rooms.find((room) => room.id === 'restricted-channel')!;
    invited.viewerInvitation = {
      id: 'invitation-1',
      senderId: 'sender-1',
      expiresAt: new Date('2099-01-01'),
      viewerCanAccept: true
    };
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/restricted-channel"]') as HTMLAnchorElement;
    await expect.element(row).toBeVisible();
    expect(row.classList.contains('opacity-60')).toBe(false);
    await expect
      .element(row.querySelector<HTMLElement>('[aria-label="You have been invited"]'))
      .toBeVisible();
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await browserPage.getByRole('menuitem', { name: 'You have been invited' }).click();
    expect(mocks.store.roomDirectory.joinRoom).not.toHaveBeenCalled();
    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/restricted-channel');
  });

  it.each(['badge', 'notification'] as const)(
    'shows %s attention for an invited nonmember',
    async (delivery) => {
      const invited = (mocks.store.navigation.rooms as RoomsListItem[]).find(
        (room) => room.id === 'restricted-channel'
      )!;
      invited.viewerInvitation = {
        id: 'invitation',
        senderId: 'sender',
        expiresAt: new Date('2099-01-01'),
        viewerCanAccept: true
      };
      if (delivery === 'badge') mocks.unreadRoomIds.add(invited.id);
      else {
        invited.viewerNotificationCount = 1;
        invited.viewerImportantNotificationCount = 1;
      }
      const { container } = render(RoomList);
      const row = q(container, '[href="/chat/-/restricted-channel"]')!;
      await expect
        .element(
          row.querySelector<HTMLElement>(
            delivery === 'badge'
              ? '[data-testid="room-unread-dot"]'
              : '[data-testid="room-notification-badge"]'
          )
        )
        .toBeVisible();
      expect(mocks.store.roomDirectory.joinRoom).not.toHaveBeenCalled();
    }
  );

  it.each([true, false])(
    'shows Invite member only with room invitation authority (%s)',
    async (allowed) => {
      const rooms = mocks.store.navigation.rooms as RoomsListItem[];
      rooms.find((room) => room.id === 'channel-1')!.viewerCanInvite = allowed;
      const { container } = render(RoomList);
      const row = q(container, '[href="/chat/-/channel-1"]')!;
      row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
      await expect.element(browserPage.getByTestId('copy-room-id')).toBeVisible();
      const invite = browserPage.getByRole('menuitem', { name: 'Invite member' });
      if (allowed) await expect.element(invite).toBeVisible();
      else await expect.element(invite).not.toBeInTheDocument();
    }
  );

  it('offers a join action for a visible non-member room', async () => {
    const { container } = render(RoomList);
    q(container, '[data-testid="room-group-more"]')?.click();
    await vi.waitFor(() =>
      expect(container.querySelector('[href="/chat/-/joinable-channel"]')).not.toBeNull()
    );
    const row = q(container, '[href="/chat/-/joinable-channel"]') as HTMLAnchorElement;

    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Join room'));

    const join = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Join room'
    );
    const markRead = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Mark as read'
    );
    const leave = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Leave room'
    );
    await expect.element(join ?? null).toBeEnabled();
    expect(markRead).toBeUndefined();
    expect(leave).toBeUndefined();

    join!.click();
    await vi.waitFor(() =>
      expect(mocks.store.roomDirectory.joinRoom).toHaveBeenCalledWith('joinable-channel')
    );
  });

  it('offers room settings to a non-member room manager alongside Join', async () => {
    const rooms = mocks.store.navigation.rooms as Array<{
      id: string;
      viewerCanManageRoom: boolean;
    }>;
    const channel = rooms.find((room) => room.id === 'joinable-channel');
    if (!channel) throw new Error('Missing mocked room joinable-channel');
    channel.viewerCanManageRoom = true;

    const { container } = render(RoomList);
    q(container, '[data-testid="room-group-more"]')?.click();
    await vi.waitFor(() =>
      expect(container.querySelector('[href="/chat/-/joinable-channel"]')).not.toBeNull()
    );
    const row = q(container, '[href="/chat/-/joinable-channel"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Open room settings'));

    const join = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Join room'
    );
    const settings = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Open room settings'
    );
    await expect.element(join ?? null).toBeEnabled();
    await expect.element(settings ?? null).toBeInTheDocument();

    expect(join?.closest('.menu-section')).not.toBe(settings?.closest('.menu-section'));

    settings!.click();
    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/manage/rooms/joinable-channel');
  });

  it('shows a disabled join action for a visible restricted room', async () => {
    const { container } = render(RoomList);
    q(container, '[data-testid="room-group-more"]')?.click();
    await vi.waitFor(() =>
      expect(container.querySelector('[href="/chat/-/restricted-channel"]')).not.toBeNull()
    );
    const row = q(container, '[href="/chat/-/restricted-channel"]') as HTMLAnchorElement;

    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Join room'));

    const join = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Join room'
    );
    await expect.element(join ?? null).toBeDisabled();
  });

  it('opens room actions after a touch long-press and suppresses its synthetic click', async () => {
    vi.useFakeTimers();
    mocks.activeCallRoomIds.add('channel-1');
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    const callIcon = q(row, '[data-testid="room-call-icon"]') as HTMLElement;

    callIcon.dispatchEvent(
      new PointerEvent('pointerdown', {
        bubbles: true,
        pointerId: 1,
        pointerType: 'touch',
        isPrimary: true,
        clientX: 20,
        clientY: 30
      })
    );
    await vi.advanceTimersByTimeAsync(500);

    const leave = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Leave room'
    );
    await expect.element(leave ?? null).toBeInTheDocument();

    callIcon.dispatchEvent(
      new PointerEvent('pointerup', {
        bubbles: true,
        pointerId: 1,
        pointerType: 'touch',
        isPrimary: true
      })
    );
    callIcon.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
    expect(mocks.goto).not.toHaveBeenCalled();

    callIcon.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/channel-1');
    vi.useRealTimers();
  });

  it('cancels a pending long-press when touch movement indicates scrolling', async () => {
    vi.useFakeTimers();
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;

    row.dispatchEvent(
      new PointerEvent('pointerdown', {
        bubbles: true,
        pointerId: 2,
        pointerType: 'touch',
        isPrimary: true,
        clientX: 10,
        clientY: 10
      })
    );
    row.dispatchEvent(
      new PointerEvent('pointermove', {
        bubbles: true,
        pointerId: 2,
        pointerType: 'touch',
        isPrimary: true,
        clientX: 20,
        clientY: 10
      })
    );
    await vi.advanceTimersByTimeAsync(500);

    expect(document.querySelector('dialog.bottom-sheet')).toBeNull();
    vi.useRealTimers();
  });

  it('keeps a touch-native contextmenu in the sheet presentation', async () => {
    vi.useFakeTimers();
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;

    row.dispatchEvent(
      new PointerEvent('pointerdown', {
        bubbles: true,
        pointerId: 3,
        pointerType: 'touch',
        isPrimary: true,
        clientX: 12,
        clientY: 16
      })
    );
    row.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 12, clientY: 16 })
    );
    await vi.advanceTimersByTimeAsync(0);

    await expect
      .element(q(document.body, 'dialog.bottom-sheet'))
      .toHaveAttribute('aria-label', 'Actions for #general');
    vi.useRealTimers();
  });

  it('opens the other user menu for a DM and copies the user ID', async () => {
    // Participant order must not determine whose profile opens.
    setDMParticipants([user('teal', 'teal', 'Teal'), user('me', 'me', 'Me')]);
    const { container } = render(RoomList);
    const menu = await openDMMenu(container);
    expect(menu.textContent).toContain('Teal');
    expect(menu.textContent).toContain('@teal');
    expect(menu.textContent).not.toContain('Mark as read');
    expect(menu.textContent).not.toContain('Copy room ID');
    expect(menu.textContent).not.toContain('Leave room');
    q(menu, '[data-testid="copy-user-id"]')!.click();
    await vi.waitFor(() => expect(mocks.writeClipboardText).toHaveBeenCalledWith('teal'));
    await expect.element(menu).not.toBeInTheDocument();
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('opens the viewer menu for a self-DM', async () => {
    setDMParticipants([user('me', 'me', 'Me')]);
    const { container } = render(RoomList);
    const menu = await openDMMenu(container);
    expect(menu.textContent).toContain('@me');
    q(menu, '[data-testid="copy-user-id"]')!.click();
    await vi.waitFor(() => expect(mocks.writeClipboardText).toHaveBeenCalledWith('me'));
  });

  it('shows a deleted DM partner without a messaging action', async () => {
    setDMParticipants([user('me', 'me', 'Me'), deletedDirectMessageParticipant('deleted-peer')]);
    const { container } = render(RoomList);
    const menu = await openDMMenu(container);
    expect(menu.textContent).toContain('[deleted user]');
    expect(menu.textContent).not.toContain('Open conversation');
    q(menu, '[data-testid="copy-user-id"]')!.click();
    await vi.waitFor(() => expect(mocks.writeClipboardText).toHaveBeenCalledWith('deleted-peer'));
  });

  it.each([
    ['group DM', ['me', 'teal', 'river']],
    ['unresolved partner', ['me', 'missing']],
    ['group DM with an unresolved participant', ['me', 'teal', 'missing']],
    ['unresolved participant IDs', []]
  ])('keeps the conversation menu for %s', async (_name, participantIds) => {
    setDMParticipants([user('me', 'me', 'Me'), user('teal', 'teal', 'Teal')], participantIds);
    const { container } = render(RoomList);
    q(container, '[href="/chat/-/dm-with-participants"]')!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    );
    await expect.element(browserPage.getByTestId('copy-room-id')).toBeVisible();
    expect(document.querySelector('[aria-label="User profile"]')).toBeNull();
    expect(document.body.textContent).not.toContain('Leave room');
  });

  it('does not treat a loading DM partner as a self-DM', async () => {
    setDMParticipants([user('me', 'me', 'Me')], ['me', 'teal']);
    const { container } = render(RoomList);
    q(container, '[href="/chat/-/dm-with-participants"]')!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    );
    await expect.element(browserPage.getByTestId('copy-room-id')).toBeVisible();
    expect(document.querySelector('[aria-label="User profile"]')).toBeNull();
  });

  it.each([true, false])('gates the DM messaging action with permission=%s', async (allowed) => {
    mocks.store.permissions.canStartDMs = allowed;
    const { container } = render(RoomList);
    const menu = await openDMMenu(container);
    const send = Array.from(menu.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Open conversation'
    );
    expect(!!send).toBe(allowed);
    if (send) {
      send.click();
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-with-participants');
      await expect.element(menu).not.toBeInTheDocument();
    }
  });

  it.each([true, false])(
    'preserves the bot management action with permission=%s',
    async (allowed) => {
      mocks.store.permissions.canManageBots = allowed;
      setDMParticipants([user('me', 'me', 'Me'), { ...user('teal', 'teal', 'Teal'), isBot: true }]);
      const { container } = render(RoomList);
      const menu = await openDMMenu(container);
      expect(!!menu.querySelector('[data-testid="manage-bot"]')).toBe(allowed);
    }
  );

  it('dismisses the DM user menu and replaces it with channel actions', async () => {
    const { container } = render(RoomList);
    const menu = await openDMMenu(container);
    await userEvent.keyboard('{Escape}');
    await expect.element(menu).not.toBeInTheDocument();
    const reopened = await openDMMenu(container);
    q(container, '[href="/chat/-/channel-1"]')!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true })
    );
    await expect.element(browserPage.getByTestId('copy-room-id')).toBeVisible();
    await expect.element(reopened).not.toBeInTheDocument();
  });

  it('opens the DM user menu as a sheet after long-press without navigating', async () => {
    vi.useFakeTimers();
    mocks.activeCallRoomIds.add('dm-with-participants');
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/dm-with-participants"]')!;
    const callIcon = q(row, '[data-testid="room-call-icon"]')!;
    callIcon.dispatchEvent(
      new PointerEvent('pointerdown', {
        bubbles: true,
        pointerId: 4,
        pointerType: 'touch',
        isPrimary: true,
        clientX: 20,
        clientY: 30
      })
    );
    await vi.advanceTimersByTimeAsync(500);
    vi.useRealTimers();
    await expect
      .element(browserPage.getByRole('dialog', { name: 'User profile', exact: true }))
      .toBeVisible();
    expect(document.querySelector('dialog.bottom-sheet[aria-label="User profile"]')).not.toBeNull();
    callIcon.dispatchEvent(
      new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        clientX: 20,
        clientY: 30
      })
    );
    callIcon.dispatchEvent(
      new PointerEvent('pointerup', {
        bubbles: true,
        pointerId: 4,
        pointerType: 'touch',
        isPrimary: true
      })
    );
    const click = new MouseEvent('click', { bubbles: true, cancelable: true });
    callIcon.dispatchEvent(click);
    expect(click.defaultPrevented).toBe(true);
    expect(mocks.goto).not.toHaveBeenCalled();
    expect(
      document.querySelectorAll('dialog.bottom-sheet[aria-label="User profile"]')
    ).toHaveLength(1);
  });

  it('opens the existing leave-room confirmation flow from room actions', async () => {
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Leave room'));

    const leave = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Leave room'
    );
    leave!.click();

    expect(mocks.pushState).toHaveBeenCalledWith('', {
      modal: { type: 'leaveRoom', serverId: 'origin', roomId: 'channel-1', roomName: 'general' }
    });
  });

  it('opens room settings for viewers who can manage the room', async () => {
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Open room settings'));

    const settings = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Open room settings'
    );
    settings!.click();

    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/manage/rooms/channel-1');
  });

  it('archives a managed room through the lazily loaded sidebar confirmation', async () => {
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Archive room'));

    const archiveMenuItem = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Archive room'
    );
    archiveMenuItem!.click();
    await vi.waitFor(() => {
      expect(document.body.textContent).toContain('Are you sure you want to archive #general?');
    });
    const archiveButtons = Array.from(document.querySelectorAll('button')).filter(
      (button) => button.textContent?.trim() === 'Archive room'
    );
    archiveButtons.at(-1)!.click();

    await vi.waitFor(() => {
      expect(mocks.roomCommandAPI.archiveRoom).toHaveBeenCalledWith('channel-1');
    });
  });

  it('hides room settings without room.manage', async () => {
    const rooms = mocks.store.navigation.rooms as Array<{
      id: string;
      viewerCanManageRoom: boolean;
    }>;
    const channel = rooms.find((room) => room.id === 'channel-1');
    if (!channel) throw new Error('Missing mocked room channel-1');
    channel.viewerCanManageRoom = false;
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Leave room'));

    const settings = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Open room settings'
    );
    expect(settings).toBeUndefined();
    expect(document.querySelectorAll('[role="menu"] .menu-section')).toHaveLength(3);
    expect(document.body.textContent).not.toContain('Archive room');
  });

  it('labels a bot beside its name in the DM list', () => {
    const rooms = mocks.store.navigation.rooms as Array<{
      id: string;
      members: Array<{ id: string; isBot?: boolean }>;
    }>;
    const dm = rooms.find((room) => room.id === 'dm-with-participants')!;
    dm.members.find((member) => member.id === 'teal')!.isBot = true;
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/dm-with-participants"]')!;
    expect(
      row.querySelector('[data-testid="bot-badge"]')?.previousElementSibling?.textContent
    ).toBe('Teal');
  });

  it('renders active-call DM rows with the pulse icon and participant avatars', async () => {
    mocks.activeCallRoomIds.add('dm-with-participants');
    mocks.projectedCallParticipants.set('dm-with-participants', [
      {
        userId: 'teal',
        login: 'teal',
        displayName: 'Teal',
        avatarUrl: null,
        isBot: true
      }
    ]);

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/dm-with-participants"]')).toBeInTheDocument();
    const dmRow = q(container, '[href="/chat/-/dm-with-participants"]');
    const icon = dmRow?.querySelector('[data-testid="room-call-icon"]');
    const pulseIcon = icon?.querySelector('[data-testid="active-call-pulse-icon"]');
    expect(icon).not.toBeNull();
    expect(icon?.classList.contains('text-action')).toBe(true);
    expect(icon?.querySelector('[class~="icon-[uil--phone]"]')).not.toBeNull();
    expect(pulseIcon).not.toBeNull();
    expect(pulseIcon?.classList.contains('animate-ping')).toBe(true);
    expect(dmRow?.querySelector('[data-testid="room-call-participants"]')).not.toBeNull();
    expect(dmRow?.querySelectorAll('[data-testid="room-call-participant-avatar"]')).toHaveLength(1);
    expect(
      dmRow?.querySelector('[data-testid="room-call-participants"] [data-testid="bot-badge"]')
    ).toBeNull();
    expect(dmRow!.querySelector('[data-testid="room-call-participants"]')?.nextElementSibling).toBe(
      icon
    );
    expect(dmRow?.firstElementChild?.querySelector('[data-testid="room-call-icon"]')).toBeNull();
  });

  it('renders the active-call phone icon when participants are not loaded', async () => {
    mocks.activeCallRoomIds.add('dm-phone-only');

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/dm-phone-only"]')).toBeInTheDocument();
    const dmRow = q(container, '[href="/chat/-/dm-phone-only"]');
    const icon = dmRow?.querySelector('[data-testid="room-call-icon"]');
    expect(icon).not.toBeNull();
    expect(icon?.querySelector('[class~="icon-[uil--phone]"]')).not.toBeNull();
    expect(icon?.querySelector('[data-testid="active-call-pulse-icon"]')).not.toBeNull();
    expect(dmRow?.querySelector('[data-testid="room-call-participants"]')).toBeNull();
  });

  it('renders active-call channel rows with the pulse icon and participant avatars', async () => {
    mocks.activeCallRoomIds.add('channel-1');
    mocks.projectedCallParticipants.set('channel-1', [
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
    expect(icon).not.toBeNull();
    expect(icon?.querySelector('[class~="icon-[uil--phone]"]')).not.toBeNull();
    expect(pulseIcon).not.toBeNull();
    expect(pulseIcon?.classList.contains('animate-ping')).toBe(true);
    expect(leadingIcon?.textContent).toBe('#');
    expect(leadingIcon).not.toBe(icon);
    expect(channelRow?.querySelector('[data-testid="room-call-participants"]')).not.toBeNull();
    expect(
      channelRow?.querySelectorAll('[data-testid="room-call-participant-avatar"]')
    ).toHaveLength(1);
    expect(
      channelRow!.querySelector('[data-testid="room-call-participants"]')?.nextElementSibling
    ).toBe(icon);
  });

  it('renders a compact overflow count for larger active calls', async () => {
    mocks.activeCallRoomIds.add('channel-1');
    mocks.projectedCallParticipants.set('channel-1', [
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
    expect(mocks.appUi.requestRoomSidebarPanel).toHaveBeenCalledWith(
      'origin',
      'channel-1',
      'call',
      'desktop'
    );
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
    expect(mocks.appUi.requestRoomSidebarPanel).toHaveBeenCalledWith(
      'origin',
      'dm-with-participants',
      'call',
      'desktop'
    );
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
      expect(mocks.appUi.requestRoomSidebarPanel).toHaveBeenCalledWith(
        'origin',
        'channel-1',
        'call',
        'desktop'
      );
    }
  );

  it('hides unjoined rooms until expanded and can hide them again', async () => {
    const { container } = render(RoomList);
    const more = q(container, '[data-testid="room-group-more"]');
    await expect.element(more).toHaveTextContent('2 more');
    expect(container.querySelector('[href="/chat/-/joinable-channel"]')).toBeNull();
    await expect.element(q(container, '[href="/chat/-/channel-1"]')).toBeInTheDocument();
    more?.click();
    await expect.element(more).toHaveAttribute('aria-expanded', 'true');
    await expect.element(more).toHaveTextContent('Show less');
    await vi.waitFor(() =>
      expect(container.querySelector('[href="/chat/-/joinable-channel"]')).not.toBeNull()
    );
    more?.click();
    await expect.element(more).toHaveAttribute('aria-expanded', 'false');
    await expect
      .poll(() => container.querySelector('[href="/chat/-/joinable-channel"]'))
      .toBeNull();
  });

  it('expands groups independently and omits the control for joined-only groups', async () => {
    mocks.store.navigation.roomGroups = [
      {
        id: 'discovery-first',
        name: 'Projects',
        viewerCanManageGroup: false,
        roomIds: ['joinable-channel']
      },
      {
        id: 'discovery-second',
        name: 'Gaming',
        viewerCanManageGroup: false,
        roomIds: ['restricted-channel']
      },
      {
        id: 'discovery-joined',
        name: 'Joined',
        viewerCanManageGroup: false,
        roomIds: ['channel-1']
      }
    ];
    const { container } = render(RoomList);
    const sections = container.querySelectorAll('[data-testid="room-group-section"]');
    const first = sections[0]?.querySelector<HTMLButtonElement>('[data-testid="room-group-more"]');
    const second = sections[1]?.querySelector<HTMLButtonElement>('[data-testid="room-group-more"]');
    expect(sections[2]?.querySelector('[data-testid="room-group-more"]')).toBeNull();
    first?.click();
    await expect.element(first).toHaveAttribute('aria-expanded', 'true');
    await expect.element(second).toHaveAttribute('aria-expanded', 'false');
    await expect
      .poll(() => container.querySelector('[href="/chat/-/joinable-channel"]'))
      .not.toBeNull();
    expect(container.querySelector('[href="/chat/-/restricted-channel"]')).toBeNull();
    second?.click();
    await expect
      .poll(() => container.querySelector('[href="/chat/-/restricted-channel"]'))
      .not.toBeNull();
  });

  it('hides a single unjoined room behind the disclosure', async () => {
    mocks.store.navigation.roomGroups = [
      {
        id: 'single',
        name: 'Projects',
        viewerCanManageGroup: false,
        roomIds: ['channel-1', 'joinable-channel']
      }
    ];
    const { container } = render(RoomList);
    expect(container.querySelector('[href="/chat/-/joinable-channel"]')).toBeNull();
    const more = q(container, '[data-testid="room-group-more"]');
    await expect.element(more).toHaveTextContent('1 more');
    more?.click();
    await expect.element(more).toHaveAttribute('aria-expanded', 'true');
    await expect
      .poll(() => container.querySelector('[href="/chat/-/joinable-channel"]'))
      .not.toBeNull();
  });

  it('keeps the current unjoined room visible and hides the remaining room', async () => {
    mocks.activeRoomId = 'joinable-channel';
    const { container } = render(RoomList);
    await expect.element(q(container, '[href="/chat/-/joinable-channel"]')).toBeInTheDocument();
    expect(container.querySelector('[href="/chat/-/restricted-channel"]')).toBeNull();
    await expect
      .element(q(container, '[data-testid="room-group-more"]'))
      .toHaveTextContent('1 more');
  });

  it('lets faded joinable non-member channel rows navigate to the room route', async () => {
    const { container } = render(RoomList);

    q(container, '[data-testid="room-group-more"]')?.click();
    await vi.waitFor(() =>
      expect(container.querySelector('[href="/chat/-/joinable-channel"]')).not.toBeNull()
    );
    const row = q(container, '[href="/chat/-/joinable-channel"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    expect(row.className).toContain('opacity-60');

    expect(row.getAttribute('href')).toBe('/chat/-/joinable-channel');
    expect(row.getAttribute('aria-disabled')).toBeNull();
    expect(mocks.pushState).not.toHaveBeenCalled();
  });

  it('lets faded non-joinable channel rows navigate to the inline access screen', async () => {
    const { container } = render(RoomList);

    q(container, '[data-testid="room-group-more"]')?.click();
    await vi.waitFor(() =>
      expect(container.querySelector('[href="/chat/-/restricted-channel"]')).not.toBeNull()
    );
    const row = q(container, '[href="/chat/-/restricted-channel"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    expect(row.className).toContain('opacity-60');
    const icon = row.querySelector('.sidebar-icon');
    expect(icon?.classList.contains('icon-[uil--lock]')).toBe(true);
    expect(row.querySelectorAll('[class~="icon-[uil--lock]"]')).toHaveLength(1);

    expect(row.getAttribute('href')).toBe('/chat/-/restricted-channel');
    expect(row.getAttribute('aria-disabled')).toBeNull();
    expect(mocks.pushState).not.toHaveBeenCalled();
  });

  it('renders unread channel rows and icons in full-contrast text', async () => {
    setRoomUnread('channel-1', true);

    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    const icon = row.querySelector('.sidebar-icon');
    expect(row.classList.contains('sidebar-item-attention')).toBe(true);
    expect(icon?.classList.contains('text-text-top')).toBe(true);
    expect(icon?.classList.contains('text-muted')).toBe(false);
  });

  it('marks the current room with the shared action-coloured route treatment', async () => {
    mocks.activeRoomId = 'channel-1';

    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    await expect.element(row).toHaveAttribute('aria-current', 'page');
    expect(row.classList.contains('sidebar-item')).toBe(true);
    expect(row.classList.contains('sidebar-item-current')).toBe(false);
    expect(row.querySelector('.sidebar-icon')?.classList.contains('text-muted')).toBe(true);
  });

  it('reveals the selected channel row with the browser nearest alignment', async () => {
    mocks.activeRoomId = 'channel-1';
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');

    try {
      render(RoomList);

      await vi.waitFor(() =>
        expect(scrollIntoView).toHaveBeenCalledWith({ block: 'nearest', inline: 'nearest' })
      );
    } finally {
      scrollIntoView.mockRestore();
    }
  });

  it('reveals the selected row when the room list finishes loading', async () => {
    const navigation = mocks.store.navigation;
    const roomDescriptor = Object.getOwnPropertyDescriptor(navigation, 'rooms')!;
    const loadingDescriptor = Object.getOwnPropertyDescriptor(navigation, 'isInitialLoading')!;
    const loadedRooms = navigation.rooms;
    const rooms = new SvelteMap<string, typeof loadedRooms>();
    const loading = new SvelteMap([['value', true]]);
    Object.defineProperty(navigation, 'rooms', {
      configurable: true,
      get: () => rooms.get('value') ?? []
    });
    Object.defineProperty(navigation, 'isInitialLoading', {
      configurable: true,
      get: () => loading.get('value') ?? false
    });
    mocks.activeRoomId = 'channel-1';
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');

    try {
      const { container } = render(RoomList);
      await tick();
      expect(scrollIntoView).not.toHaveBeenCalled();

      rooms.set('value', loadedRooms);
      loading.set('value', false);
      await vi.waitFor(() => {
        expect(q(container, 'a[aria-current="page"]')).not.toBeNull();
        expect(scrollIntoView).toHaveBeenCalledTimes(1);
      });
    } finally {
      scrollIntoView.mockRestore();
      Object.defineProperty(navigation, 'rooms', roomDescriptor);
      Object.defineProperty(navigation, 'isInitialLoading', loadingDescriptor);
    }
  });

  it('reveals a room selected through route navigation', async () => {
    mocks.activeRoomId = 'channel-1';
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');

    try {
      const { container } = render(RoomList);
      await expect.element(q(container, 'a[aria-current="page"]')).toBeInTheDocument();
      await vi.waitFor(() => expect(scrollIntoView).toHaveBeenCalledTimes(1));

      activeRoomRoute.set('roomId', 'dm-phone-only');
      const dmRow = q(container, '[href="/chat/-/dm-phone-only"]');
      await expect.element(dmRow).toHaveAttribute('aria-current', 'page');
      await vi.waitFor(() => expect(scrollIntoView).toHaveBeenCalledTimes(2));
      expect(scrollIntoView.mock.instances[1]).toBe(dmRow);
    } finally {
      scrollIntoView.mockRestore();
    }
  });

  it('reveals the selected DM in a collapsed section', async () => {
    mocks.activeRoomId = 'dm-with-participants';
    localStorage.setItem('chatto:i:origin:collapsible:dms', '1');
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');

    try {
      const { container } = render(RoomList);

      await vi.waitFor(() => {
        expect(q(container, '[href="/chat/-/dm-with-participants"]')).not.toBeNull();
        expect(scrollIntoView).toHaveBeenCalledTimes(1);
      });
    } finally {
      scrollIntoView.mockRestore();
    }
  });

  it('leaves an already visible selected row at its current scroll position', async () => {
    mocks.activeRoomId = 'channel-1';
    const { container } = render(RoomList);
    container.style.height = '800px';
    container.style.overflowY = 'auto';
    expect(container.scrollTop).toBe(0);

    activeRoomRoute.set('roomId', 'dm-with-participants');
    await expect
      .element(q(container, '[href="/chat/-/dm-with-participants"]'))
      .toHaveAttribute('aria-current', 'page');
    expect(container.scrollTop).toBe(0);
  });

  it('scrolls an offscreen selected DM into the sidebar viewport', async () => {
    mocks.activeRoomId = 'channel-1';
    const { container } = render(RoomList);
    container.style.height = '72px';
    container.style.overflowY = 'auto';
    const pageScroll = window.scrollY;
    expect(container.scrollHeight).toBeGreaterThan(container.clientHeight);

    activeRoomRoute.set('roomId', 'dm-phone-only');

    await vi.waitFor(() => expect(container.scrollTop).toBeGreaterThan(0));
    expect(window.scrollY).toBe(pageScroll);
  });

  it('waits while a desktop sidebar is closed', async () => {
    mocks.activeRoomId = 'channel-1';
    sidebarNav.toggle();
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');

    try {
      const { container } = render(RoomList);
      await expect.element(q(container, 'a[aria-current="page"]')).toBeInTheDocument();
      await tick();
      expect(scrollIntoView).not.toHaveBeenCalled();

      sidebarNav.toggle();
      await vi.waitFor(() => expect(scrollIntoView).toHaveBeenCalledTimes(1));
    } finally {
      scrollIntoView.mockRestore();
    }
  });

  it('positions a mobile drawer without opening it', async () => {
    mocks.activeRoomId = 'dm-with-participants';
    sidebarNav.setMobile(true);
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');

    try {
      render(RoomList);

      await vi.waitFor(() => expect(scrollIntoView).toHaveBeenCalledTimes(1));
      expect(sidebarNav.isOpen).toBe(false);
    } finally {
      scrollIntoView.mockRestore();
    }
  });

  it('does not scroll when the selected room has no sidebar row', async () => {
    mocks.activeRoomId = 'missing-room';
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');

    try {
      render(RoomList);

      await tick();
      expect(scrollIntoView).not.toHaveBeenCalled();
    } finally {
      scrollIntoView.mockRestore();
    }
  });

  it('does not scroll when the room list is empty', async () => {
    mocks.store.navigation.rooms = [];
    mocks.activeRoomId = 'missing-room';
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');

    try {
      render(RoomList);

      await tick();
      expect(scrollIntoView).not.toHaveBeenCalled();
    } finally {
      scrollIntoView.mockRestore();
    }
  });

  it('uses the established globe icon for universal joined rooms', async () => {
    const universal = mocks.store.navigation.rooms.find(
      (room: { id: string }) => room.id === 'channel-1'
    ) as unknown as { isUniversal: boolean };
    universal.isUniversal = true;

    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    const icon = q(row, '[class~="icon-[uil--globe]"]');
    await expect.element(icon).toHaveAttribute('aria-label', 'Universal');
    expect(icon?.getAttribute('title')).toBeTruthy();
  });

  it('renders room groups as sections and keeps notification rooms visible while collapsed', async () => {
    setRoomNotificationCount('channel-1', 1);
    mocks.store.navigation.roomGroups = [
      {
        id: 'community',
        name: 'Community',
        viewerCanManageGroup: false,
        roomIds: ['channel-1', 'joinable-channel']
      }
    ];
    localStorage.setItem('chatto:i:origin:collapsible:set:community', '1');

    const { container } = render(RoomList);

    await expect.element(q(container, '[data-testid="room-group-section"]')).toBeInTheDocument();
    await expect
      .element(q(container, '[data-testid="room-group-section"] button'))
      .toHaveAttribute('aria-expanded', 'false');
    await expect.element(q(container, '[href="/chat/-/channel-1"]')).toBeInTheDocument();
    expect(container.querySelector('[href="/chat/-/joinable-channel"]')).toBeNull();
  });

  it('resolves server-local non-chat sidebar links against the active server and opens externally', async () => {
    mocks.store.navigation.roomGroups = [
      {
        id: 'g1',
        name: 'Links',
        viewerCanManageGroup: false,
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
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  it.each([
    [true, true, ['Create room', 'Create link']],
    [true, false, ['Create room']],
    [false, true, ['Create link']],
    [false, false, []]
  ] as const)(
    'gates the creation menu with room=%s and manage=%s',
    async (room, manage, labels) => {
      mocks.store.navigation.roomGroups = [
        {
          id: 'creation-permissions',
          name: 'Projects',
          roomIds: ['channel-1'],
          viewerCanCreateRoom: room,
          viewerCanManageGroup: manage
        }
      ];
      const { container } = render(RoomList);
      const trigger = container.querySelector<HTMLButtonElement>(
        '[data-testid="room-group-create-button"]'
      );
      if (labels.length === 0) {
        expect(trigger).toBeNull();
        return;
      }
      await expect.element(trigger).toBeVisible();
      expect(getComputedStyle(trigger!).opacity).toBe('1');
      await userEvent.click(trigger!);
      const menu = document.querySelector<HTMLElement>(
        '[role="menu"][aria-label="Add to Projects"]'
      );
      await expect.element(menu).toBeVisible();
      expect(
        Array.from(menu!.querySelectorAll('[role="menuitem"]')).map((item) =>
          item.textContent?.trim()
        )
      ).toEqual(labels);
      await expect.element(trigger).toHaveAttribute('aria-expanded', 'true');
    }
  );

  it('updates creation controls and an open menu when projected group permissions change', async () => {
    const originalNavigation = mocks.store.navigation;
    const projection = new ServerProjectionStore();
    const setPermissions = (create: boolean, manage: boolean) => {
      projection.roomGroups = [
        new RoomGroup({
          id: 'reactive-group',
          name: 'Projects',
          viewerState: new RoomGroupViewerState({
            permissions: [
              new PermissionGrant({ permission: 'room.create', granted: create }),
              new PermissionGrant({ permission: 'room.manage', granted: manage })
            ]
          })
        })
      ];
    };
    mocks.store.navigation = new NavigationStore(
      new RoomListView(projection, { hasUsableProjection: true }),
      () => ({
        unreadNotificationCount: 0,
        importantUnreadNotificationCount: 0,
        roomUnreadCounts: {},
        roomImportantUnreadCounts: {}
      })
    ) as unknown as typeof originalNavigation;
    try {
      setPermissions(false, false);
      const { container } = render(RoomList);
      expect(container.querySelector('[data-testid="room-group-create-button"]')).toBeNull();
      setPermissions(true, true);
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="room-group-create-button"]')).not.toBeNull()
      );
      const trigger = q(container, '[data-testid="room-group-create-button"]') as HTMLButtonElement;
      expect(getComputedStyle(trigger).backgroundColor).toBe('rgba(0, 0, 0, 0)');
      expect(trigger.getBoundingClientRect().height).toBeLessThanOrEqual(24);
      await userEvent.click(trigger);
      await expect.element(document.querySelector<HTMLElement>('[role="menu"]')).toBeVisible();
      setPermissions(false, true);
      await vi.waitFor(() =>
        expect(
          Array.from(document.querySelectorAll('[role="menuitem"]')).map((item) =>
            item.textContent?.trim()
          )
        ).toEqual(['Create link'])
      );
      setPermissions(false, false);
      await vi.waitFor(() => {
        expect(document.querySelector('[role="menu"]')).toBeNull();
        expect(container.querySelector('[data-testid="room-group-create-button"]')).toBeNull();
      });
      setPermissions(true, true);
      await vi.waitFor(() =>
        expect(container.querySelector('[data-testid="room-group-create-button"]')).not.toBeNull()
      );
      expect(document.querySelector('[role="menu"]')).toBeNull();
    } finally {
      mocks.store.navigation = originalNavigation;
    }
  });

  it('opens the creation menu by keyboard in a collapsed group and restores focus on dismissal', async () => {
    mocks.store.navigation.roomGroups = [
      {
        id: 'creation-keyboard',
        name: 'Projects',
        roomIds: ['channel-1'],
        viewerCanCreateRoom: true,
        viewerCanManageGroup: true
      }
    ];
    const { container } = render(RoomList, { props: { canReorderGroups: true } });
    const header = q(container, '[data-testid="room-group-section"] button') as HTMLButtonElement;
    await userEvent.click(header);
    await expect.element(header).toHaveAttribute('aria-expanded', 'false');
    const trigger = q(container, '[data-testid="room-group-create-button"]') as HTMLButtonElement;
    await expect.element(trigger).toBeVisible();
    trigger.focus();
    await userEvent.keyboard('{Enter}');
    const menu = document.querySelector<HTMLElement>('[role="menu"][aria-label="Add to Projects"]');
    await expect.element(menu).toBeVisible();
    const items = menu!.querySelectorAll<HTMLButtonElement>('[role="menuitem"]');
    await expect.element(items[0]).toHaveFocus();
    await userEvent.keyboard('{ArrowDown}');
    await expect.element(items[1]).toHaveFocus();
    await userEvent.keyboard('{Escape}');
    await expect.element(trigger).toHaveFocus();
    await expect.element(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(document.querySelector('[role="menu"]')).toBeNull();
    await userEvent.keyboard(' ');
    await expect.element(document.querySelector<HTMLElement>('[role="menu"]')).toBeVisible();
    await userEvent.click(header);
    expect(document.querySelector('[role="menu"]')).toBeNull();
    expect(mocks.layoutAPI.moveRoomGroup).not.toHaveBeenCalled();
    expect(mocks.layoutAPI.moveSidebarItem).not.toHaveBeenCalled();
  });

  it('creates and joins a room in the group selected through the creation menu', async () => {
    mocks.roomCommandAPI.createRoom.mockResolvedValueOnce({ id: 'created-room' });
    mocks.store.navigation.roomGroups = ['first', 'second'].map((id) => ({
      id,
      name: id,
      roomIds: [],
      viewerCanCreateRoom: true,
      viewerCanManageGroup: true
    }));
    const { container } = render(RoomList);
    const trigger = q(container, '[aria-label="Add to second"]') as HTMLButtonElement;
    await userEvent.click(trigger);
    const newRoom = Array.from(
      document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    ).find((item) => item.textContent?.trim() === 'Create room')!;
    await userEvent.click(newRoom);
    await vi.waitFor(() => expect(document.querySelector('#room-name')).not.toBeNull());
    const input = document.querySelector<HTMLInputElement>('#room-name')!;
    input.value = 'new-project';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    const submit = document.querySelector<HTMLButtonElement>('dialog button[type="submit"]')!;
    await expect.element(submit).toBeEnabled();
    await userEvent.click(submit);
    await vi.waitFor(() =>
      expect(mocks.roomCommandAPI.createRoom).toHaveBeenCalledWith(
        expect.objectContaining({
          groupId: 'second',
          name: 'new-project'
        })
      )
    );
    expect(mocks.roomCommandAPI.joinRoom).toHaveBeenCalledWith('created-room');
    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/created-room');
    expect(document.querySelector('[role="menu"]')).toBeNull();
  });

  it.each(['context menu', 'creation menu'])(
    'adds HTTPS to a new sidebar link from the %s',
    async (entry) => {
      mocks.store.navigation.rooms = [];
      mocks.store.navigation.roomGroups = [
        {
          id: 'resources',
          name: 'Resources',
          viewerCanManageGroup: true,
          roomIds: [],
          items: []
        }
      ];

      const { container } = render(RoomList);
      if (entry === 'creation menu') {
        await userEvent.click(
          q(container, '[data-testid="room-group-create-button"]') as HTMLButtonElement
        );
      } else {
        const groupHeader = Array.from(container.querySelectorAll('button')).find((button) =>
          button.textContent?.includes('Resources')
        );
        groupHeader!.dispatchEvent(
          new MouseEvent('contextmenu', {
            bubbles: true,
            cancelable: true,
            clientX: 40,
            clientY: 60
          })
        );

        await vi.waitFor(() =>
          expect(
            document.querySelector('[role="menu"][aria-label="Settings for Resources"]')
          ).not.toBeNull()
        );
      }
      const newLink = Array.from(document.querySelectorAll('button')).find(
        (button) => button.textContent?.trim() === 'Create link'
      );
      newLink!.click();

      await vi.waitFor(() => expect(document.querySelector('#sidebar-link-url')).not.toBeNull());
      const label = document.querySelector<HTMLInputElement>('#sidebar-link-label')!;
      const url = document.querySelector<HTMLInputElement>('#sidebar-link-url')!;
      label.value = 'Docs';
      label.dispatchEvent(new Event('input', { bubbles: true }));
      url.value = 'docs.example.test/guide';
      url.dispatchEvent(new Event('input', { bubbles: true }));

      const submit = Array.from(document.querySelectorAll<HTMLButtonElement>('button')).find(
        (button) => button.type === 'submit' && button.textContent?.trim() === 'Create link'
      );
      await expect.element(submit ?? null).toBeEnabled();
      submit!.click();

      await vi.waitFor(() =>
        expect(mocks.layoutAPI.createSidebarLink).toHaveBeenCalledWith({
          groupId: 'resources',
          label: 'Docs',
          url: 'https://docs.example.test/guide'
        })
      );
    }
  );

  it('keeps an empty manageable group visible and opens its settings from a context menu', async () => {
    mocks.store.navigation.rooms = [];
    mocks.store.navigation.roomGroups = [
      {
        id: 'private-group',
        name: 'Private Group',
        viewerCanManageGroup: true,
        roomIds: [],
        items: []
      }
    ];

    const { container } = render(RoomList);

    const groupHeader = Array.from(container.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Private Group')
    );
    await expect.element(groupHeader ?? null).toBeInTheDocument();
    expect(container.querySelector('[class~="icon-[uil--setting]"]')).toBeNull();

    groupHeader!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 40, clientY: 60 })
    );
    await vi.waitFor(() =>
      expect(
        document.querySelector('[role="menu"][aria-label="Settings for Private Group"]')
      ).not.toBeNull()
    );

    const settings = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Open group settings'
    );
    await expect.element(settings ?? null).toBeInTheDocument();
    await expect
      .element(
        Array.from(document.querySelectorAll('button')).find(
          (button) => button.textContent?.trim() === 'Delete group'
        ) ?? null
      )
      .toBeInTheDocument();

    settings!.click();
    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/manage/room-groups/private-group');
  });

  it('disables deletion for a room group that contains a sidebar link', async () => {
    mocks.store.navigation.rooms = [];
    mocks.store.navigation.roomGroups = [
      {
        id: 'resources',
        name: 'Resources',
        viewerCanManageGroup: true,
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
    const groupHeader = Array.from(container.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Resources')
    );
    groupHeader!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 40, clientY: 60 })
    );

    await vi.waitFor(() =>
      expect(
        document.querySelector('[role="menu"][aria-label="Settings for Resources"]')
      ).not.toBeNull()
    );
    const deleteGroup = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Delete group'
    );
    await expect.element(deleteGroup ?? null).toBeDisabled();
    deleteGroup!.click();
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  it('disables deletion for a room group that contains a legacy room entry', async () => {
    mocks.store.navigation.rooms = [];
    mocks.store.navigation.roomGroups = [
      {
        id: 'private-rooms',
        name: 'Private Rooms',
        viewerCanManageGroup: true,
        roomIds: ['hidden-room']
      }
    ];

    const { container } = render(RoomList);
    const groupHeader = Array.from(container.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Private Rooms')
    );
    groupHeader!.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 40, clientY: 60 })
    );

    await vi.waitFor(() =>
      expect(
        document.querySelector('[role="menu"][aria-label="Settings for Private Rooms"]')
      ).not.toBeNull()
    );
    const deleteGroup = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Delete group'
    );
    await expect.element(deleteGroup ?? null).toBeDisabled();
  });

  it.each([
    { name: 'without reorder permission', canReorderGroups: false },
    { name: 'with reorder permission', canReorderGroups: true }
  ])('keeps a room group indicator visible $name', async ({ name, canReorderGroups }) => {
    mocks.store.navigation.roomGroups = [
      {
        id: `indicator-${name}`,
        name: 'Projects',
        viewerCanManageGroup: canReorderGroups,
        viewerCanCreateRoom: canReorderGroups,
        roomIds: ['channel-1']
      }
    ];

    const { container, getByRole } = render(RoomList, { props: { canReorderGroups } });
    const heading = getByRole('button', { name: 'Projects', exact: true });
    const icon = q(container, '[data-testid="room-group-disclosure-icon"]')!;
    const handle = q(container, '[data-testid="room-group-drag-handle"]');
    const hasOverlay = canReorderGroups;

    expect(Boolean(handle)).toBe(hasOverlay);
    await userEvent.unhover(heading);
    await expect.poll(() => getComputedStyle(icon).opacity).toBe('1');
    // Wait for the header's hover transition before sampling its resting colour.
    await Promise.all(
      heading
        .element()
        .parentElement!.getAnimations({ subtree: true })
        .map((animation) => animation.finished)
    );
    const mutedColour = getComputedStyle(heading.element()).color;

    for (const expanded of [true, false]) {
      await expect.element(heading).toHaveAttribute('aria-expanded', String(expanded));
      await userEvent.hover(heading);
      await expect.poll(() => getComputedStyle(heading.element()).color).not.toBe(mutedColour);
      await expect.poll(() => getComputedStyle(icon).opacity).toBe(hasOverlay ? '0' : '1');
      if (handle) await expect.poll(() => getComputedStyle(handle).opacity).toBe('1');

      await userEvent.unhover(heading);
      heading.element().focus();
      await userEvent.tab();
      await userEvent.tab({ shift: true });
      await expect.element(heading).toHaveFocus();
      await expect.poll(() => getComputedStyle(heading.element()).color).not.toBe(mutedColour);
      await expect.poll(() => getComputedStyle(icon).opacity).toBe(hasOverlay ? '0' : '1');
      if (handle) await expect.poll(() => getComputedStyle(handle).opacity).toBe('1');

      await userEvent.keyboard(' ');
      await expect.element(heading).toHaveAttribute('aria-expanded', String(!expanded));
      heading.element().blur();
    }

    await userEvent.click(heading);
    await userEvent.unhover(heading);
    await expect.element(heading).toHaveFocus();
    await expect.poll(() => getComputedStyle(heading.element()).color).toBe(mutedColour);
  });

  it('shows permission-gated drag and creation controls without room or group menu buttons', async () => {
    const channel = mocks.store.navigation.rooms.find(
      (room: { id: string }) => room.id === 'channel-1'
    );
    mocks.store.navigation.rooms = [channel] as never;
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: true,
        viewerCanCreateRoom: true,
        roomIds: ['channel-1'],
        items: [
          { id: 'room:channel-1', type: 'room', roomId: 'channel-1' },
          {
            id: 'link:docs',
            type: 'link',
            link: { id: 'docs', label: 'Docs', url: '/docs' }
          }
        ]
      }
    ];

    const { container } = render(RoomList, { props: { canReorderGroups: true } });

    await expect
      .element(q(container, '[data-testid="room-group-create-button"]'))
      .toBeInTheDocument();
    await expect
      .element(q(container, '[data-testid="room-group-drag-handle"]'))
      .toBeInTheDocument();
    expect(container.querySelector('[data-testid="room-group-actions-button"]')).toBeNull();
    await expect.element(q(container, '[data-testid="room-drag-handle"]')).toBeInTheDocument();
    expect(container.querySelector('[data-testid="room-actions-button"]')).toBeNull();
    await expect
      .element(q(container, '[data-testid="sidebar-link-drag-handle"]'))
      .toBeInTheDocument();
    expect(container.querySelector('[data-testid="sidebar-link-actions-button"]')).toBeNull();
    const linkLeadingIcon = q(container, '[data-testid="sidebar-link-leading-icon"]');
    expect(q(container, '[data-testid="sidebar-link-drag-handle"]')?.parentElement).toBe(
      linkLeadingIcon
    );
  });

  it('places the new-group control directly after managed groups and before direct messages', () => {
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: true,
        viewerCanCreateRoom: true,
        roomIds: ['channel-1'],
        items: [{ id: 'room:channel-1', type: 'room', roomId: 'channel-1' }]
      }
    ];

    const { container } = render(RoomList, { props: { canReorderGroups: true } });
    const control = q(container, '[data-testid="create-room-group-control"]');
    if (!control) throw new Error('Expected the new-group control');

    expect(control.previousElementSibling?.getAttribute('data-testid')).toBe(
      'room-groups-dropzone'
    );
    expect(control.previousElementSibling?.classList).toContain('sidebar-drop-target');
    expect(control.nextElementSibling?.getAttribute('data-testid')).toBe('room-group-section');
  });

  it('starts room-group dragging only from the group drag handle', async () => {
    mocks.store.navigation.rooms = [];
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: true,
        roomIds: [],
        items: []
      },
      {
        id: 'operations',
        name: 'Operations',
        viewerCanManageGroup: true,
        roomIds: [],
        items: []
      }
    ];
    const { container } = render(RoomList, { props: { canReorderGroups: true } });
    const header = Array.from(
      container.querySelectorAll<HTMLButtonElement>(
        'button[aria-expanded]:not([data-testid="room-group-more"])'
      )
    ).find((button) => button.textContent?.trim() === 'Projects');
    const title = header?.querySelector(':scope > span:last-child');
    expect(title).not.toBeNull();

    title!.dispatchEvent(
      new MouseEvent('mousedown', {
        bubbles: true,
        cancelable: true,
        button: 0,
        clientX: 20,
        clientY: 20
      })
    );
    window.dispatchEvent(
      new MouseEvent('mousemove', {
        bubbles: true,
        cancelable: true,
        button: 0,
        clientX: 50,
        clientY: 50
      })
    );
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

    expect(document.querySelector('#dnd-action-dragged-el')).toBeNull();
    window.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, button: 0 }));
  });

  it('keeps room groups mounted while a room drag updates its drop zones', async () => {
    const channel = mocks.store.navigation.rooms.find(
      (room: { id: string }) => room.id === 'channel-1'
    );
    mocks.store.navigation.rooms = [channel] as never;
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: true,
        roomIds: ['channel-1'],
        items: [{ id: 'room:channel-1', type: 'room', roomId: 'channel-1' }]
      },
      {
        id: 'operations',
        name: 'Operations',
        viewerCanManageGroup: true,
        roomIds: [],
        items: []
      }
    ];
    const { container } = render(RoomList, { props: { canReorderGroups: true } });
    const handle = q(container, '[data-testid="room-drag-handle"]') as HTMLButtonElement;
    const target = handle.querySelector('span') ?? handle;

    target.dispatchEvent(
      new MouseEvent('mousedown', {
        bubbles: true,
        cancelable: true,
        button: 0,
        clientX: 20,
        clientY: 20
      })
    );
    window.dispatchEvent(
      new MouseEvent('mousemove', {
        bubbles: true,
        cancelable: true,
        button: 0,
        clientX: 50,
        clientY: 50
      })
    );
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

    expect(document.querySelector('#dnd-action-dragged-el')).not.toBeNull();
    expect(container.querySelectorAll('[data-testid="room-group-section"]')).toHaveLength(2);
    expect(container.textContent).toContain('Projects');
    expect(container.textContent).toContain('Operations');

    window.dispatchEvent(
      new MouseEvent('mouseup', { bubbles: true, button: 0, clientX: 50, clientY: 50 })
    );
    await new Promise((resolve) => setTimeout(resolve, 250));
  });

  it('does not treat bubbling room drag events as room-group drag events', async () => {
    const channel = mocks.store.navigation.rooms.find(
      (room: { id: string }) => room.id === 'channel-1'
    );
    mocks.store.navigation.rooms = [channel] as never;
    const roomItem = { id: 'room:channel-1', type: 'room' as const, roomId: 'channel-1' };
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: true,
        roomIds: ['channel-1'],
        items: [roomItem]
      },
      {
        id: 'operations',
        name: 'Operations',
        viewerCanManageGroup: true,
        roomIds: [],
        items: []
      }
    ];
    const { container } = render(RoomList, { props: { canReorderGroups: true } });
    const itemDropzone = q(container, '[data-testid="room-group-items-dropzone"]') as HTMLElement;

    itemDropzone.dispatchEvent(
      new CustomEvent('consider', {
        bubbles: true,
        detail: { items: [roomItem], info: { id: roomItem.id } }
      })
    );
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

    expect(container.querySelectorAll('[data-testid="room-group-section"]')).toHaveLength(2);
    expect(container.textContent).toContain('Projects');
    expect(container.textContent).toContain('Operations');
  });

  it('keeps an empty group visible when the viewer can create rooms in it', async () => {
    mocks.store.navigation.rooms = [];
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: false,
        viewerCanCreateRoom: true,
        roomIds: [],
        items: []
      }
    ];

    const { container } = render(RoomList);

    await expect.element(q(container, '[data-testid="room-group-section"]')).toBeInTheDocument();
    await expect
      .element(q(container, '[data-testid="room-group-create-button"]'))
      .toBeInTheDocument();
  });

  it('omits drag handles and zones on touch-only devices so touches reach the rows', async () => {
    const matchMedia = window.matchMedia.bind(window);
    const spy = vi.spyOn(window, 'matchMedia').mockImplementation((query) =>
      query === TOUCH_ONLY_QUERY
        ? ({
            matches: true,
            media: query,
            onchange: null,
            addEventListener: () => {},
            removeEventListener: () => {},
            addListener: () => {},
            removeListener: () => {},
            dispatchEvent: () => false
          } satisfies MediaQueryList)
        : matchMedia(query)
    );
    try {
      mocks.store.navigation.roomGroups = [
        {
          id: 'projects',
          name: 'Projects',
          viewerCanManageGroup: true,
          viewerCanCreateRoom: true,
          roomIds: ['channel-1'],
          items: [
            { id: 'room:channel-1', type: 'room', roomId: 'channel-1' },
            {
              id: 'link:docs',
              type: 'link',
              link: { id: 'docs', label: 'Docs', url: '/docs' }
            }
          ]
        }
      ];

      const { container } = render(RoomList, { props: { canReorderGroups: true } });

      await expect
        .element(q(container, '[data-testid="room-group-disclosure-icon"]'))
        .toBeInTheDocument();
      expect(container.querySelector('[data-testid="room-group-drag-handle"]')).toBeNull();
      expect(container.querySelector('[data-testid="room-drag-handle"]')).toBeNull();
      expect(container.querySelector('[data-testid="sidebar-link-drag-handle"]')).toBeNull();
      expect(container.querySelector('[data-testid="room-group-items-dropzone"]')).toBeNull();
      expect(container.querySelector('[data-testid="room-groups-dropzone"]')).toBeNull();

      // Without drag zones, touches on rows must still reach app-shell gestures
      // such as the mobile sidebar swipe.
      const reachedContainer = vi.fn();
      container.addEventListener('touchstart', reachedContainer);
      q(container, '[data-testid="sidebar-link-leading-icon"]')?.dispatchEvent(
        new Event('touchstart', { bubbles: true })
      );
      expect(reachedContainer).toHaveBeenCalledOnce();
    } finally {
      spy.mockRestore();
    }
  });

  it('persists a relative sidebar-item placement after a handled drop', async () => {
    const channel = mocks.store.navigation.rooms.find(
      (room: { id: string }) => room.id === 'channel-1'
    );
    mocks.store.navigation.rooms = [channel] as never;
    const roomItem = { id: 'room:channel-1', type: 'room' as const, roomId: 'channel-1' };
    const linkItem = {
      id: 'link:docs',
      type: 'link' as const,
      link: { id: 'docs', label: 'Docs', url: '/docs' }
    };
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: true,
        roomIds: ['channel-1'],
        items: [roomItem, linkItem]
      }
    ];
    const { container } = render(RoomList);
    const dropzone = q(container, '[data-testid="room-group-items-dropzone"]') as HTMLElement;

    dropzone.dispatchEvent(
      new CustomEvent('finalize', {
        detail: { items: [linkItem, roomItem], info: { id: linkItem.id } }
      })
    );

    await vi.waitFor(() => {
      expect(mocks.layoutAPI.moveSidebarItem).toHaveBeenCalledWith({
        item: { kind: 'link', id: 'docs' },
        groupId: 'projects',
        before: { kind: 'room', id: 'channel-1' }
      });
    });
  });

  it('persists a relative room-group placement after a handled drop', async () => {
    mocks.store.navigation.rooms = [];
    const first = {
      id: 'first',
      name: 'First',
      viewerCanManageGroup: true,
      roomIds: [],
      items: []
    };
    const second = {
      id: 'second',
      name: 'Second',
      viewerCanManageGroup: true,
      roomIds: [],
      items: []
    };
    mocks.store.navigation.roomGroups = [first, second];
    const { container } = render(RoomList, { props: { canReorderGroups: true } });
    const dropzone = q(container, '[data-testid="room-groups-dropzone"]') as HTMLElement;
    const keepVisibleWhenCollapsed = () => false;
    const movedSection = {
      id: 'group:second',
      label: second.name,
      items: [],
      persistKey: 'second',
      keepVisibleWhenCollapsed,
      group: second
    };
    const nextSection = {
      id: 'group:first',
      label: first.name,
      items: [],
      persistKey: 'first',
      keepVisibleWhenCollapsed,
      group: first
    };

    dropzone.dispatchEvent(
      new CustomEvent('finalize', {
        detail: { items: [movedSection, nextSection], info: { id: movedSection.id } }
      })
    );

    await vi.waitFor(() => {
      expect(mocks.layoutAPI.moveRoomGroup).toHaveBeenCalledWith({
        groupId: 'second',
        beforeGroupId: 'first'
      });
    });
  });

  it('rewrites remote conversation sidebar links and navigates locally', async () => {
    mocks.store.navigation.roomGroups = [
      {
        id: 'g1',
        name: 'Links',
        viewerCanManageGroup: false,
        roomIds: [],
        items: [
          {
            id: 'link:chat',
            type: 'link',
            link: {
              id: 'chat',
              label: 'Chat',
              url: 'https://remote.example.test/chat/-/R123456789abcde'
            }
          }
        ]
      }
    ];
    const open = vi.spyOn(window, 'open').mockReturnValue(null);
    const { container } = render(RoomList);
    const link = q(
      container,
      '[href="/chat/remote.example.test/R123456789abcde"]'
    ) as HTMLAnchorElement;
    await expect.element(link).toBeInTheDocument();
    link.click();
    expect(mocks.goto).toHaveBeenCalledWith('/chat/remote.example.test/R123456789abcde');
    expect(open).not.toHaveBeenCalled();
    open.mockRestore();
  });

  it.each(['https://chat.example.test/admin', 'http://remote.example.test:8080/admin'])(
    'renders non-chat links on registered servers as external anchors: %s',
    async (url) => {
      mocks.store.navigation.roomGroups = [
        {
          id: 'g1',
          name: 'Links',
          viewerCanManageGroup: false,
          roomIds: [],
          items: [
            {
              id: 'link:admin',
              type: 'link',
              link: {
                id: 'admin',
                label: 'Admin',
                url
              }
            }
          ]
        }
      ];

      const { container } = render(RoomList);

      const link = q(container, `[href="${url}"]`) as HTMLAnchorElement;
      await expect.element(link).toBeInTheDocument();
      expect(link.getAttribute('target')).toBe('_blank');
      expect(link.getAttribute('rel')).toBe('noopener noreferrer');
    }
  );

  it('renders external sidebar links as new-tab anchors', async () => {
    mocks.store.navigation.roomGroups = [
      {
        id: 'g1',
        name: 'Links',
        viewerCanManageGroup: false,
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
    mocks.notificationPath.mockReturnValue('/chat/-/channel-1/thread-1');
    mocks.store.notifications.markRead.mockResolvedValue(true);

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="room-notification-badge"]');
    await expect.element(badge).toBeInTheDocument();
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.resolveRoomNotification).toHaveBeenCalledWith('channel-1', {
        isDM: false,
        attentionLevel: NotificationAttentionLevel.IMPORTANT
      });
      expect(mocks.store.pendingHighlights.set).toHaveBeenCalledWith(
        'channel-1',
        'thread-1',
        'event-1',
        'mention-1'
      );
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'channel-1');
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
      );
      expect(mocks.store.notifications.markRead).not.toHaveBeenCalled();
      expect(mocks.notificationPath).toHaveBeenCalledWith('origin', roomNotification);
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/channel-1/thread-1');
    });
  });

  it('uses a neutral room badge when only ambient notifications are unread', async () => {
    setRoomNotificationCount('channel-1', 2, 0);

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="room-ambient-notification-badge"]');
    await expect.element(badge).toHaveClass('bg-text');
    await expect.element(badge).toHaveTextContent('2');
    expect(q(container, '[data-testid="room-notification-badge"]')).toBeNull();
  });

  it('shows only the orange room badge when every notification is important', async () => {
    setRoomNotificationCount('channel-1', 3);

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="room-notification-badge"]');
    await expect.element(badge).toHaveClass('bg-attention');
    await expect.element(badge).toHaveTextContent('3');
    expect(q(container, '[data-testid="room-ambient-notification-badge"]')).toBeNull();
  });

  it('shows important and ambient counts side by side', async () => {
    setRoomNotificationCount('channel-1', 5, 2);

    const { container } = render(RoomList);

    const important = q(container, '[data-testid="room-notification-badge"]');
    const ambient = q(container, '[data-testid="room-ambient-notification-badge"]');
    await expect.element(important).toHaveClass('bg-attention');
    await expect.element(important).toHaveTextContent('2');
    await expect.element(ambient).toHaveClass('bg-text');
    await expect.element(ambient).toHaveTextContent('3');
    expect(
      ambient!.compareDocumentPosition(important!) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy();
  });

  it('opens an ambient notification from the neutral badge', async () => {
    setRoomNotificationCount('channel-1', 5, 2);
    mocks.store.notifications.resolveRoomNotification.mockResolvedValue({
      ok: true,
      totalCount: 3,
      notification: notification('reaction-1', 'channel-1')
    });
    mocks.notificationPath.mockReturnValue('/chat/-/channel-1');

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="room-ambient-notification-badge"]');
    await expect.element(badge).toBeInTheDocument();
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.resolveRoomNotification).toHaveBeenCalledWith('channel-1', {
        isDM: false,
        attentionLevel: NotificationAttentionLevel.AMBIENT
      });
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/channel-1');
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
    mocks.notificationPath.mockReturnValue('/chat/-/dm-with-participants');
    mocks.store.notifications.markRead.mockResolvedValue(true);

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="dm-notification-badge"]');
    await expect.element(badge).toBeInTheDocument();
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.resolveRoomNotification).toHaveBeenCalledWith(
        'dm-with-participants',
        { isDM: true, attentionLevel: NotificationAttentionLevel.IMPORTANT }
      );
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith(
        'origin',
        'dm-with-participants'
      );
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
      );
      expect(mocks.store.pendingHighlights.set).toHaveBeenCalledWith(
        'dm-with-participants',
        null,
        'event-1',
        'dm-1'
      );
      expect(mocks.store.notifications.markRead).not.toHaveBeenCalled();
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/dm-with-participants');
    });
  });

  it('leaves a stale room badge to converge through the authoritative projection', async () => {
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
        isDM: false,
        attentionLevel: NotificationAttentionLevel.IMPORTANT
      });
      expect(mocks.goto).not.toHaveBeenCalled();
      expect(mocks.store.notifications.markRead).not.toHaveBeenCalled();
    });
  });
});
