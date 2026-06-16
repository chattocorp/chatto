import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { Client } from '@urql/svelte';
import { NotificationLevel, RoomType } from '$lib/gql/graphql';
import { NotificationLevelStore } from './notificationLevel.svelte';
import { RoomUnreadStore } from './roomUnread.svelte';
import { isRoomStateRefreshEvent, RoomsStore } from './rooms.svelte';

type QueryRoom = {
  id: string;
  name: string;
  type: RoomType;
  hasUnread: boolean;
  archived: boolean;
  viewerNotificationPreference: {
    level: NotificationLevel;
    effectiveLevel: NotificationLevel;
  } | null;
  members: {
    users: Array<{
      id: string;
      login: string;
      displayName: string;
      avatarUrl: string | null;
      presenceStatus: string;
    }>;
  };
};

type QueryResponse = {
  viewer: {
    user: {
      id: string;
      rooms: QueryRoom[];
    };
  };
  server: {
    roomGroups: Array<{
      id: string;
      name: string;
      rooms: Array<{ id: string }>;
    }>;
  };
};

type NotificationCountsResponse = {
  viewer: {
    user: {
      rooms: Array<{
        id: string;
        viewerNotifications: {
          totalCount: number;
        };
      }>;
    };
  };
};

function makeRoom(id: string, overrides: Partial<QueryRoom> = {}): QueryRoom {
  return {
    id,
    name: overrides.name ?? id,
    type: overrides.type ?? RoomType.Channel,
    hasUnread: overrides.hasUnread ?? false,
    archived: overrides.archived ?? false,
    viewerNotificationPreference:
      overrides.viewerNotificationPreference === undefined
        ? {
            level: NotificationLevel.Default,
            effectiveLevel: NotificationLevel.Normal
          }
        : overrides.viewerNotificationPreference,
    members: overrides.members ?? {
      users: [
        {
          id: 'U1',
          login: 'alice',
          displayName: 'Alice',
          avatarUrl: null,
          presenceStatus: 'ONLINE'
        }
      ]
    }
  };
}

function makeResponse(
  rooms: QueryRoom[],
  groups: QueryResponse['server']['roomGroups'] = []
): QueryResponse {
  return {
    viewer: {
      user: {
        id: 'U1',
        rooms
      }
    },
    server: {
      roomGroups: groups
    }
  };
}

function makeStore(client: Client) {
  return new RoomsStore(client, new NotificationLevelStore(), new RoomUnreadStore());
}

function operationName(document: unknown): string | undefined {
  return (
    document as {
      definitions?: Array<{ name?: { value?: string } }>;
    }
  ).definitions?.[0]?.name?.value;
}

function makeCountsResponse(counts: Record<string, number>): NotificationCountsResponse {
  return {
    viewer: {
      user: {
        rooms: Object.entries(counts).map(([id, totalCount]) => ({
          id,
          viewerNotifications: { totalCount }
        }))
      }
    }
  };
}

