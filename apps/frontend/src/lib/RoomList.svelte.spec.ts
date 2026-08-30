import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';

import { NotificationSignalKind } from '$lib/api-client/notifications';
import type { RoomsListGroup } from '$lib/state/server/rooms.svelte';
import { getToasts, toast } from '$lib/ui/toast';

const { mocks } = vi.hoisted(() => ({
  mocks: {
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
      archiveRoom: vi.fn().mockResolvedValue(null)
    },
    appUi: {
      disableRoomCallWideFor: vi.fn(),
      requestRoomSidebarPanel: vi.fn()
    },
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
        markRead: vi.fn(),
        getCleanPath: vi.fn().mockReturnValue('/chat/-/room')
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
        livekitUrl: null,
        supportsFeature: vi.fn().mockReturnValue(true)
      },
      navigation: {
        rooms: [],
        roomGroups: [] as RoomsListGroup[],
        isInitialLoading: false,
        currentUserId: 'me'
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

vi.mock('$app/state', () => ({
  page: {
    params: {
      serverId: '-',
      get roomId() {
        return mocks.activeRoomId;
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
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: () => '-',
  segmentToServerId: () => 'origin'
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

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    isOriginServer: vi.fn(() => true),
    getServer: vi.fn(() => ({ id: 'origin', url: 'https://chat.example.test' })),
    originServer: { id: 'origin' },
    servers: [{ id: 'origin', url: 'https://chat.example.test' }]
  }
}));

vi.mock('$lib/state/appUi.svelte', () => ({
  getAppUiState: () => mocks.appUi,
  getRoomSidebarPresentation: () => 'desktop'
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: string) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
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
    presenceStatus: 'ONLINE'
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
  mocks.activeCallRoomIds = new Set();
  mocks.projectedCallParticipants = new Map();
  mocks.unreadRoomIds = new Set();
  mocks.store.navigation.roomGroups = [];
  mocks.store.navigation.isInitialLoading = false;
  mocks.store.navigation.currentUserId = 'me';
  mocks.store.serverInfo.supportsFeature.mockReturnValue(true);
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
  mocks.store.notifications.getCleanPath.mockReturnValue('/chat/-/room');
  mocks.store.roomDirectory.joinRoom.mockResolvedValue({ ok: true });
  mocks.markNavigationRoomAsRead.mockResolvedValue(true);
});

describe('RoomList', () => {
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
    const groupHeaders = container.querySelectorAll<HTMLButtonElement>('button[aria-expanded]');
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
    const groupHeaders = container.querySelectorAll<HTMLButtonElement>('button[aria-expanded]');
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

  it('renders a full-width separator between adjacent room and DM sections', () => {
    const { container } = render(RoomList);
    const roomList = q(container, 'nav.room-list') as HTMLElement;
    const groupHeaders = container.querySelectorAll<HTMLButtonElement>('button[aria-expanded]');
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

  it('shows Copy Room ID as the final context-menu row and copies the ID', async () => {
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;

    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() =>
      expect(document.querySelector('[data-testid="copy-room-id"]')).not.toBeNull()
    );

    const copyRoomId = q(document.body, '[data-testid="copy-room-id"]') as HTMLButtonElement;
    await expect.element(copyRoomId).toHaveTextContent('Copy Room ID');
    const menuItems = copyRoomId.closest('[role="menu"]')?.querySelectorAll('[role="menuitem"]');
    expect(menuItems?.item((menuItems?.length ?? 0) - 1)).toBe(copyRoomId);

    copyRoomId.click();

    await vi.waitFor(() => expect(mocks.writeClipboardText).toHaveBeenCalledWith('channel-1'));
    expect(getToasts().map((item) => item.message)).toContain('Copied to clipboard');
  });

  it('offers a join action for a visible non-member room', async () => {
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/joinable-channel"]') as HTMLAnchorElement;

    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Join Room'));

    const join = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Join Room'
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
    const row = q(container, '[href="/chat/-/joinable-channel"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Room settings'));

    const join = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Join Room'
    );
    const settings = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Room settings'
    );
    await expect.element(join ?? null).toBeEnabled();
    await expect.element(settings ?? null).toBeInTheDocument();

    settings!.click();
    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/manage/rooms/joinable-channel');
  });

  it('shows a disabled join action for a visible restricted room', async () => {
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/restricted-channel"]') as HTMLAnchorElement;

    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Join Room'));

    const join = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Join Room'
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

  it('does not offer leave for direct-message rooms', async () => {
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/dm-with-participants"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Mark as read'));

    const leave = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Leave room'
    );
    expect(leave).toBeUndefined();
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
    await vi.waitFor(() => expect(document.body.textContent).toContain('Room settings'));

    const settings = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Room settings'
    );
    settings!.click();

    expect(mocks.goto).toHaveBeenCalledWith('/chat/-/manage/rooms/channel-1');
  });

  it('archives a managed room through the lazily loaded sidebar confirmation', async () => {
    const { container } = render(RoomList);
    const row = q(container, '[href="/chat/-/channel-1"]') as HTMLAnchorElement;
    row.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(document.body.textContent).toContain('Archive Room'));

    const archiveMenuItem = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'Archive Room'
    );
    archiveMenuItem!.click();
    await vi.waitFor(() => {
      expect(document.body.textContent).toContain('Are you sure you want to archive #general?');
    });
    const archiveButtons = Array.from(document.querySelectorAll('button')).filter(
      (button) => button.textContent?.trim() === 'Archive Room'
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
      (button) => button.textContent?.trim() === 'Room settings'
    );
    expect(settings).toBeUndefined();
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
    expect(dmRow?.querySelector('[data-testid="bot-badge"]')).not.toBeNull();
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

  it('lets faded joinable non-member channel rows navigate to the room route', async () => {
    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/joinable-channel"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    expect(row.className).toContain('opacity-60');

    expect(row.getAttribute('href')).toBe('/chat/-/joinable-channel');
    expect(row.getAttribute('aria-disabled')).toBeNull();
    expect(mocks.pushState).not.toHaveBeenCalled();
  });

  it('lets faded non-joinable channel rows navigate to the inline access screen', async () => {
    const { container } = render(RoomList);

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

  it('renders server-local sidebar links as same-tab anchors resolved against the active server', async () => {
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
    expect(link.getAttribute('target')).toBeNull();
    expect(link.getAttribute('rel')).toBeNull();
  });

  it('adds HTTPS when a new sidebar link uses a hostname without a scheme', async () => {
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
    const newLink = Array.from(document.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === 'New Link'
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
      (button) => button.type === 'submit' && button.textContent?.trim() === 'Create Link'
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
  });

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
      (button) => button.textContent?.trim() === 'Settings'
    );
    await expect.element(settings ?? null).toBeInTheDocument();
    await expect
      .element(
        Array.from(document.querySelectorAll('button')).find(
          (button) => button.textContent?.trim() === 'Delete Group'
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
      (button) => button.textContent?.trim() === 'Delete Group'
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
      (button) => button.textContent?.trim() === 'Delete Group'
    );
    await expect.element(deleteGroup ?? null).toBeDisabled();
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

    await expect.element(q(container, '[data-testid="create-room-button"]')).toBeInTheDocument();
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
      container.querySelectorAll<HTMLButtonElement>('button[aria-expanded]')
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
    await expect.element(q(container, '[data-testid="create-room-button"]')).toBeInTheDocument();
  });

  it('keeps context-menu attachments but hides drag handles for a server without relative moves', () => {
    mocks.store.serverInfo.supportsFeature.mockReturnValue(false);
    mocks.store.navigation.roomGroups = [
      {
        id: 'projects',
        name: 'Projects',
        viewerCanManageGroup: true,
        viewerCanCreateRoom: true,
        roomIds: ['channel-1']
      }
    ];

    const { container } = render(RoomList, { props: { canReorderGroups: true } });

    expect(container.querySelector('[data-testid="room-group-drag-handle"]')).toBeNull();
    expect(container.querySelector('[data-testid="room-drag-handle"]')).toBeNull();
    expect(container.querySelector('[data-testid="room-group-actions-button"]')).toBeNull();
    expect(container.querySelector('[data-testid="room-actions-button"]')).toBeNull();
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

  it('renders active-server host sidebar links as same-tab anchors', async () => {
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
    mocks.store.notifications.getCleanPath.mockReturnValue('/chat/-/channel-1/thread-1');
    mocks.store.notifications.markRead.mockResolvedValue(true);

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
        'event-1',
        'mention-1'
      );
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'channel-1');
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
      );
      expect(mocks.store.notifications.markRead).not.toHaveBeenCalled();
      expect(mocks.store.notifications.getCleanPath).toHaveBeenCalledWith(
        'origin',
        roomNotification
      );
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/channel-1/thread-1');
    });
  });

  it('uses a neutral room badge when only ambient notifications are unread', async () => {
    setRoomNotificationCount('channel-1', 2, 0);

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="room-notification-badge"]');
    await expect.element(badge).toHaveClass('bg-text');
    await expect.element(badge).not.toHaveClass('bg-attention');
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
    mocks.store.notifications.markRead.mockResolvedValue(true);

    const { container } = render(RoomList);

    const badge = q(container, '[data-testid="dm-notification-badge"]');
    await expect.element(badge).toBeInTheDocument();
    (badge?.closest('button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.store.notifications.resolveRoomNotification).toHaveBeenCalledWith(
        'dm-with-participants',
        { isDM: true }
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
        isDM: false
      });
      expect(mocks.goto).not.toHaveBeenCalled();
      expect(mocks.store.notifications.markRead).not.toHaveBeenCalled();
    });
  });
});
