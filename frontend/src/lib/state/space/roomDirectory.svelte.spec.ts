import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { Client } from '@urql/svelte';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import { RoomDirectoryStore, type DirectoryRoom } from './roomDirectory.svelte';

const SPACE_ID = 's_main';

function makeRoom(id: string, overrides: Partial<DirectoryRoom> = {}): DirectoryRoom {
  return {
    id,
    name: overrides.name ?? id,
    description: overrides.description ?? null,
    archived: overrides.archived ?? false,
    viewerCanJoinRoom: overrides.viewerCanJoinRoom ?? true
  };
}

type QueryResponse = { server: { id: string; rooms: DirectoryRoom[] } | null };

function makeClient(opts: {
  query?: QueryResponse | null;
  joinError?: string;
  leaveError?: string;
}) {
  const queryMock = vi.fn(() => ({
    toPromise: () => Promise.resolve({ data: opts.query ?? null, error: null })
  }));
  const mutationMock = vi.fn((doc: unknown) => {
    // crude heuristic — gql.tada returns objects, distinguish by error opt
    const docStr = JSON.stringify(doc);
    const isJoin = docStr.includes('joinRoom');
    const isLeave = docStr.includes('leaveRoom');
    const errMsg = isJoin ? opts.joinError : isLeave ? opts.leaveError : undefined;
    return {
      toPromise: () =>
        Promise.resolve({
          data: errMsg ? null : { joinRoom: true, leaveRoom: true },
          error: errMsg ? { message: errMsg } : null
        })
    };
  });
  const client = { query: queryMock, mutation: mutationMock } as unknown as Client;
  return { client, queryMock, mutationMock };
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

describe('RoomDirectoryStore — initial load', () => {
  it('populates allRooms and clears isLoading', async () => {
    const { client } = makeClient({
      query: {
        server: { id: SPACE_ID, rooms: [makeRoom('r1'), makeRoom('r2', { archived: true })] }
      }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);

    expect(store.isLoading).toBe(true);
    await settle();

    // Both rooms (archived + non-archived) are stored — the directory
    // surfaces archived state to UI but the store keeps them. Filtering is
    // a presentation concern.
    expect(store.allRooms.map((r) => r.id)).toEqual(['r1', 'r2']);
    expect(store.isLoading).toBe(false);
  });

  it('keeps allRooms unchanged when the query returns no data', async () => {
    const { client } = makeClient({ query: null });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    expect(store.allRooms).toEqual([]);
    expect(store.isLoading).toBe(false);
  });
});

describe('RoomDirectoryStore — isJoined predicate', () => {
  it('returns true when the room is in the joined set', async () => {
    const { client } = makeClient({ query: null });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    expect(store.isJoined('r1', new Set(['r1']))).toBe(true);
    expect(store.isJoined('r2', new Set(['r1']))).toBe(false);
  });

  it('returns true for an optimistically-just-joined room even if not in the joined set yet', async () => {
    const { client } = makeClient({ query: null });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.justJoinedIds.add('r1');
    expect(store.isJoined('r1', new Set())).toBe(true);
  });

  it('returns false for an optimistically-just-left room even if still in the joined set', async () => {
    const { client } = makeClient({ query: null });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.justLeftIds.add('r1');
    expect(store.isJoined('r1', new Set(['r1']))).toBe(false);
  });

  it('justLeft takes precedence over justJoined when both are set', async () => {
    const { client } = makeClient({ query: null });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.justJoinedIds.add('r1');
    store.justLeftIds.add('r1');
    expect(store.isJoined('r1', new Set())).toBe(false);
  });
});

describe('RoomDirectoryStore — joinRoom', () => {
  it('marks joining during the request and just-joined on success', async () => {
    const { client } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [makeRoom('r1', { name: 'general' })] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    const promise = store.joinRoom('r1');
    expect(store.joiningIds.has('r1')).toBe(true);

    const result = await promise;
    expect(result.ok).toBe(true);
    if (result.ok) expect(result.room?.name).toBe('general');
    expect(store.joiningIds.has('r1')).toBe(false);
    expect(store.justJoinedIds.has('r1')).toBe(true);
  });

  it('returns an error result and does not set just-joined when the mutation fails', async () => {
    const { client } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [makeRoom('r1')] } },
      joinError: 'permission denied'
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    const result = await store.joinRoom('r1');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.message).toBe('permission denied');
    expect(store.joiningIds.has('r1')).toBe(false);
    expect(store.justJoinedIds.has('r1')).toBe(false);
  });

  it('clears a stale justLeft when the user re-joins', async () => {
    const { client } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [makeRoom('r1')] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.justLeftIds.add('r1');
    await store.joinRoom('r1');

    expect(store.justJoinedIds.has('r1')).toBe(true);
    expect(store.justLeftIds.has('r1')).toBe(false);
  });
});