function makeClient(responses: Array<QueryResponse | NotificationCountsResponse | null>) {
  const queue = [...responses];
  const queryMock = vi.fn(() => ({
    toPromise: () => Promise.resolve({ data: queue.shift() ?? null, error: null })
  }));
  const client = { query: queryMock } as unknown as Client;
  return { client, queryMock };
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('RoomsStore - refresh', () => {
  it('discards out-of-order responses', async () => {
    let resolveFirst!: (value: { data: QueryResponse; error: null }) => void;
    let resolveSecond!: (value: { data: QueryResponse; error: null }) => void;
    const queryMock = vi.fn().mockImplementation((document: unknown) => {
      if (operationName(document) === 'GetMyServerRoomNotificationCounts') {
        return {
          toPromise: () => Promise.resolve({ data: makeCountsResponse({ newer: 4 }), error: null })
        };
      }
      if (queryMock.mock.calls.length === 1) {
        return {
          toPromise: () => new Promise((resolve) => (resolveFirst = resolve))
        };
      }
      return {
        toPromise: () => new Promise((resolve) => (resolveSecond = resolve))
      };
    });
    const store = makeStore({ query: queryMock } as unknown as Client);

    void store.refresh();
    void store.refresh();

    resolveSecond({
      data: makeResponse(
        [makeRoom('newer')],
        [{ id: 'g1', name: 'Lobby', rooms: [{ id: 'newer' }] }]
      ),
      error: null
    });
    await settle();

    expect(store.rooms.map((room) => room.id)).toEqual(['newer']);
    expect(store.roomGroups).toEqual([{ id: 'g1', name: 'Lobby', roomIds: ['newer'] }]);
    await vi.waitFor(() => {
      expect(store.rooms.find((room) => room.id === 'newer')?.viewerNotificationCount).toBe(4);
    });

    resolveFirst({
      data: makeResponse(
        [makeRoom('older')],
        [{ id: 'g1', name: 'Lobby', rooms: [{ id: 'older' }] }]
      ),
      error: null
    });
    await settle();

    expect(store.rooms.map((room) => room.id)).toEqual(['newer']);
    expect(store.roomGroups).toEqual([{ id: 'g1', name: 'Lobby', roomIds: ['newer'] }]);
  });

  it('patches notification counts from the optional compatibility query', async () => {
    let resolveCounts!: (value: { data: NotificationCountsResponse; error: null }) => void;
    const queryMock = vi.fn((document: unknown) => {
      if (operationName(document) === 'GetMyServerRoomNotificationCounts') {
        return {
          toPromise: () => new Promise((resolve) => (resolveCounts = resolve))
        };
      }
      return {
        toPromise: () => Promise.resolve({ data: makeResponse([makeRoom('general')]), error: null })
      };
    });
    const store = makeStore({ query: queryMock } as unknown as Client);

    await store.refresh();

    expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 0 }]);
    resolveCounts({ data: makeCountsResponse({ general: 7 }), error: null });

    await vi.waitFor(() => {
      expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 7 }]);
    });
  });

  it('keeps rooms visible when the optional notification count field is unsupported', async () => {
    const queryMock = vi.fn((document: unknown) => {
      if (operationName(document) === 'GetMyServerRoomNotificationCounts') {
        return {
          toPromise: () =>
            Promise.resolve({
              data: null,
              error: {
                message: 'Cannot query field "viewerNotifications" on type "Room".'
              }
            })
        };
      }
      return {
        toPromise: () => Promise.resolve({ data: makeResponse([makeRoom('general')]), error: null })
      };
    });
    const store = makeStore({ query: queryMock } as unknown as Client);

    await store.refresh();
    await settle();

    expect(store.rooms).toMatchObject([{ id: 'general', viewerNotificationCount: 0 }]);
  });
});

describe('RoomsStore - ingestServerEvent', () => {
  function makeEvent(typename: string) {
    return { event: { __typename: typename } };
  }

  it('uses one shared predicate for room state refresh events', () => {
    expect(isRoomStateRefreshEvent('RoomCreatedEvent')).toBe(true);
    expect(isRoomStateRefreshEvent('RoomGroupsUpdatedEvent')).toBe(true);
    expect(isRoomStateRefreshEvent('ReactionAddedEvent')).toBe(false);
  });

  it('refreshes on RoomCreatedEvent', () => {
    const { client } = makeClient([]);
    const store = makeStore(client);
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent('RoomCreatedEvent'));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('refreshes on RoomGroupsUpdatedEvent', () => {
    const { client } = makeClient([]);
    const store = makeStore(client);
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent('RoomGroupsUpdatedEvent'));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('refreshes on UserJoinedRoomEvent', () => {
    const { client } = makeClient([]);
    const store = makeStore(client);
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent('UserJoinedRoomEvent'));

    expect(store.refresh).toHaveBeenCalledOnce();
  });

  it('does not refresh on irrelevant event types', () => {
    const { client } = makeClient([]);
    const store = makeStore(client);
    store.refresh = vi.fn().mockResolvedValue(undefined);

    store.ingestServerEvent(makeEvent('ReactionAddedEvent'));
    store.ingestServerEvent(makeEvent('HeartbeatEvent'));

    expect(store.refresh).not.toHaveBeenCalled();
  });
});
