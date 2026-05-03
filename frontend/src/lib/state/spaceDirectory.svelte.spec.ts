import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import type { Client } from '@urql/svelte';
import type { RegisteredInstance } from '$lib/state/instance/registry.svelte';
import {
  SpaceDirectoryStore,
  type ClientManager,
  type InstanceSpaceData
} from './spaceDirectory.svelte';

type SpaceFixture = { id: string; name: string; description?: string };
type LoadResponse = {
  data: { spaces: SpaceFixture[] | null; viewer: { canListSpaces: boolean } | null } | null;
  error?: { message: string } | null;
};

function instance(id: string, name = id): RegisteredInstance {
  return { id, name, url: `https://${id}` } as RegisteredInstance;
}

/**
 * Build a stub `ClientManager` that returns a per-instance urql client. Each
 * client's `.query` is configurable via the `responses` map; missing entries
 * default to an empty success.
 */
function makeClientManager(
  responses: Record<string, LoadResponse | (() => Promise<LoadResponse>)>,
  joinResponses: Record<string, { error?: { message: string } | null }> = {}
) {
  const queryMocks: Record<string, ReturnType<typeof vi.fn>> = {};
  const mutationMocks: Record<string, ReturnType<typeof vi.fn>> = {};

  const getClient = (instanceId: string): { client: Client } => {
    const queryMock =
      queryMocks[instanceId] ??
      (queryMocks[instanceId] = vi.fn(() => {
        const r = responses[instanceId];
        const value = typeof r === 'function' ? r() : Promise.resolve(r ?? { data: null });
        return { toPromise: () => value };
      }));
    const mutationMock =
      mutationMocks[instanceId] ??
      (mutationMocks[instanceId] = vi.fn(() => ({
        toPromise: () =>
          Promise.resolve({
            data: joinResponses[instanceId]?.error ? null : { joinSpace: true },
            error: joinResponses[instanceId]?.error ?? null
          })
      })));
    return {
      client: { query: queryMock, mutation: mutationMock } as unknown as Client
    };
  };

  return { manager: { getClient } satisfies ClientManager, queryMocks, mutationMocks };
}

