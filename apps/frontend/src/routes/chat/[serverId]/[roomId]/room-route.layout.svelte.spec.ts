import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import { PresenceStatus, RoomType } from '$lib/render/types';
import type { RoomsListItem } from '$lib/state/server/rooms.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    page: {
      params: { serverId: '-', roomId: 'room-1' } as Record<string, string | undefined>,
      route: { id: '/chat/[serverId]/[roomId]' as string | null },
      state: {},
      url: new URL('https://chat.example.test/chat/-/room-1')
    },
    roomsStore: {
      rooms: [] as RoomsListItem[],
      roomGroups: [] as Array<{ id: string; name: string; roomIds: string[] }>,
      isInitialLoading: false
    },
    joinRoom: vi.fn(),
    loadJoinPreview: vi.fn(),
    refreshRooms: vi.fn(),
    toastSuccess: vi.fn(),
    toastError: vi.fn()
  }
}));

vi.mock('$app/state', () => ({
  page: mocks.page
}));

vi.mock('$app/navigation', () => ({
  goto: mocks.goto
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
      .replace('[threadId]', params?.threadId ?? '')
      .replace('[messageId]', params?.messageId ?? '')
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: unknown, fallback: unknown) => fallback
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      rooms: {
        get rooms() {
          return mocks.roomsStore.rooms;
        },
        get isInitialLoading() {
          return mocks.roomsStore.isInitialLoading;
        },
        get roomGroups() {
          return mocks.roomsStore.roomGroups;
        },
        refresh: mocks.refreshRooms
      },
      roomDirectory: {
        joinRoom: mocks.joinRoom,
        loadJoinPreview: mocks.loadJoinPreview
      }
    })
  }
}));

vi.mock('$lib/ui/toast', () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError
  }
}));

vi.mock('./Room.svelte', async () => {
  const { default: RoomMock } = await import('./RoomRouteLayoutRoomMock.svelte');
  return { default: RoomMock };
});

import Layout from './+layout.svelte';

function room(overrides: Partial<RoomsListItem> = {}): RoomsListItem {
  return {
    id: 'room-1',
    name: 'development',
    type: RoomType.Channel,
    isUniversal: false,
    viewerIsMember: true,
    viewerCanJoinRoom: true,
    viewerNotificationCount: 0,
    members: [],
    ...overrides
  };
}

