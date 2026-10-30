import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RoomService } from '@chatto/api-types/api/v1/rooms_connect';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { fakeServer, mockService, receivedRequest } from '@chatto/client/testing/fakeServer';
import type { ConnectAPIConfig } from '@chatto/client/api/connect';
import { navigationVisits } from '$lib/navigation/mutationCompletion';

const mocks = vi.hoisted(() => ({
  store: {
    realtimeSync: {
      hasDisplayableView: true,
      isRecoveringSnapshot: false,
      waitForNextCaughtUp: vi.fn()
    },
    accountId: 'self' as string | null,
    projection: { rooms: new Map<string, RoomWithViewerState>() },
    ensureRoomAvailable: vi.fn(),
    loadRoomMembers: vi.fn()
  },
  getAPI: vi.fn(),
  goto: vi.fn(),
  committed: null as Promise<void> | null
}));
vi.mock('$app/state', () => ({
  navigating: {
    get complete() {
      return mocks.committed;
    }
  }
}));
vi.mock('$app/navigation', () => ({ goto: mocks.goto }));
vi.mock('$lib/state/server/serverUi', () => ({
  serverUi: () => ({ pendingHighlights: { set: vi.fn() } })
}));
vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: { tryGetStore: () => mocks.store },
  serverConnectionManager: { getClient: () => ({ getAPI: mocks.getAPI }) }
}));
vi.mock('$lib/navigation', () => ({
  segmentToServerId: () => 'origin',
  serverIdToSegment: () => '-'
}));
vi.mock('$app/paths', () => ({
  resolve: (_: string, params: { userId: string }) => `/chat/-/user/${params.userId}`
}));
import { load } from './+layout';
import { load as messageLoad } from './m/[messageId]/+page';

const rooms = mockService(RoomService);
const messages = mockService(MessageService);
const config = fakeServer((router) => {
  router.service(RoomService, rooms);
  router.service(MessageService, messages);
});

beforeEach(() => {
  vi.resetAllMocks();
  mocks.store.accountId = 'self';
  mocks.store.realtimeSync.hasDisplayableView = true;
  mocks.committed = null;
  mocks.store.projection.rooms.clear();
  mocks.getAPI.mockImplementation((factory: (config: ConnectAPIConfig) => unknown) =>
    factory(config)
  );
  rooms.startDM.mockReturnValue({ room: { id: 'dm' } });
});

async function open(userId = 'recipient') {
  // Reading child params would make thread changes restart the parent's request.
  const params = new Proxy(
    { serverId: '-', userId },
    {
      get(target, key) {
        if (key !== 'serverId' && key !== 'userId')
          throw new Error(`Unexpected dependency: ${String(key)}`);
        return target[key as keyof typeof target];
      }
    }
  );
  const result = await load({ params, parent: async () => ({}), depends: vi.fn() } as never);
  return result!.conversationRoomId;
}

