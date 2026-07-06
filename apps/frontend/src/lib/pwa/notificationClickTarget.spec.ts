import { describe, expect, it, vi } from 'vitest';
import {
  NOTIFICATION_CLICK_TARGET_CACHE_NAME,
  NOTIFICATION_CLICK_TARGET_MAX_AGE_MS,
  NOTIFICATION_CLICK_TARGET_PATH,
  clearPendingNotificationClickTarget,
  consumePendingNotificationClickTarget,
  writePendingNotificationClickTarget
} from './notificationClickTarget';

function createMemoryCacheStorage(initialPayload?: unknown) {
  const entries = new Map<string, Response>();
  if (initialPayload !== undefined) {
    entries.set(
      `https://chatto.example${NOTIFICATION_CLICK_TARGET_PATH}`,
      new Response(JSON.stringify(initialPayload))
    );
  }

  const cache = {
    match: vi.fn(async (request: Request) => entries.get(request.url)?.clone()),
    put: vi.fn(async (request: Request, response: Response) => {
      entries.set(request.url, response.clone());
    }),
    delete: vi.fn(async (request: Request) => entries.delete(request.url))
  };
  const cacheStorage = {
    open: vi.fn(async () => cache)
  };

  return { cache, cacheStorage, entries };
}

describe('notification click target storage', () => {
  it('writes and consumes a pending same-origin target once', async () => {
    const { cacheStorage } = createMemoryCacheStorage();
    const target = 'https://chatto.example/chat/-/room-1?highlight=event-1';

    await writePendingNotificationClickTarget(cacheStorage, 'https://chatto.example', target, 100);

    await expect(
      consumePendingNotificationClickTarget(cacheStorage, 'https://chatto.example', { now: 150 })
    ).resolves.toBe(target);
    await expect(
      consumePendingNotificationClickTarget(cacheStorage, 'https://chatto.example', { now: 151 })
    ).resolves.toBeNull();
    expect(cacheStorage.open).toHaveBeenCalledWith(NOTIFICATION_CLICK_TARGET_CACHE_NAME);
  });

  it('deletes and ignores stale, future, malformed, and cross-origin payloads', async () => {
    for (const payload of [
      {
        url: 'https://chatto.example/chat/-/room-1',
        createdAt: 1
      },
      {
        url: 'https://chatto.example/chat/-/room-1',
        createdAt: NOTIFICATION_CLICK_TARGET_MAX_AGE_MS + 10_000
      },
      {
        url: 'http://[',
        createdAt: 100
      },
      {
        url: 'https://other.example/chat/-/room-1',
        createdAt: 100
      },
      {
        url: 42,
        createdAt: 100
      }
    ]) {
      const { cache, cacheStorage } = createMemoryCacheStorage(payload);

      await expect(
        consumePendingNotificationClickTarget(cacheStorage, 'https://chatto.example', {
          maxAgeMs: NOTIFICATION_CLICK_TARGET_MAX_AGE_MS,
          now: NOTIFICATION_CLICK_TARGET_MAX_AGE_MS + 2
        })
      ).resolves.toBeNull();
      expect(cache.delete).toHaveBeenCalledOnce();
    }
  });

  it('clears only the expected pending target when an expected URL is provided', async () => {
    const target = 'https://chatto.example/chat/-/room-1';
    const { cache, cacheStorage, entries } = createMemoryCacheStorage({
      url: target,
      createdAt: 100
    });

    await clearPendingNotificationClickTarget(
      cacheStorage,
      'https://chatto.example',
      'https://chatto.example/chat/-/room-2'
    );

    expect(cache.delete).not.toHaveBeenCalled();
    expect(entries.size).toBe(1);

    await clearPendingNotificationClickTarget(cacheStorage, 'https://chatto.example', target);

    expect(cache.delete).toHaveBeenCalledOnce();
    expect(entries.size).toBe(0);
  });
});
