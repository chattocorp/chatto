export const NOTIFICATION_CLICK_ACK_TIMEOUT_MS = 750;
export const NOTIFICATION_CLICK_MESSAGE_TYPE = 'notification-click';
export const NOTIFICATION_CLICK_ACK_MESSAGE_TYPE = 'notification-click-ack';
const NOTIFICATION_CLICK_FALLBACK_PATH = '/chat';

interface NotificationClickPort {
  onmessage: ((event: MessageEvent) => void) | null;
  close?: () => void;
}

interface NotificationClickMessageChannel {
  port1: NotificationClickPort;
  port2: unknown;
}

export interface NotificationClickClient {
  focused?: boolean;
  visibilityState?: DocumentVisibilityState;
  focus?: () => Promise<NotificationClickClient | null>;
  postMessage?: (message: unknown, transfer?: unknown[]) => void;
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

/**
 * How a click was routed: an open window acknowledged and routed it
 * (`client`), a hidden window was focused and asked to route it without an
 * acknowledgement (`focus`), or a new window opened the target (`open`).
 */
export type NotificationClickRouteResult = 'client' | 'focus' | 'open';

export interface NotificationClickRouteOptions {
  /** Total time that all visible windows together have to acknowledge. */
  ackTimeoutMs?: number;
  createMessageChannel?: () => NotificationClickMessageChannel;
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

function createDefaultMessageChannel(): NotificationClickMessageChannel {
  return new MessageChannel();
}

function isNotificationClickAck(message: unknown): boolean {
  return (
    typeof message === 'object' &&
    message !== null &&
    'type' in message &&
    message.type === NOTIFICATION_CLICK_ACK_MESSAGE_TYPE
  );
}

/** Send the click message to `client`. Returns false when the window cannot receive it. */
function postClickMessage(
  client: NotificationClickClient,
  url: string,
  transfer: unknown[] = []
): boolean {
  if (typeof client.postMessage !== 'function') return false;
  try {
    client.postMessage({ type: NOTIFICATION_CLICK_MESSAGE_TYPE, url }, transfer);
    return true;
  } catch {
    return false;
  }
}

function notifyClientAndWaitForAck(
  client: NotificationClickClient,
  url: string,
  timeoutMs: number,
  createMessageChannel: () => NotificationClickMessageChannel
): Promise<boolean> {
  return new Promise((resolve) => {
    const channel = createMessageChannel();
    let settled = false;
    const timeout = setTimeout(() => finish(false), timeoutMs);

    function finish(acknowledged: boolean) {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      channel.port1.onmessage = null;
      channel.port1.close?.();
      resolve(acknowledged);
    }

    channel.port1.onmessage = (event) => {
      if (isNotificationClickAck(event.data)) finish(true);
    };

    if (!postClickMessage(client, url, [channel.port2])) finish(false);
  });
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
 * Browsers allow window actions only briefly after a click. In Chromium, the
 * first `focus()` or `openWindow()` call also uses up the click's permission,
 * so the click allows one window action. Firefox allows window actions for
 * about one second after the click on desktop, and WebKit for about two.
 *
 * The worker that shows push notifications has a narrow `/__chatto/push/…/`
 * scope. It never controls the app windows, so `WindowClient.navigate()`
 * always fails there.
 *
 * When all windows are hidden, the worker focuses the first one at once and
 * then sends it the message. It does not wait for an acknowledgement: a
 * frozen background page cannot acknowledge, but it gets the message when the
 * focus resumes it. Otherwise, the worker asks the windows, in priority
 * order, to route in place. The page acknowledges on receipt, and all windows
 * share one acknowledgement deadline. The worker focuses the window that
 * acknowledges. In all other cases, the worker opens the target in a new
 * window.
 */
export async function routeNotificationClick(
  rawUrl: string | undefined,
  origin: string,
  clients: NotificationClickClients,
  options: NotificationClickRouteOptions = {}
): Promise<NotificationClickRouteResult> {
  const url = normalizeNotificationClickUrl(rawUrl, origin);
  const createMessageChannel = options.createMessageChannel ?? createDefaultMessageChannel;
  const deadline = performance.now() + (options.ackTimeoutMs ?? NOTIFICATION_CLICK_ACK_TIMEOUT_MS);
  const clientList = [
    ...(await clients.matchAll({
      type: 'window',
      includeUncontrolled: true
    }))
  ].sort((a, b) => clientPriority(a) - clientPriority(b));

  const [first] = clientList;
  if (first?.visibilityState === 'hidden') {
    if (await focusClient(first, options.logger)) {
      postClickMessage(first, url);
      return 'focus';
    }
  } else {
    for (const client of clientList) {
      const remainingMs = deadline - performance.now();
      if (remainingMs <= 0) break;
      if (await notifyClientAndWaitForAck(client, url, remainingMs, createMessageChannel)) {
        await focusClient(client, options.logger);
        return 'client';
      }
    }
  }

  await clients.openWindow(url);
  return 'open';
}