describe('conversation route load', () => {
  it('waits for the initial projection before opening the conversation', async () => {
    let finish!: (ready: boolean) => void;
    mocks.store.realtimeSync.hasDisplayableView = false;
    mocks.store.realtimeSync.waitForNextCaughtUp.mockReturnValue(
      new Promise<boolean>((resolve) => {
        finish = resolve;
      })
    );
    const pending = open();
    await vi.waitFor(() =>
      expect(mocks.store.realtimeSync.waitForNextCaughtUp).toHaveBeenCalledOnce()
    );
    expect(rooms.startDM).not.toHaveBeenCalled();
    finish(true);
    await expect(pending).resolves.toBe('dm');
  });
  it('does not create a DM until navigation commits, or after cancellation', async () => {
    let cancel!: (error: Error) => void;
    mocks.committed = new Promise<void>((_resolve, reject) => {
      cancel = reject;
    });
    const pending = open();
    await Promise.resolve();
    expect(rooms.startDM).not.toHaveBeenCalled();
    cancel(new Error('navigation canceled'));
    await expect(pending).resolves.toBeNull();
    expect(rooms.startDM).not.toHaveBeenCalled();
  });
  it.each(['recipient', 'self'])('opens %s and awaits authoritative room data', async (userId) => {
    let finish!: () => void;
    mocks.store.ensureRoomAvailable.mockReturnValue(
      new Promise<void>((resolve) => {
        finish = resolve;
      })
    );
    let completed = false;
    const pending = Promise.resolve(open(userId)).then((result) => {
      completed = true;
      return result;
    });
    await vi.waitFor(() => expect(mocks.store.ensureRoomAvailable).toHaveBeenCalledWith('dm'));
    expect(receivedRequest(rooms.startDM)?.participantIds).toEqual(
      userId === 'self' ? [] : ['recipient']
    );
    expect(completed).toBe(false);
    finish();
    await expect(pending).resolves.toBe('dm');
  });

  it('reuses a projected conversation without issuing a creation command', async () => {
    mocks.store.projection.rooms.set(
      'known',
      new RoomWithViewerState({
        room: { id: 'known', kind: RoomKind.DM },
        memberUserIds: ['self', 'recipient'],
        viewerState: { isMember: true }
      })
    );
    await expect(open()).resolves.toBe('known');
    expect(rooms.startDM).not.toHaveBeenCalled();
  });

  it('does not create a conversation before authentication', async () => {
    mocks.store.accountId = null;
    await expect(open()).resolves.toBeNull();
    expect(rooms.startDM).not.toHaveBeenCalled();
  });

  it.each(['command', 'refresh', 'missing'])(
    'returns a retryable result after %s failure',
    async (stage) => {
      if (stage === 'command') rooms.startDM.mockRejectedValueOnce(new Error('offline'));
      if (stage === 'refresh')
        mocks.store.ensureRoomAvailable.mockRejectedValueOnce(new Error('offline'));
      if (stage === 'missing') rooms.startDM.mockReturnValueOnce({});
      await expect(open()).resolves.toBeNull();
      await expect(open()).resolves.toBe('dm');
    }
  );
});

describe('message destination load', () => {
  it('does not read an old conversation through a replacement session', async () => {
    let finish!: (roomId: string) => void;
    const conversationRoomId = new Promise<string>((resolve) => {
      finish = resolve;
    });
    await messageLoad({
      params: { serverId: '-', userId: 'recipient', messageId: 'event' },
      parent: async () => ({ conversationRoomId })
    } as never);
    mocks.store = { ...mocks.store };
    finish('old-room');
    await conversationRoomId;
    expect(messages.getMessage).not.toHaveBeenCalled();
  });
  it('does not redirect after another destination replaces the pending lookup', async () => {
    let finish!: () => void;
    messages.getMessage.mockImplementation(async () => {
      await new Promise<void>((resolve) => {
        finish = resolve;
      });
      return {};
    });
    await messageLoad({
      params: { serverId: '-', userId: 'recipient', messageId: 'event' },
      parent: async () => ({ conversationRoomId: Promise.resolve('dm') })
    } as never);
    await vi.waitFor(() => expect(messages.getMessage).toHaveBeenCalledOnce());
    navigationVisits.leave();
    finish();
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(mocks.goto).not.toHaveBeenCalled();
  });
  it.each([false, true])('redirects a missing or failed message lookup: %s', async (fails) => {
    if (fails) messages.getMessage.mockRejectedValue(new Error('offline'));
    else messages.getMessage.mockReturnValue({});
    await messageLoad({
      params: { serverId: '-', userId: 'recipient', messageId: 'event' },
      parent: async () => ({ conversationRoomId: Promise.resolve('dm') })
    } as never);
    await vi.waitFor(() =>
      expect(mocks.goto).toHaveBeenCalledWith('/chat/-/user/recipient', { replaceState: true })
    );
  });
});
