import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { retireOfflineShell } from './retireOfflineShell';

/** Browser API doubles keep registration identity and cache ownership visible to each test. */
function registration(scope = '/', script = '/service-worker.js') {
  return {
    scope: new URL(scope, 'https://chatto.example').href,
    active: { scriptURL: new URL(script, 'https://chatto.example').href },
    waiting: null,
    installing: null,
    pushManager: { getSubscription: vi.fn(async (): Promise<object | null> => null) },
    update: vi.fn(async () => {}),
    unregister: vi.fn(async () => true)
  };
}

const getRegistrations = vi.fn<() => Promise<ReturnType<typeof registration>[]>>();
const cacheNames = new Set<string>();
const deleteCache = vi.fn(async (name: string) => cacheNames.delete(name));

describe('retireOfflineShell', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getRegistrations.mockResolvedValue([]);
    cacheNames.clear();
    for (const name of [
      'chatto-shell-v1',
      'chatto-shell-v2',
      'chatto-badge-state-v1',
      'chatto-badge-state-v2',
      'unrelated-cache'
    ])
      cacheNames.add(name);
    vi.stubGlobal('window', { location: { origin: 'https://chatto.example' } });
    vi.stubGlobal('navigator', { serviceWorker: { getRegistrations } });
    vi.stubGlobal('caches', {
      keys: vi.fn(async () => [...cacheNames]),
      delete: deleteCache
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  it('does not register a worker on a fresh browser and deletes only retired caches', async () => {
    await retireOfflineShell();

    expect(getRegistrations).toHaveBeenCalledOnce();
    expect([...cacheNames]).toEqual(['unrelated-cache']);
  });

  it('removes an unsubscribed Chatto root while leaving scoped push and other workers intact', async () => {
    const root = registration();
    const push = registration('/__chatto/push/server/');
    const other = registration('/another-app/', '/another-worker.js');
    getRegistrations.mockResolvedValue([root, push, other]);

    await retireOfflineShell();

    expect(root.unregister).toHaveBeenCalledOnce();
    expect(root.update).not.toHaveBeenCalled();
    expect(push.unregister).not.toHaveBeenCalled();
    expect(push.update).not.toHaveBeenCalled();
    expect(other.unregister).not.toHaveBeenCalled();
    expect([...cacheNames]).toEqual(['unrelated-cache']);
  });

  it('does not unregister a root owned by another script', async () => {
    const other = registration('/', '/another-worker.js');
    getRegistrations.mockResolvedValue([other]);

    await retireOfflineShell();

    expect(other.unregister).not.toHaveBeenCalled();
    expect(other.update).not.toHaveBeenCalled();
  });

  it('updates a subscribed legacy root and removes it on a later visit after push migration', async () => {
    const root = registration();
    root.pushManager.getSubscription.mockResolvedValueOnce({});
    getRegistrations.mockResolvedValue([root]);

    await retireOfflineShell();
    expect(root.update).toHaveBeenCalledOnce();
    expect(root.unregister).not.toHaveBeenCalled();
    expect([...cacheNames]).toEqual(['unrelated-cache']);

    await retireOfflineShell();
    expect(root.unregister).toHaveBeenCalledOnce();
  });

  it.each(['subscription', 'update', 'enumeration'])(
    'preserves registrations and deletes caches when %s fails',
    async (operation) => {
      const root = registration();
      const error = new Error('Browser API unavailable');
      getRegistrations.mockResolvedValue([root]);
      root.pushManager.getSubscription.mockResolvedValue({});
      if (operation === 'subscription') root.pushManager.getSubscription.mockRejectedValue(error);
      if (operation === 'update') root.update.mockRejectedValue(error);
      if (operation === 'enumeration') getRegistrations.mockRejectedValue(error);

      await expect(retireOfflineShell()).rejects.toBe(error);
      expect(root.unregister).not.toHaveBeenCalled();
      expect([...cacheNames]).toEqual(['unrelated-cache']);
    }
  );

  it('retries cache deletion after a partial failure', async () => {
    deleteCache.mockRejectedValueOnce(new Error('Storage unavailable'));
    await expect(retireOfflineShell()).rejects.toThrow('Storage unavailable');

    await retireOfflineShell();

    expect([...cacheNames]).toEqual(['unrelated-cache']);
  });

  it('cleans caches without service-worker support', async () => {
    vi.stubGlobal('navigator', {});

    await retireOfflineShell();

    expect(getRegistrations).not.toHaveBeenCalled();
    expect([...cacheNames]).toEqual(['unrelated-cache']);
  });

  it('works when Cache Storage is unavailable', async () => {
    vi.stubGlobal('caches', undefined);
    const root = registration();
    getRegistrations.mockResolvedValue([root]);

    await retireOfflineShell();

    expect(root.unregister).toHaveBeenCalledOnce();
  });
});
