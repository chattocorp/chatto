export const NOTIFICATION_CLICK_TARGET_CACHE_NAME = 'chatto-notification-click-v1';
export const NOTIFICATION_CLICK_TARGET_PATH_PREFIX = '/__chatto/notification-click-target/';
export const NOTIFICATION_CLICK_TARGET_MAX_AGE_MS = 10 * 60 * 1000;

type NotificationClickTargetPayload = {
  id: string;
  url: string;
  createdAt: number;
};

export type PendingNotificationClickTarget = {
  id: string;
  url: string;
};

type NotificationClickTargetCache = Pick<Cache, 'match' | 'put' | 'delete' | 'keys'>;
type NotificationClickTargetCacheStorage = {
  open(name: string): Promise<NotificationClickTargetCache>;
};

type WritePendingNotificationClickTargetOptions = {
  id?: string;
  now?: number;
};

type ClearPendingNotificationClickTargetOptions = {
  expectedUrl?: string;
  id?: string;
};

function createPendingClickId(): string {
  return (
    globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  );
}

function targetRequest(origin: string, id: string): Request {
  return new Request(
    new URL(`${NOTIFICATION_CLICK_TARGET_PATH_PREFIX}${encodeURIComponent(id)}`, origin).href
  );
}

function targetIdFromRequest(request: Request, origin: string): string | null {
  try {
    const url = new URL(request.url);
    if (url.origin !== origin || !url.pathname.startsWith(NOTIFICATION_CLICK_TARGET_PATH_PREFIX)) {
      return null;
    }
    const id = decodeURIComponent(url.pathname.slice(NOTIFICATION_CLICK_TARGET_PATH_PREFIX.length));
    return id || null;
  } catch {
    return null;
  }
}

function isPendingTargetPayload(value: unknown): value is NotificationClickTargetPayload {
  return (
    typeof value === 'object' &&
    value !== null &&
    'id' in value &&
    typeof value.id === 'string' &&
    'url' in value &&
    typeof value.url === 'string' &&
    'createdAt' in value &&
    typeof value.createdAt === 'number' &&
    Number.isFinite(value.createdAt)
  );
}

async function readPendingTargetPayload(
  cache: NotificationClickTargetCache,
  origin: string,
  id: string
): Promise<NotificationClickTargetPayload | null> {
  const response = await cache.match(targetRequest(origin, id));
  if (!response) return null;

  try {
    const payload: unknown = await response.json();
    if (!isPendingTargetPayload(payload)) return null;
    return payload.id === id ? payload : null;
  } catch {
    return null;
  }
}

function isPayloadCurrent(
  payload: NotificationClickTargetPayload,
  origin: string,
  now: number,
  maxAgeMs: number
): payload is NotificationClickTargetPayload {
  if (payload.createdAt > now || now - payload.createdAt > maxAgeMs) return false;

  try {
    const target = new URL(payload.url);
    return target.origin === origin;
  } catch {
    return false;
  }
}

export async function writePendingNotificationClickTarget(
  cacheStorage: NotificationClickTargetCacheStorage,
  origin: string,
  url: string,
  options: number | WritePendingNotificationClickTargetOptions = {}
): Promise<PendingNotificationClickTarget> {
  const id =
    typeof options === 'object' && options.id !== undefined ? options.id : createPendingClickId();
  const now = typeof options === 'number' ? options : (options.now ?? Date.now());
  const cache = await cacheStorage.open(NOTIFICATION_CLICK_TARGET_CACHE_NAME);
  const payload: NotificationClickTargetPayload = { id, url, createdAt: now };
  await cache.put(
    targetRequest(origin, id),
    new Response(JSON.stringify(payload), {
      headers: {
        'cache-control': 'no-store',
        'content-type': 'application/json'
      }
    })
  );
  return { id, url };
}

export async function consumePendingNotificationClickTarget(
  cacheStorage: NotificationClickTargetCacheStorage,
  origin: string,
  options: {
    maxAgeMs?: number;
    now?: number;
  } = {}
): Promise<string | null> {
  const cache = await cacheStorage.open(NOTIFICATION_CLICK_TARGET_CACHE_NAME);
  const now = options.now ?? Date.now();
  const maxAgeMs = options.maxAgeMs ?? NOTIFICATION_CLICK_TARGET_MAX_AGE_MS;

  let newest: NotificationClickTargetPayload | null = null;
  for (const request of await cache.keys()) {
    const id = targetIdFromRequest(request, origin);
    if (!id) continue;

    const payload = await readPendingTargetPayload(cache, origin, id);
    await cache.delete(request);
    if (!payload || !isPayloadCurrent(payload, origin, now, maxAgeMs)) continue;
    if (!newest || payload.createdAt > newest.createdAt) {
      newest = payload;
    }
  }

  return newest ? new URL(newest.url).href : null;
}

export async function clearPendingNotificationClickTarget(
  cacheStorage: NotificationClickTargetCacheStorage,
  origin: string,
  options: ClearPendingNotificationClickTargetOptions = {}
): Promise<void> {
  const cache = await cacheStorage.open(NOTIFICATION_CLICK_TARGET_CACHE_NAME);
  if (options.id) {
    const payload = await readPendingTargetPayload(cache, origin, options.id);
    if (options.expectedUrl && payload?.url !== options.expectedUrl) return;
    await cache.delete(targetRequest(origin, options.id));
    return;
  }

  if (options.expectedUrl) {
    for (const request of await cache.keys()) {
      const id = targetIdFromRequest(request, origin);
      if (!id) continue;
      const payload = await readPendingTargetPayload(cache, origin, id);
      if (payload?.url === options.expectedUrl) {
        await cache.delete(request);
      }
    }
    return;
  }

  for (const request of await cache.keys()) {
    if (targetIdFromRequest(request, origin)) {
      await cache.delete(request);
    }
  }
}