describe('RoomDirectoryStore — leaveRoom', () => {
  it('marks leaving during the request and just-left on success, clearing justJoined', async () => {
    const { client } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [makeRoom('r1')] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.justJoinedIds.add('r1'); // simulate prior optimistic join
    const promise = store.leaveRoom('r1');
    expect(store.leavingIds.has('r1')).toBe(true);

    const result = await promise;
    expect(result.ok).toBe(true);
    expect(store.leavingIds.has('r1')).toBe(false);
    expect(store.justLeftIds.has('r1')).toBe(true);
    expect(store.justJoinedIds.has('r1')).toBe(false);
  });

  it('returns an error result on failure', async () => {
    const { client } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [makeRoom('r1')] } },
      leaveError: 'cannot leave'
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    const result = await store.leaveRoom('r1');
    expect(result.ok).toBe(false);
    expect(store.leavingIds.has('r1')).toBe(false);
    expect(store.justLeftIds.has('r1')).toBe(false);
  });
});

describe('RoomDirectoryStore — refresh clears optimistic state', () => {
  it('refresh clears just-* sets so the authoritative joined membership wins', async () => {
    const { client } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [makeRoom('r1')] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.justJoinedIds.add('r1');
    store.justLeftIds.add('r2');

    await store.refresh();
    await settle();

    expect(store.justJoinedIds.size).toBe(0);
    expect(store.justLeftIds.size).toBe(0);
  });
});

describe('RoomDirectoryStore — ingestSpaceEvent', () => {
  function makeEvent(typename: string): RoomEventViewFragment {
    return { event: { __typename: typename } } as unknown as RoomEventViewFragment;
  }

  it('refreshes on UserJoinedRoomEvent', async () => {
    const { client, queryMock } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();
    expect(queryMock).toHaveBeenCalledTimes(1);

    store.ingestSpaceEvent(makeEvent('UserJoinedRoomEvent'));
    await settle();
    expect(queryMock).toHaveBeenCalledTimes(2);
  });

  it('refreshes on UserLeftRoomEvent', async () => {
    const { client, queryMock } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.ingestSpaceEvent(makeEvent('UserLeftRoomEvent'));
    await settle();
    expect(queryMock).toHaveBeenCalledTimes(2);
  });

  it('refreshes on RoomArchivedEvent and RoomUnarchivedEvent', async () => {
    const { client, queryMock } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.ingestSpaceEvent(makeEvent('RoomArchivedEvent'));
    await settle();
    store.ingestSpaceEvent(makeEvent('RoomUnarchivedEvent'));
    await settle();

    expect(queryMock).toHaveBeenCalledTimes(3);
  });

  it('does NOT refresh on irrelevant event types', async () => {
    const { client, queryMock } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.ingestSpaceEvent(makeEvent('MessagePostedEvent'));
    store.ingestSpaceEvent(makeEvent('ReactionAddedEvent'));
    await settle();

    expect(queryMock).toHaveBeenCalledTimes(1);
  });

  it('ingestRoomLayoutUpdated triggers a refresh', async () => {
    const { client, queryMock } = makeClient({
      query: { server: { id: SPACE_ID, rooms: [] } }
    });
    const store = new RoomDirectoryStore(client, SPACE_ID);
    await settle();

    store.ingestRoomLayoutUpdated();
    await settle();
    expect(queryMock).toHaveBeenCalledTimes(2);
  });
});

describe('RoomDirectoryStore — concurrent refresh guard', () => {
  it('discards out-of-order responses', async () => {
    let resolveFirst!: (value: { data: QueryResponse; error: null }) => void;
    let resolveSecond!: (value: { data: QueryResponse; error: null }) => void;

    const queryMock = vi
      .fn()
      .mockImplementationOnce(() => ({
        toPromise: () => new Promise((r) => (resolveFirst = r))
      }))
      .mockImplementationOnce(() => ({
        toPromise: () => new Promise((r) => (resolveSecond = r))
      }));
    const client = { query: queryMock, mutation: vi.fn() } as unknown as Client;

    const store = new RoomDirectoryStore(client, SPACE_ID);
    void store.refresh(); // a second concurrent load

    // Resolve the SECOND load first (out-of-order)
    resolveSecond({
      data: { server: { id: SPACE_ID, rooms: [makeRoom('newer')] } },
      error: null
    });
    await settle();

    expect(store.allRooms.map((r) => r.id)).toEqual(['newer']);

    // The earlier load now resolves — should be ignored
    resolveFirst({
      data: { server: { id: SPACE_ID, rooms: [makeRoom('older')] } },
      error: null
    });
    await settle();

    expect(store.allRooms.map((r) => r.id)).toEqual(['newer']);
  });
});