async function settle() {
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function dataOf(store: SpaceDirectoryStore, id: string): InstanceSpaceData | undefined {
  return store.instanceData.get(id);
}

// ---------------------------------------------------------------------------
// loadAll
// ---------------------------------------------------------------------------

describe('SpaceDirectoryStore — loadAll', () => {
  it('populates per-instance data on success', async () => {
    const { manager } = makeClientManager({
      i1: {
        data: { spaces: [{ id: 's1', name: 'Alpha' }], viewer: { canListSpaces: true } }
      }
    });
    const store = new SpaceDirectoryStore(manager);

    await store.loadAll([instance('i1')]);
    await settle();

    const d = dataOf(store, 'i1')!;
    expect(d.loading).toBe(false);
    expect(d.canBrowse).toBe(true);
    expect(d.spaces).toHaveLength(1);
    expect(d.error).toBeNull();
  });

  it('captures GraphQL errors per-instance and marks loading=false', async () => {
    const { manager } = makeClientManager({
      i1: { data: null, error: { message: 'boom' } }
    });
    const store = new SpaceDirectoryStore(manager);

    await store.loadAll([instance('i1')]);
    await settle();

    const d = dataOf(store, 'i1')!;
    expect(d.error).toBe('boom');
    expect(d.loading).toBe(false);
    expect(d.spaces).toEqual([]);
  });

  it('records canBrowse=false when viewer cannot list spaces (and stores no spaces)', async () => {
    const { manager } = makeClientManager({
      i1: {
        data: {
          spaces: [{ id: 's1', name: 'Hidden' }],
          viewer: { canListSpaces: false }
        }
      }
    });
    const store = new SpaceDirectoryStore(manager);

    await store.loadAll([instance('i1')]);
    await settle();

    const d = dataOf(store, 'i1')!;
    expect(d.canBrowse).toBe(false);
    expect(d.spaces).toEqual([]);
  });

  it('captures network/transport errors thrown by the client', async () => {
    const { manager } = makeClientManager({
      i1: () => Promise.reject(new Error('network down'))
    });
    const store = new SpaceDirectoryStore(manager);

    await store.loadAll([instance('i1')]);
    await settle();

    const d = dataOf(store, 'i1')!;
    expect(d.error).toBe('network down');
    expect(d.loading).toBe(false);
  });

  it('drops entries for instances that are no longer in the authenticated set', async () => {
    const { manager } = makeClientManager({
      i1: { data: { spaces: [], viewer: { canListSpaces: true } } },
      i2: { data: { spaces: [], viewer: { canListSpaces: true } } }
    });
    const store = new SpaceDirectoryStore(manager);

    await store.loadAll([instance('i1'), instance('i2')]);
    await settle();
    expect(store.instanceData.size).toBe(2);

    // i2 disconnected
    await store.loadAll([instance('i1')]);
    await settle();
    expect(store.instanceData.size).toBe(1);
    expect(store.instanceData.has('i1')).toBe(true);
    expect(store.instanceData.has('i2')).toBe(false);
  });

  it('runs per-instance queries in parallel', async () => {
    let resolveSlow!: (v: LoadResponse) => void;
    const slowPromise = new Promise<LoadResponse>((r) => (resolveSlow = r));
    const { manager, queryMocks } = makeClientManager({
      i1: { data: { spaces: [], viewer: { canListSpaces: true } } },
      i2: () => slowPromise
    });
    const store = new SpaceDirectoryStore(manager);

    const loadPromise = store.loadAll([instance('i1'), instance('i2')]);
    await settle();

    // Both queries fired despite i2 being slow.
    expect(queryMocks.i1).toHaveBeenCalledTimes(1);
    expect(queryMocks.i2).toHaveBeenCalledTimes(1);

    // i1 has already settled while i2 is still loading.
    expect(dataOf(store, 'i1')!.loading).toBe(false);
    expect(dataOf(store, 'i2')!.loading).toBe(true);

    resolveSlow({ data: { spaces: [], viewer: { canListSpaces: true } } });
    await loadPromise;
    await settle();
    expect(dataOf(store, 'i2')!.loading).toBe(false);
  });

  it('ignores results for instances removed mid-flight', async () => {
    let resolveI2!: (v: LoadResponse) => void;
    const { manager } = makeClientManager({
      i1: { data: { spaces: [], viewer: { canListSpaces: true } } },
      i2: () => new Promise<LoadResponse>((r) => (resolveI2 = r))
    });
    const store = new SpaceDirectoryStore(manager);

    void store.loadAll([instance('i1'), instance('i2')]);
    await settle();

    // i2 disappears before its query resolves
    await store.loadAll([instance('i1')]);
    await settle();
    expect(store.instanceData.has('i2')).toBe(false);

    // The slow i2 result arrives — it should be discarded
    resolveI2({ data: { spaces: [{ id: 'late', name: 'Late' }], viewer: { canListSpaces: true } } });
    await settle();
    expect(store.instanceData.has('i2')).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// joinSpace
// ---------------------------------------------------------------------------

describe('SpaceDirectoryStore — joinSpace', () => {
  it('sets joiningKey during the request and clears it afterwards', async () => {
    const { manager } = makeClientManager({});
    const store = new SpaceDirectoryStore(manager);

    const promise = store.joinSpace('i1', 's1');
    expect(store.joiningKey).toBe('i1:s1');

    const result = await promise;
    expect(result.ok).toBe(true);
    expect(store.joiningKey).toBeNull();
  });

  it('returns an error result on failure (and still clears joiningKey)', async () => {
    const { manager } = makeClientManager(
      {},
      { i1: { error: { message: 'permission denied' } } }
    );
    const store = new SpaceDirectoryStore(manager);

    const result = await store.joinSpace('i1', 's1');
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.message).toBe('permission denied');
    expect(store.joiningKey).toBeNull();
  });
});
