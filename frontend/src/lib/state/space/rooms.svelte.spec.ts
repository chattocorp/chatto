import { describe, it, expect, vi } from 'vitest';
import { flushSync, untrack } from 'svelte';
import type { Client } from '@urql/svelte';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import { SpaceRoomsStore } from './rooms.svelte';

const SPACE_ID = 's_main';

type RawRoom = {
  id: string;
  name: string;
  hasUnread: boolean;
  hasMention: boolean;
  archived: boolean;
  viewerNotificationPreference: { level: string; effectiveLevel: string } | null;
};

function makeRawRoom(id: string, overrides: Partial<RawRoom> = {}): RawRoom {
  return {
    id,
    name: overrides.name ?? id,
    hasUnread: overrides.hasUnread ?? false,
    hasMention: overrides.hasMention ?? false,
    archived: overrides.archived ?? false,
    viewerNotificationPreference: overrides.viewerNotificationPreference ?? null
  };
}

type QueryResponse = {
  me: { rooms: RawRoom[] } | null;
  space: {
    roomLayout: {
      sections: { id: string; name: string; rooms: { id: string }[] }[];
      unsectionedRoomIds: string[];
    } | null;
  } | null;
};

function makeMockClient(response: QueryResponse | null = null) {
  const queryMock = vi.fn(() => ({
    toPromise: () => Promise.resolve({ data: response, error: null })
  }));
  const client = { query: queryMock } as unknown as Client;
  return { client, queryMock };
}

function makeMockNotificationLevels() {
  return {
    setRoomPreference: vi.fn(),
    isRoomMuted: vi.fn(() => false)
  } as unknown as ConstructorParameters<typeof SpaceRoomsStore>[2] & {
    setRoomPreference: ReturnType<typeof vi.fn>;
  };
}

function makeMockRoomUnread() {
  return {
    initSpaceRooms: vi.fn(),
    setRoomUnread: vi.fn(),
    roomIsUnread: vi.fn(() => false)
  } as unknown as ConstructorParameters<typeof SpaceRoomsStore>[3] & {
    initSpaceRooms: ReturnType<typeof vi.fn>;
  };
}

function makeStore(response: QueryResponse | null = null) {
  const { client, queryMock } = makeMockClient(response);
  const notificationLevels = makeMockNotificationLevels();
  const roomUnread = makeMockRoomUnread();
  const store = new SpaceRoomsStore(client, SPACE_ID, notificationLevels, roomUnread);
  return { store, client, queryMock, notificationLevels, roomUnread };
}

