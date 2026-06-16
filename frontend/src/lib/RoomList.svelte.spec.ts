import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q } from '$lib/test-utils';
import { RoomType } from '$lib/gql/graphql';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    activeCallRoomIds: new Set<string>(),
    callParticipants: new Map<string, unknown[]>(),
    store: {
      currentUser: { user: { id: 'me' } },
      notifications: {
        hasDMRoomNotification: vi.fn().mockReturnValue(false),
        hasRoomNotification: vi.fn().mockReturnValue(false),
        getDMRoomNotification: vi.fn().mockReturnValue(null),
        getRoomNotification: vi.fn().mockReturnValue(null),
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
        roomGroups: null,
        isInitialLoading: false,
        currentUserId: 'me',
        markRead: vi.fn(),
        bumpRoom: vi.fn(),
        setUnread: vi.fn()
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
  goto: vi.fn()
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params?: Record<string, string>) =>
    path
      .replace('[serverId]', params?.serverId ?? '')
      .replace('[roomId]', params?.roomId ?? '')
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
      hasUnread: false,
      members: []
    },
    {
      id: 'dm-with-participants',
      name: '',
      type: RoomType.Dm,
      hasUnread: false,
      members: [user('me', 'me', 'Me'), user('teal', 'teal', 'Teal')]
    },
    {
      id: 'dm-phone-only',
      name: '',
      type: RoomType.Dm,
      hasUnread: false,
      members: [user('me', 'me', 'Me'), user('river', 'river', 'River')]
    }
  ] as never;
}

beforeEach(() => {
  localStorage.clear();
  mocks.activeCallRoomIds = new Set();
  mocks.callParticipants = new Map();
  mocks.store.rooms.roomGroups = null;
  mocks.store.rooms.isInitialLoading = false;
  mocks.store.rooms.currentUserId = 'me';
  setRooms();
  vi.clearAllMocks();
});

describe('RoomList', () => {
  it('renders active-call badges for DM rows', async () => {
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
    const badge = dmRow?.querySelector('[data-testid="room-call-badge"]');
    expect(badge).not.toBeNull();
    expect(badge?.querySelector('.uil--phone')).not.toBeNull();
    expect(badge?.textContent).toContain('T');
  });

  it('renders a phone-only active-call badge when participants are not loaded', async () => {
    mocks.activeCallRoomIds.add('dm-phone-only');

    const { container } = render(RoomList);

    await expect.element(q(container, '[href="/chat/-/dm-phone-only"]')).toBeInTheDocument();
    const dmRow = q(container, '[href="/chat/-/dm-phone-only"]');
    const badge = dmRow?.querySelector('[data-testid="room-call-badge"]');
    expect(badge).not.toBeNull();
    expect(badge?.querySelector('.uil--phone')).not.toBeNull();
    expect(badge?.querySelector('.inline-flex')).toBeNull();
  });

  it('keeps channel active-call badges working', async () => {
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
    expect(channelRow?.querySelector('[data-testid="room-call-badge"]')).not.toBeNull();
    expect(channelRow?.querySelector('.uil--phone')).not.toBeNull();
  });
});
