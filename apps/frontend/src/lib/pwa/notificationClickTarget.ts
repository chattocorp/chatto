export const NOTIFICATION_CLICK_TARGET_CACHE_NAME = 'chatto-notification-click-v1';
export const NOTIFICATION_CLICK_TARGET_PATH = '/__chatto/notification-click-target';
export const NOTIFICATION_CLICK_TARGET_MAX_AGE_MS = 10 * 60 * 1000;

type NotificationClickTargetPayload = {
  url: string;
  createdAt: number;
};

type NotificationClickTargetCache = Pick<Cache, 'match' | 'put' | 'delete'>;
type NotificationClickTargetCacheStorage = {
  open(name: string): Promise<NotificationClickTargetCache>;
};

function targetRequest(origin: string): Request {
  return new Request(new URL(NOTIFICATION_CLICK_TARGET_PATH, origin).href);
}

function isPendingTargetPayload(value: unknown): value is NotificationClickTargetPayload {
  return (
    typeof value === 'object' &&
    value !== null &&
    'url' in value &&
    typeof value.url === 'string' &&
    'createdAt' in value &&
    typeof value.createdAt === 'number' &&
    Number.isFinite(value.createdAt)
  );
}

async function readPendingTargetPayload(
  cache: NotificationClickTargetCache,
  origin: string
): Promise<NotificationClickTargetPayload | null> {
  const response = await cache.match(targetRequest(origin));
  if (!response) return null;

  try {
    const payload: unknown = await response.json();
    return isPendingTargetPayload(payload) ? payload : null;
  } catch {
    return null;
  }
}

export async function writePendingNotificationClickTarget(
  cacheStorage: NotificationClickTargetCacheStorage,
  origin: string,
  url: string,
  now = Date.now()
): Promise<void> {
  const cache = await cacheStorage.open(NOTIFICATION_CLICK_TARGET_CACHE_NAME);
  const payload: NotificationClickTargetPayload = { url, createdAt: now };
  await cache.put(
    targetRequest(origin),
    new Response(JSON.stringify(payload), {
      headers: {
        'cache-control': 'no-store',
        'content-type': 'application/json'
      }
    })
  );
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
  const request = targetRequest(origin);
  const payload = await readPendingTargetPayload(cache, origin);
  await cache.delete(request);

  if (!payload) return null;

  const now = options.now ?? Date.now();
  const maxAgeMs = options.maxAgeMs ?? NOTIFICATION_CLICK_TARGET_MAX_AGE_MS;
  if (payload.createdAt > now || now - payload.createdAt > maxAgeMs) return null;

  try {
    const target = new URL(payload.url);
    if (target.origin !== origin) return null;
    return target.href;
  } catch {
    return null;
  }
}

export async function clearPendingNotificationClickTarget(
  cacheStorage: NotificationClickTargetCacheStorage,
  origin: string,
  expectedUrl?: string
): Promise<void> {
  const cache = await cacheStorage.open(NOTIFICATION_CLICK_TARGET_CACHE_NAME);
  if (expectedUrl) {
    const payload = await readPendingTargetPayload(cache, origin);
    if (payload?.url !== expectedUrl) return;
  }
  await cache.delete(targetRequest(origin));
}