async function settle() {
  // Wait two microtask ticks for the in-flight query → setState chain to drain.
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

describe('SpaceRoomsStore — initial load', () => {
  it('populates rooms, filters archived, and clears loading flag', async () => {
    const { store } = makeStore({
      me: {
        rooms: [
          makeRawRoom('r1', { name: 'general' }),
          makeRawRoom('r2', { name: 'archived', archived: true }),
          makeRawRoom('r3', { name: 'random', hasUnread: true })
        ]
      },
      space: null
    });

    expect(store.isInitialLoading).toBe(true);
    expect(store.rooms).toEqual([]);

    await settle();

    expect(store.isInitialLoading).toBe(false);
    expect(store.rooms.map((r) => r.id)).toEqual(['r1', 'r3']);
    expect(store.rooms[1]).toMatchObject({
      id: 'r3',
      name: 'random',
      hasUnread: true,
      hasMention: false
    });
  });

  it('maps room layout sections and unsectioned ids', async () => {
    const { store } = makeStore({
      me: { rooms: [makeRawRoom('r1'), makeRawRoom('r2'), makeRawRoom('r3')] },
      space: {
        roomLayout: {
          sections: [
            { id: 'sec1', name: 'Channels', rooms: [{ id: 'r1' }, { id: 'r2' }] }
          ],
          unsectionedRoomIds: ['r3']
        }
      }
    });

    await settle();

    expect(store.layoutSections).toEqual([
      { id: 'sec1', name: 'Channels', roomIds: ['r1', 'r2'] }
    ]);
    expect(store.unsectionedRoomIds).toEqual(['r3']);
  });

  it('leaves layout null when space has no roomLayout', async () => {
    const { store } = makeStore({
      me: { rooms: [makeRawRoom('r1')] },
      space: { roomLayout: null }
    });

    await settle();

    expect(store.layoutSections).toBeNull();
    expect(store.unsectionedRoomIds).toEqual([]);
  });

  it('forwards notification preferences and unread init to instance stores', async () => {
    const { store, notificationLevels, roomUnread } = makeStore({
      me: {
        rooms: [
          makeRawRoom('r1', {
            viewerNotificationPreference: { level: 'ALL', effectiveLevel: 'ALL' }
          }),
          makeRawRoom('r2', { archived: true }),
          makeRawRoom('r3')
        ]
      },
      space: null
    });

    await settle();

    expect(notificationLevels.setRoomPreference).toHaveBeenCalledWith(
      SPACE_ID,
      'r1',
      'ALL',
      'ALL'
    );
    // initSpaceRooms gets only the visible (non-archived) rooms
    expect(roomUnread.initSpaceRooms).toHaveBeenCalledWith(SPACE_ID, [
      expect.objectContaining({ id: 'r1' }),
      expect.objectContaining({ id: 'r3' })
    ]);
    // No-op getter to satisfy `requireAssertions` for the store reference.
    expect(store).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// Per-room flag mutations
// ---------------------------------------------------------------------------

describe('SpaceRoomsStore — flag mutations', () => {
  async function makeLoaded() {
    const fixture = makeStore({
      me: {
        rooms: [
          makeRawRoom('r1', { hasUnread: true, hasMention: true }),
          makeRawRoom('r2')
        ]
      },
      space: null
    });
    await settle();
    return fixture;
  }

  it('markRead clears both flags', async () => {
    const { store } = await makeLoaded();
    store.markRead('r1');
    expect(store.rooms[0]).toMatchObject({ hasUnread: false, hasMention: false });
  });

  it('setUnread/setMention/clearMention update the matching room', async () => {
    const { store } = await makeLoaded();
    store.setUnread('r2');
    store.setMention('r2');
    expect(store.rooms[1]).toMatchObject({ hasUnread: true, hasMention: true });
    store.clearMention('r2');
    expect(store.rooms[1]).toMatchObject({ hasMention: false });
  });

  it('mutators are no-ops for unknown roomId', async () => {
    const { store } = await makeLoaded();
    const before = store.rooms.map((r) => ({ ...r }));
    store.markRead('r_unknown');
    store.setMention('r_unknown');
    store.setUnread('r_unknown');
    store.clearMention('r_unknown');
    expect(store.rooms).toEqual(before);
  });

  it('mutators called from inside a $effect do not loop', async () => {
    // Regression: a previous version of patchRoom read `rooms` (via findIndex)
    // and then wrote to it. When `markRead` was called from within a $effect
    // (RoomList's "clear unread when entering a room" effect), the read+write
    // pair triggered Svelte's effect_update_depth_exceeded guard.
    const { store } = await makeLoaded();
    let activeRoomId = $state('r1');
    let runs = 0;

    const stop = $effect.root(() => {
      $effect(() => {
        runs++;
        store.markRead(activeRoomId);
      });
    });

    flushSync();
    // Trigger a re-run by changing the tracked state.
    activeRoomId = 'r2';
    flushSync();
    stop();

    // Two effect runs (initial + reactive update); the bug would manifest as
    // either an explosion of runs or a thrown effect_update_depth_exceeded.
    expect(runs).toBe(2);
    expect(untrack(() => store.rooms[0])).toMatchObject({ hasUnread: false, hasMention: false });
    expect(untrack(() => store.rooms[1])).toMatchObject({ hasUnread: false, hasMention: false });
  });
});

// ---------------------------------------------------------------------------
// Subscription event ingestion
// ---------------------------------------------------------------------------

function makeEvent(typename: string, fields: Record<string, unknown> = {}): RoomEventViewFragment {
  return {
    id: `evt_${typename}`,
    createdAt: '2024-01-01T00:00:00Z',
    actorId: 'u_actor',
    actor: { id: 'u_actor' },
    event: { __typename: typename, ...fields }
  } as unknown as RoomEventViewFragment;
}

describe('SpaceRoomsStore — ingestSpaceEvent', () => {
  it.each([
    'UserJoinedRoomEvent',
    'UserLeftRoomEvent',
    'RoomUpdatedEvent',
    'RoomArchivedEvent',
    'RoomUnarchivedEvent'
  ])('refreshes on %s', async (typename) => {
    const { store, queryMock } = makeStore({ me: { rooms: [] }, space: null });
    await settle();
    const callsBefore = queryMock.mock.calls.length;

    store.ingestSpaceEvent(makeEvent(typename));
    await settle();

    expect(queryMock.mock.calls.length).toBe(callsBefore + 1);
  });

  it.each(['MessagePostedEvent', 'ReactionAddedEvent', 'PresenceUpdatedEvent'])(
    'ignores %s',
    async (typename) => {
      const { store, queryMock } = makeStore({ me: { rooms: [] }, space: null });
      await settle();
      const callsBefore = queryMock.mock.calls.length;

      store.ingestSpaceEvent(makeEvent(typename));
      await settle();

      expect(queryMock.mock.calls.length).toBe(callsBefore);
    }
  );

  it('ignores events with no event payload', async () => {
    const { store, queryMock } = makeStore({ me: { rooms: [] }, space: null });
    await settle();
    const callsBefore = queryMock.mock.calls.length;

    store.ingestSpaceEvent({ id: 'evt', event: null } as unknown as RoomEventViewFragment);
    await settle();

    expect(queryMock.mock.calls.length).toBe(callsBefore);
  });
});

// ---------------------------------------------------------------------------
// Concurrent refresh guard
// ---------------------------------------------------------------------------

describe('SpaceRoomsStore — concurrent refresh guard', () => {
  it('drops the older response when refreshes overlap', async () => {
    // Build a client whose first query resolves only after the second one,
    // simulating an in-flight initial fetch overtaken by an event-driven refresh.
    let resolveFirst: (v: unknown) => void = () => {};
    const firstPromise = new Promise<unknown>((r) => {
      resolveFirst = r;
    });
    const secondResponse = {
      data: { me: { rooms: [makeRawRoom('r_second')] }, space: null },
      error: null
    };

    let call = 0;
    const queryMock = vi.fn(() => ({
      toPromise: () => {
        call++;
        return call === 1 ? firstPromise : Promise.resolve(secondResponse);
      }
    }));
    const client = { query: queryMock } as unknown as Client;
    const store = new SpaceRoomsStore(
      client,
      SPACE_ID,
      makeMockNotificationLevels(),
      makeMockRoomUnread()
    );

    // Trigger a second refresh while the first is still pending.
    void store.refresh();

    // Now resolve the first (stale) response. The store must NOT apply it.
    resolveFirst({ data: { me: { rooms: [makeRawRoom('r_first')] }, space: null }, error: null });
    await settle();

    expect(store.rooms.map((r) => r.id)).toEqual(['r_second']);
  });
});