function renderLayout() {
  return render(Layout, {
    props: {
      data: {
        user: null,
        serverInfo: null,
        serverInfoLoaded: true,
        serverSegment: '-',
        roomId: 'room-1'
      },
      children: testSnippet('<div data-testid="message-resolver"></div>')
    }
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.page.params = { serverId: '-', roomId: 'room-1' };
  mocks.page.route.id = '/chat/[serverId]/[roomId]';
  mocks.page.state = {};
  mocks.page.url = new URL('https://chat.example.test/chat/-/room-1');
  mocks.roomsStore.rooms = [room()];
  mocks.roomsStore.roomGroups = [];
  mocks.roomsStore.isInitialLoading = false;
  mocks.loadJoinPreview.mockResolvedValue(null);
  mocks.joinRoom.mockResolvedValue({ ok: true, room: { id: 'room-1', name: 'development' } });
  mocks.refreshRooms.mockResolvedValue(undefined);
});

describe('room route layout access handling', () => {
  it('renders the room without redirecting when the viewer is already a member', async () => {
    const { container } = renderLayout();

    await tick();

    expect(mocks.goto).not.toHaveBeenCalled();
    expect(q(container, '[data-testid="room-layout-room"]')?.dataset.roomId).toBe('room-1');
  });

  it('renders an inline join screen for a room deep link when the viewer is not a member', async () => {
    mocks.roomsStore.rooms = [room({ viewerIsMember: false })];

    const { container } = renderLayout();

    await expect.element(q(container, 'h1')).toHaveTextContent('#development');
    await expect
      .element(q(container, 'section'))
      .toHaveTextContent('Join this room to read and participate.');
    expect(q(container, '[data-testid="room-layout-room"]')).toBeNull();
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('renders the inline join screen instead of the message resolver for nonmember message links', async () => {
    mocks.page.url = new URL('https://chat.example.test/chat/-/room-1/m/Eabc123DEF456gh');
    mocks.roomsStore.rooms = [room({ viewerIsMember: false })];

    const { container } = renderLayout();

    await expect.element(q(container, 'h1')).toHaveTextContent('#development');
    await expect
      .element(q(container, 'section'))
      .toHaveTextContent('Join this room to read and participate.');
    expect(q(container, '[data-testid="message-resolver"]')).toBeNull();
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('shows room context and a limited member preview before joining', async () => {
    mocks.roomsStore.rooms = [
      room({ viewerIsMember: false, description: 'Coordination for the launch.' })
    ];
    mocks.roomsStore.roomGroups = [{ id: 'group-1', name: 'Projects', roomIds: ['room-1'] }];
    mocks.loadJoinPreview.mockResolvedValue({
      memberCount: 6,
      sampleMembers: [
        {
          id: 'user-1',
          login: 'alice',
          displayName: 'Alice',
          deleted: false,
          avatarUrl: null,
          presenceStatus: PresenceStatus.Offline,
          customStatus: null
        },
        {
          id: 'user-2',
          login: 'bob',
          displayName: 'Bob',
          deleted: false,
          avatarUrl: null,
          presenceStatus: PresenceStatus.Online,
          customStatus: null
        }
      ]
    });

    const { container } = renderLayout();

    await expect
      .element(q(container, '[data-testid="room-join-preview"]'))
      .toHaveTextContent('Coordination for the launch.');
    await expect
      .element(q(container, '[data-testid="room-join-preview"]'))
      .toHaveTextContent('In Projects');
    await vi.waitFor(() => {
      expect(q(container, '[aria-label="Room members"]')?.textContent).toContain('6 members');
      expect(q(container, '[aria-label="Room members"]')?.textContent).not.toContain('Alice');
      expect(q(container, '[aria-label="Room members"]')?.textContent).not.toContain('Bob');
      expect(
        q(container, '[aria-label="Room members"]')?.querySelectorAll('[role="img"]')
      ).toHaveLength(2);
    });
    expect(mocks.loadJoinPreview).toHaveBeenCalledWith('room-1');
  });

  it('renders an empty room preview without hiding the join action', async () => {
    mocks.roomsStore.rooms = [room({ viewerIsMember: false })];
    mocks.loadJoinPreview.mockResolvedValue({ memberCount: 0, sampleMembers: [] });

    const { container } = renderLayout();

    await vi.waitFor(() => {
      expect(q(container, '[aria-label="Room members"]')?.textContent).toContain('0 members');
    });
    await expect.element(q(container, 'button')).toHaveTextContent('Join Room');
  });

  it('removes the preview skeleton after a best-effort preview miss', async () => {
    let resolvePreview!: (value: null) => void;
    mocks.roomsStore.rooms = [room({ viewerIsMember: false })];
    mocks.loadJoinPreview.mockReturnValue(
      new Promise<null>((resolve) => {
        resolvePreview = resolve;
      })
    );

    const { container } = renderLayout();

    expect(q(container, '[aria-label="Room members"] .skeleton')).not.toBeNull();
    resolvePreview(null);
    await vi.waitFor(() => expect(q(container, '[aria-label="Room members"]')).toBeNull());
    await expect.element(q(container, 'button')).toHaveTextContent('Join Room');
  });

  it('joins a nonmember room inline and refreshes room membership without changing URLs', async () => {
    mocks.roomsStore.rooms = [room({ viewerIsMember: false })];

    const { container } = renderLayout();
    (q(container, 'button') as HTMLButtonElement).click();

    await vi.waitFor(() => {
      expect(mocks.joinRoom).toHaveBeenCalledWith('room-1');
      expect(mocks.toastSuccess).toHaveBeenCalledWith('Joined #development');
      expect(mocks.refreshRooms).toHaveBeenCalledOnce();
    });
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('renders inline access denial for restricted nonmember rooms', async () => {
    mocks.roomsStore.rooms = [
      room({
        viewerIsMember: false,
        viewerCanJoinRoom: false,
        description: 'Restricted description'
      })
    ];

    const { container } = renderLayout();

    await expect
      .element(q(container, 'section'))
      .toHaveTextContent('You do not have permission to join this room.');
    await expect.element(q(container, 'a[href="/chat/-"]')).toHaveTextContent('Return to Server');
    expect(q(container, 'button')).toBeNull();
    expect(container.textContent).not.toContain('Restricted description');
    expect(mocks.loadJoinPreview).not.toHaveBeenCalled();
    expect(mocks.joinRoom).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });

  it('renders a nested thread message route with its thread and message targets', async () => {
    mocks.page.params = {
      serverId: '-',
      roomId: 'room-1',
      threadId: 'Ethread123ABC456',
      messageId: 'Ereply123ABC4567'
    };
    mocks.page.route.id = '/chat/[serverId]/[roomId]/[threadId]/m/[messageId]';
    mocks.page.url = new URL(
      'https://chat.example.test/chat/-/room-1/Ethread123ABC456/m/Ereply123ABC4567'
    );

    const { container } = renderLayout();
    await tick();

    const renderedRoom = q(container, '[data-testid="room-layout-room"]') as HTMLElement;
    expect(renderedRoom?.dataset.threadId).toBe('Ethread123ABC456');
    expect(renderedRoom?.dataset.routeMessageId).toBe('Ereply123ABC4567');
    expect(q(container, '[data-testid="message-resolver"]')).toBeNull();
  });

  it('renders the target thread after joining has made the viewer a member', async () => {
    mocks.page.params = { serverId: '-', roomId: 'room-1', threadId: 'Ethread123ABC456' };
    mocks.page.url = new URL('https://chat.example.test/chat/-/room-1/Ethread123ABC456');
    mocks.roomsStore.rooms = [room()];

    const { container } = renderLayout();

    await tick();

    const renderedRoom = q(container, '[data-testid="room-layout-room"]') as HTMLElement;
    expect(renderedRoom?.dataset.roomId).toBe('room-1');
    expect(renderedRoom?.dataset.threadId).toBe('Ethread123ABC456');
  });
});
