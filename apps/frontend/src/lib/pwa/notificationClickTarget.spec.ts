import { describe, expect, it, vi } from 'vitest';
import {
  NOTIFICATION_CLICK_TARGET_CACHE_NAME,
  NOTIFICATION_CLICK_TARGET_MAX_AGE_MS,
  NOTIFICATION_CLICK_TARGET_PATH_PREFIX,
  clearPendingNotificationClickTarget,
  consumePendingNotificationClickTarget,
  writePendingNotificationClickTarget
} from './notificationClickTarget';

function pendingTargetUrl(id: string): string {
  return `https://chatto.example${NOTIFICATION_CLICK_TARGET_PATH_PREFIX}${encodeURIComponent(id)}`;
}

function createMemoryCacheStorage(initialPayloads: Record<string, unknown> = {}) {
  const entries = new Map<string, Response>();
  for (const [id, payload] of Object.entries(initialPayloads)) {
    entries.set(pendingTargetUrl(id), new Response(JSON.stringify(payload)));
  }

  const cache = {
    match: vi.fn(async (request: Request) => entries.get(request.url)?.clone()),
    put: vi.fn(async (request: Request, response: Response) => {
      entries.set(request.url, response.clone());
    }),
    delete: vi.fn(async (request: Request) => entries.delete(request.url)),
    keys: vi.fn(async () => Array.from(entries.keys()).map((url) => new Request(url)))
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

    await expect(
      writePendingNotificationClickTarget(cacheStorage, 'https://chatto.example', target, {
        id: 'click-1',
        now: 100
      })
    ).resolves.toEqual({ id: 'click-1', url: target });

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
        id: 'click-1',
        url: 'https://chatto.example/chat/-/room-1',
        createdAt: 1
      },
      {
        id: 'click-1',
        url: 'https://chatto.example/chat/-/room-1',
        createdAt: NOTIFICATION_CLICK_TARGET_MAX_AGE_MS + 10_000
      },
      {
        id: 'click-1',
        url: 'http://[',
        createdAt: 100
      },
      {
        id: 'click-1',
        url: 'https://other.example/chat/-/room-1',
        createdAt: 100
      },
      {
        id: 'click-1',
        url: 42,
        createdAt: 100
      }
    ]) {
      const { cache, cacheStorage } = createMemoryCacheStorage({ 'click-1': payload });

      await expect(
        consumePendingNotificationClickTarget(cacheStorage, 'https://chatto.example', {
          maxAgeMs: NOTIFICATION_CLICK_TARGET_MAX_AGE_MS,
          now: NOTIFICATION_CLICK_TARGET_MAX_AGE_MS + 2
        })
      ).resolves.toBeNull();
      expect(cache.delete).toHaveBeenCalledOnce();
    }
  });

  it('consumes the newest valid target and clears older pending entries', async () => {
    const older = 'https://chatto.example/chat/-/older';
    const newer = 'https://chatto.example/chat/-/newer';
    const { cacheStorage, entries } = createMemoryCacheStorage({
      older: { id: 'older', url: older, createdAt: 100 },
      newer: { id: 'newer', url: newer, createdAt: 200 }
    });

    await expect(
      consumePendingNotificationClickTarget(cacheStorage, 'https://chatto.example', { now: 250 })
    ).resolves.toBe(newer);
    expect(entries.size).toBe(0);
  });

  it('clears only the pending target for the provided click id', async () => {
    const older = 'https://chatto.example/chat/-/room-1';
    const newer = 'https://chatto.example/chat/-/room-2';
    const { cache, cacheStorage, entries } = createMemoryCacheStorage({
      old: { id: 'old', url: older, createdAt: 100 },
      new: { id: 'new', url: newer, createdAt: 200 }
    });

    await clearPendingNotificationClickTarget(cacheStorage, 'https://chatto.example', {
      id: 'old',
      expectedUrl: 'https://chatto.example/chat/-/wrong-room'
    });

    expect(cache.delete).not.toHaveBeenCalled();
    expect(entries.size).toBe(2);

    await clearPendingNotificationClickTarget(cacheStorage, 'https://chatto.example', {
      id: 'old',
      expectedUrl: older
    });

    expect(cache.delete).toHaveBeenCalledOnce();
    expect(entries.has(pendingTargetUrl('new'))).toBe(true);
    expect(entries.has(pendingTargetUrl('old'))).toBe(false);
  });
});
