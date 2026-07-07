import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import { RoomType } from '$lib/render/types';
import type { RoomsListItem } from '$lib/state/server/rooms.svelte';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    goto: vi.fn(),
    page: {
      params: { serverId: '-', roomId: 'room-1' } as Record<string, string | undefined>,
      state: {},
      url: new URL('https://chat.example.test/chat/-/room-1')
    },
    roomsStore: {
      rooms: [] as RoomsListItem[],
      isInitialLoading: false
    }
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

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getStore: () => ({
      rooms: mocks.roomsStore
    })
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
    hasUnread: false,
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
  mocks.page.state = {};
  mocks.page.url = new URL('https://chat.example.test/chat/-/room-1');
  mocks.roomsStore.rooms = [room()];
  mocks.roomsStore.isInitialLoading = false;
});

describe('room route layout access handling', () => {
  it('renders the room without redirecting when the viewer is already a member', async () => {
    const { container } = renderLayout();

    await tick();

    expect(mocks.goto).not.toHaveBeenCalled();
    expect(q(container, '[data-testid="room-layout-room"]')?.dataset.roomId).toBe('room-1');
  });

  it('opens the join modal for a room deep link when the viewer is not a member', async () => {
    mocks.roomsStore.rooms = [room({ viewerIsMember: false })];

    renderLayout();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-', {
        replaceState: true,
        state: {
          modal: {
            type: 'joinRoom',
            roomId: 'room-1',
            roomName: 'development',
            viewerCanJoinRoom: true,
            afterJoinPath: '/chat/-/room-1',
            closePath: '/chat/-'
          }
        }
      });
    });
  });

  it('opens the join modal with the original message link target for message deep links', async () => {
    mocks.page.url = new URL('https://chat.example.test/chat/-/room-1/m/Eabc123DEF456gh');
    mocks.roomsStore.rooms = [room({ viewerIsMember: false })];

    renderLayout();

    await vi.waitFor(() => {
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-', {
        replaceState: true,
        state: {
          modal: {
            type: 'joinRoom',
            roomId: 'room-1',
            roomName: 'development',
            viewerCanJoinRoom: true,
            afterJoinPath: '/chat/-/room-1/m/Eabc123DEF456gh',
            closePath: '/chat/-'
          }
        }
      });
    });
  });
});
