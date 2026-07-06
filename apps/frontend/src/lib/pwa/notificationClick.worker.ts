import { normalizeSameOriginUrl } from './serviceWorkerPolicy';

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
  focus?: () => Promise<NotificationClickClient | null>;
  navigate?: (url: string) => Promise<NotificationClickClient | null>;
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

export type NotificationClickRouteResult = 'client' | 'navigate' | 'open';

export interface NotificationClickRouteOptions {
  ackTimeoutMs?: number;
  createMessageChannel?: () => NotificationClickMessageChannel;
  logger?: NotificationClickLogger;
}

function sameOriginURLForPath(origin: string, pathname: string, search = '', hash = ''): string {
  return new URL(`${pathname}${search}${hash}`, origin).href;
}

function normalizeNotificationClickUrl(rawUrl: string | undefined, origin: string): string {
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

function notifyClientAndWaitForAck(
  client: NotificationClickClient,
  url: string,
  options: Required<Pick<NotificationClickRouteOptions, 'ackTimeoutMs' | 'createMessageChannel'>>
): Promise<boolean> {
  if (typeof client.postMessage !== 'function') return Promise.resolve(false);
  const postMessage = client.postMessage;

  return new Promise((resolve) => {
    const channel = options.createMessageChannel();
    let settled = false;
    const timeout = setTimeout(() => finish(false), options.ackTimeoutMs);

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

    try {
      postMessage.call(client, { type: NOTIFICATION_CLICK_MESSAGE_TYPE, url }, [channel.port2]);
    } catch {
      finish(false);
    }
  });
}

async function focusClient(
  client: NotificationClickClient,
  logger?: NotificationClickLogger
): Promise<NotificationClickClient | null> {
  if (typeof client.focus !== 'function') return null;
  try {
    return await client.focus();
  } catch (err) {
    logger?.warn('[SW] Failed to focus existing window:', err);
    return null;
  }
}

export async function routeNotificationClick(
  rawUrl: string | undefined,
  origin: string,
  clients: NotificationClickClients,
  options: NotificationClickRouteOptions = {}
): Promise<NotificationClickRouteResult> {
  const url = normalizeNotificationClickUrl(rawUrl, origin);

  const ackOptions = {
    ackTimeoutMs: options.ackTimeoutMs ?? NOTIFICATION_CLICK_ACK_TIMEOUT_MS,
    createMessageChannel: options.createMessageChannel ?? createDefaultMessageChannel
  };
  const clientList = await clients.matchAll({
    type: 'window',
    includeUncontrolled: true
  });

  for (const client of clientList) {
    const initiallyFocusedClient = await focusClient(client, options.logger);
    const focusedClient = initiallyFocusedClient ?? client;
    const acknowledged = await notifyClientAndWaitForAck(focusedClient, url, ackOptions);
    if (acknowledged) {
      return 'client';
    }

    try {
      const navigatedClient = await focusedClient.navigate?.(url);
      if (navigatedClient) {
        if (!initiallyFocusedClient) await focusClient(navigatedClient, options.logger);
        return 'navigate';
      }
    } catch (err) {
      options.logger?.warn('[SW] Failed to navigate existing window:', err);
    }
  }

  await clients.openWindow(url);
  return 'open';
}
