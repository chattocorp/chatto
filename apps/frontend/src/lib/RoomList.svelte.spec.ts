import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { RoomType } from '$lib/render/types';
import { NotificationItemKind } from '$lib/api-client/notifications';
import { serverStorageKey } from '$lib/storage/serverStorage';
import {
  consumePendingRoomSidebarPanel,
  roomSidebarPanelStorageSuffix
} from '$lib/storage/roomSidebarPanel';
import type { RoomsListGroup } from '$lib/state/server/rooms.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    activeCallRoomIds: new Set<string>(),
    projectedCallParticipants: new Map<string, unknown[]>(),
    unreadRoomIds: new Set<string>(),
    markNavigationRoomAsRead: vi.fn().mockResolvedValue(true),
    pushState: vi.fn(),
    goto: vi.fn(),
    appUi: {
      disableRoomCallWideFor: vi.fn()
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
        dismiss: vi.fn(),
        getCleanPath: vi.fn().mockReturnValue('/chat/-/room')
      },
      notificationLevels: {
        isRoomMuted: vi.fn().mockReturnValue(false)
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
      rooms: {
        rooms: [],
        roomGroups: null as RoomsListGroup[] | null,
        isInitialLoading: false,
        currentUserId: 'me',
        bumpRoom: vi.fn(),
        clearUnreadNotifications: vi.fn(),
        decrementUnreadNotification: vi.fn(),
        incrementUnreadNotification: vi.fn(),
        refreshNotificationCounts: vi.fn().mockResolvedValue(undefined)
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

vi.mock('$lib/state/appUi.svelte', () => ({
  getAppUiState: () => mocks.appUi
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: string) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/navigation/readActions', () => ({
  markNavigationRoomAsRead: mocks.markNavigationRoomAsRead
}));

import RoomList from './RoomList.svelte';

function notification(id: string, roomId: string, isDM = false) {
  if (isDM) {
    return {
      kind: NotificationItemKind.DirectMessage,
      id,
      createdAt: '2026-06-18T10:00:00Z',
      actor: null,
      summary: 'new direct message',
      room: { id: roomId }
    };
  }

  return {
    kind: NotificationItemKind.Mention,
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
  if (hasUnread) mocks.unreadRoomIds.add(roomId);
  else mocks.unreadRoomIds.delete(roomId);
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  mocks.activeCallRoomIds = new Set();
  mocks.projectedCallParticipants = new Map();
  mocks.unreadRoomIds = new Set();
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
  mocks.store.roomDirectory.joinRoom.mockResolvedValue({ ok: true });
  mocks.markNavigationRoomAsRead.mockResolvedValue(true);
});

describe('RoomList', () => {
  it('hides only DMs explicitly projected without message history', async () => {
    const empty = mocks.store.rooms.rooms.find(
      (room: { id: string }) => room.id === 'dm-with-participants'
    ) as unknown as { hasMessageHistory?: boolean };
    empty.hasMessageHistory = false;

    const { container } = render(RoomList);

    expect(container.querySelector('[href="/chat/-/dm-with-participants"]')).toBeNull();
    await expect.element(q(container, '[href="/chat/-/dm-phone-only"]')).toBeInTheDocument();
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
    await expect.element(q(document.body, '[role="separator"]')).toBeInTheDocument();

    markRead!.click();

    expect(mocks.markNavigationRoomAsRead).toHaveBeenCalledWith('origin', 'channel-1');
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
      modal: { type: 'leaveRoom', roomId: 'channel-1', roomName: 'general' }
    });
  });

  it('renders active-call DM rows with the pulse icon and participant avatars', async () => {
    mocks.activeCallRoomIds.add('dm-with-participants');
    mocks.projectedCallParticipants.set('dm-with-participants', [
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
    expect(icon?.classList.contains('text-action')).toBe(true);
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

  it('lets faded joinable non-member channel rows navigate to the room route', async () => {
    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/joinable-channel"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    expect(row.className).toContain('opacity-60');

    const event = new MouseEvent('click', { bubbles: true, cancelable: true });
    const wasNotCanceled = row.dispatchEvent(event);

    expect(wasNotCanceled).toBe(true);
    expect(mocks.pushState).not.toHaveBeenCalled();
  });

  it('lets faded non-joinable channel rows navigate to the inline access screen', async () => {
    const { container } = render(RoomList);

    const row = q(container, '[href="/chat/-/restricted-channel"]') as HTMLAnchorElement;
    await expect.element(row).toBeInTheDocument();
    expect(row.className).toContain('opacity-60');
    const icon = row.querySelector('.sidebar-icon');
    expect(icon?.classList.contains('uil--lock')).toBe(true);
    expect(row.querySelectorAll('.uil--lock')).toHaveLength(1);

    const event = new MouseEvent('click', { bubbles: true, cancelable: true });
    const wasNotCanceled = row.dispatchEvent(event);

    expect(wasNotCanceled).toBe(true);
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
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'channel-1');
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
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
      expect(mocks.appUi.disableRoomCallWideFor).toHaveBeenCalledWith(
        'origin',
        'dm-with-participants'
      );
      expect(mocks.appUi.disableRoomCallWideFor.mock.invocationCallOrder[0]).toBeLessThan(
        mocks.goto.mock.invocationCallOrder[0]
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
