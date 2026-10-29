import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  captureBrowserPushRecovery,
  createBrowserPushSubscription
} from './browserPushRegistration';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

describe('browser push creation gate', () => {
  let storage: Map<string, string>;
  let localStorage: {
    getItem: ReturnType<typeof vi.fn>;
    setItem: ReturnType<typeof vi.fn>;
    removeItem: ReturnType<typeof vi.fn>;
  };
  const subscription = { endpoint: 'https://push.example/test' } as PushSubscription;

  function operation() {
    return {
      cancelled: vi.fn(() => false),
      existing: vi.fn(async (): Promise<PushSubscription | null> => null),
      subscribe: vi.fn(async () => subscription)
    };
  }

  beforeEach(() => {
    storage = new Map();
    localStorage = {
      getItem: vi.fn((key: string) => storage.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => {
        storage.set(key, value);
      }),
      removeItem: vi.fn((key: string) => {
        storage.delete(key);
      })
    };
    vi.stubGlobal('window', { localStorage });
    // Independent module instances share the origin's storage and Web Locks,
    // as separate tabs do. Locks with different names can run concurrently.
    const tails = new Map<string, Promise<unknown>>();
    vi.stubGlobal('navigator', {
      locks: {
        request: vi.fn((name: string, callback: () => Promise<unknown>) => {
          const result = (tails.get(name) ?? Promise.resolve()).then(callback);
          tails.set(
            name,
            result.catch(() => undefined)
          );
          return result;
        })
      }
    });
  });

  afterEach(() => vi.unstubAllGlobals());

  it('can create while holding the registration lock for a server named browser', async () => {
    await expect(
      navigator.locks.request('chatto.push-registration.browser', () =>
        createBrowserPushSubscription(operation())
      )
    ).resolves.toBe(subscription);
  });

  it('keeps a failure stopped after the page module is loaded again', async () => {
    const first = operation();
    first.subscribe.mockRejectedValueOnce(new DOMException('push service error', 'AbortError'));
    await expect(createBrowserPushSubscription(first)).rejects.toThrow('push service error');
    vi.resetModules();
    const reloaded = await import('./browserPushRegistration');
    const next = operation();
    await expect(reloaded.createBrowserPushSubscription(next)).rejects.toThrow(
      'Automatic push setup is paused'
    );
    expect(next.subscribe).not.toHaveBeenCalled();
  });

  it('serializes independent tabs and stops a waiting server after failure', async () => {
    vi.resetModules();
    const otherTab = await import('./browserPushRegistration');
    const pending = deferred<PushSubscription>();
    const first = operation();
    first.subscribe.mockReturnValueOnce(pending.promise);
    const result = createBrowserPushSubscription(first);
    const rejected = expect(result).rejects.toThrow('offline');
    await vi.waitFor(() => expect(first.subscribe).toHaveBeenCalledOnce());
    const second = operation();
    const waiting = expect(otherTab.createBrowserPushSubscription(second)).rejects.toThrow(
      'Automatic push setup is paused'
    );
    expect(second.subscribe).not.toHaveBeenCalled();
    pending.reject(new Error('offline'));
    await rejected;
    await waiting;
    expect(second.subscribe).not.toHaveBeenCalled();
  });

  it('ends a manual recovery batch on its first new failure', async () => {
    const recovery = captureBrowserPushRecovery();
    const first = operation();
    first.subscribe.mockRejectedValueOnce(new Error('offline'));
    await expect(createBrowserPushSubscription({ ...first, recovery })).rejects.toThrow('offline');
    const second = operation();
    await expect(createBrowserPushSubscription({ ...second, recovery })).rejects.toThrow(
      'Automatic push setup is paused'
    );
    expect(second.subscribe).not.toHaveBeenCalled();
    await expect(
      createBrowserPushSubscription({ ...second, recovery: captureBrowserPushRecovery() })
    ).resolves.toBe(subscription);
    await expect(createBrowserPushSubscription(operation())).resolves.toBe(subscription);
  });

  it('leaves a durable stop before a page can exit during creation', async () => {
    const pending = deferred<PushSubscription>();
    const first = operation();
    first.subscribe.mockReturnValueOnce(pending.promise);
    const result = createBrowserPushSubscription(first);
    await vi.waitFor(() => expect(first.subscribe).toHaveBeenCalledOnce());
    expect(captureBrowserPushRecovery().attempt).not.toBeNull();
    pending.resolve(subscription);
    await result;
    expect(captureBrowserPushRecovery().attempt).toBeNull();
  });

  it('does not let a recovery queued during creation bypass its new failure', async () => {
    const pending = deferred<PushSubscription>();
    const first = operation();
    first.subscribe.mockReturnValueOnce(pending.promise);
    const rejected = expect(createBrowserPushSubscription(first)).rejects.toThrow('offline');
    await vi.waitFor(() => expect(first.subscribe).toHaveBeenCalledOnce());
    const second = operation();
    const waiting = expect(
      createBrowserPushSubscription({ ...second, recovery: captureBrowserPushRecovery() })
    ).rejects.toThrow('Automatic push setup is paused');
    pending.reject(new Error('offline'));
    await rejected;
    await waiting;
    expect(second.subscribe).not.toHaveBeenCalled();
  });

  it('reuses an existing subscription even when creation is paused', async () => {
    const failed = operation();
    failed.subscribe.mockRejectedValueOnce(new Error('offline'));
    await expect(createBrowserPushSubscription(failed)).rejects.toThrow();
    const existing = operation();
    existing.existing.mockResolvedValue(subscription);
    await expect(createBrowserPushSubscription(existing)).resolves.toBe(subscription);
    expect(existing.subscribe).not.toHaveBeenCalled();
    expect(captureBrowserPushRecovery().attempt).not.toBeNull();
  });

  it.each(['throw', 'discard'])('does not create when storage writes %s', async (mode) => {
    localStorage.setItem.mockImplementation(() => {
      if (mode === 'throw') throw new Error('storage unavailable');
    });
    const next = operation();
    await expect(createBrowserPushSubscription(next)).rejects.toThrow();
    expect(next.subscribe).not.toHaveBeenCalled();
  });

  it('skips work cancelled while waiting for the browser lock', async () => {
    const pending = deferred<PushSubscription>();
    const first = operation();
    first.subscribe.mockReturnValueOnce(pending.promise);
    const result = createBrowserPushSubscription(first);
    await vi.waitFor(() => expect(first.subscribe).toHaveBeenCalledOnce());
    const cancelled = operation();
    const waiting = createBrowserPushSubscription(cancelled);
    cancelled.cancelled.mockReturnValue(true);
    pending.resolve(subscription);
    await result;
    await expect(waiting).resolves.toBeNull();
    expect(cancelled.subscribe).not.toHaveBeenCalled();
  });
});
