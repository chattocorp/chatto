export const NOTIFICATION_CLICK_MESSAGE_TYPE = 'notification-click';
/**
 * Reply type that workers from earlier releases wait for. Current workers do
 * not wait for a reply; pages still send it so that an earlier worker does
 * not also open a new window.
 */
export const NOTIFICATION_CLICK_ACK_MESSAGE_TYPE = 'notification-click-ack';
const NOTIFICATION_CLICK_FALLBACK_PATH = '/chat';

export interface NotificationClickClient {
  focused?: boolean;
  visibilityState?: DocumentVisibilityState;
  focus?: () => Promise<NotificationClickClient | null>;
  postMessage?: (message: unknown) => void;
}

export interface NotificationClickClients {
  matchAll(options: {
    type: 'window';
    includeUncontrolled: true;
  }): Promise<readonly NotificationClickClient[]>;
  openWindow(url: string): Promise<NotificationClickClient | null>;
}

interface NotificationClickLogger {
  warn: (...args: unknown[]) => void;
}

/** How a click was routed: an open window got it (`client`), or a new window opened the target (`open`). */
export type NotificationClickRouteResult = 'client' | 'open';

export interface NotificationClickRouteOptions {
  logger?: NotificationClickLogger;
}

function sameOriginURLForPath(origin: string, pathname: string, search = '', hash = ''): string {
  return new URL(`${pathname}${search}${hash}`, origin).href;
}

function normalizeSameOriginUrl(value: string | undefined, origin: string): string | null {
  try {
    const url = new URL(value ?? NOTIFICATION_CLICK_FALLBACK_PATH, origin);
    return url.origin === origin ? url.href : null;
  } catch {
    return null;
  }
}

export function normalizeNotificationClickUrl(rawUrl: string | undefined, origin: string): string {
  const sameOriginUrl = normalizeSameOriginUrl(rawUrl, origin);
  if (sameOriginUrl) return sameOriginUrl;

  if (typeof rawUrl === 'string') {
    try {
      const parsed = new URL(rawUrl);
      if (parsed.pathname === '/chat' || parsed.pathname.startsWith('/chat/')) {
        return sameOriginURLForPath(origin, parsed.pathname, parsed.search, parsed.hash);
      }
    } catch {
      // Fall back to the safe same-origin chat entry point below.
    }
  }

  return sameOriginURLForPath(origin, NOTIFICATION_CLICK_FALLBACK_PATH);
}

/** Send the click message to `client`. Returns false when the window cannot receive it. */
function postClickMessage(client: NotificationClickClient, url: string): boolean {
  if (typeof client.postMessage !== 'function') return false;
  try {
    client.postMessage({ type: NOTIFICATION_CLICK_MESSAGE_TYPE, url });
    return true;
  } catch {
    return false;
  }
}

async function focusClient(
  client: NotificationClickClient,
  logger?: NotificationClickLogger
): Promise<boolean> {
  if (typeof client.focus !== 'function') return false;
  try {
    return (await client.focus()) !== null;
  } catch (err) {
    logger?.warn('[SW] Failed to focus existing window:', err);
    return false;
  }
}

/** Focused windows first, then visible ones, so the click reaches the window the user sees. */
function clientPriority(client: NotificationClickClient): number {
  if (client.focused) return 0;
  if (client.visibilityState === 'visible') return 1;
  return 2;
}

/**
 * Route a notification click to `rawUrl` after normalizing it to this origin.
 *
 * The worker focuses the best open window and sends it the target URL, and the
 * page routes in place. When no window is open, or focus fails, the worker
 * opens the target in a new window.
 *
 * The worker that shows push notifications has a narrow `/__chatto/push/…/`
 * scope. It never controls the app windows, so `WindowClient.navigate()`
 * always fails there. Chromium allows one window action (`focus()` or
 * `openWindow()`) per click, so the worker focuses before it sends the message
 * and does not wait for a reply.
 */
export async function routeNotificationClick(
  rawUrl: string | undefined,
  origin: string,
  clients: NotificationClickClients,
  options: NotificationClickRouteOptions = {}
): Promise<NotificationClickRouteResult> {
  const url = normalizeNotificationClickUrl(rawUrl, origin);
  const [target] = [
    ...(await clients.matchAll({
      type: 'window',
      includeUncontrolled: true
    }))
  ].sort((a, b) => clientPriority(a) - clientPriority(b));

  if (target && (await focusClient(target, options.logger))) {
    if (!postClickMessage(target, url)) {
      options.logger?.warn('[SW] Failed to send notification click to focused window');
    }
    return 'client';
  }

  await clients.openWindow(url);
  return 'open';
}
