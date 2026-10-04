import { describe, expect, it, vi } from 'vitest';
import {
  normalizeNotificationClickUrl,
  routeNotificationClick,
  type NotificationClickClient
} from './notificationClick.worker';

const ORIGIN = 'https://chatto.example';
const TARGET_URL = `${ORIGIN}/chat/-/room-1?highlight=event-1`;

function createAcknowledgingMessageChannel() {
  const port1 = {
    onmessage: null as ((event: MessageEvent) => void) | null,
    close: vi.fn()
  };
  const port2 = {
    postMessage: vi.fn((data: unknown) => {
      port1.onmessage?.({ data } as MessageEvent);
    })
  };

  return { port1, port2 };
}

function clientsWith(matches: NotificationClickClient[]) {
  return {
    matchAll: vi.fn(async () => matches),
    openWindow: vi.fn(async () => null)
  };
}

describe('routeNotificationClick', () => {
  it('normalizes click targets before activation attempts', () => {
    expect(
      normalizeNotificationClickUrl(
        'https://configured.example/chat/-/room-1?highlight=event-1#message',
        ORIGIN
      )
    ).toBe(`${ORIGIN}/chat/-/room-1?highlight=event-1#message`);
    expect(normalizeNotificationClickUrl('http://[', ORIGIN)).toBe(`${ORIGIN}/chat`);
    expect(normalizeNotificationClickUrl('https://other.example/settings', ORIGIN)).toBe(
      `${ORIGIN}/chat`
    );
  });

  it('focuses a window only after it acknowledges SPA routing', async () => {
    const channel = createAcknowledgingMessageChannel();
    const focus = vi.fn(async () => client);
    const postMessage = vi.fn((_message, transfer) => {
      const ackPort = transfer?.[0] as { postMessage: (message: unknown) => void };
      ackPort.postMessage({ type: 'notification-click-ack' });
    });
    const client: NotificationClickClient = { focus, postMessage };
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      createMessageChannel: () => channel
    });

    expect(result).toBe('client');
    expect(postMessage).toHaveBeenCalledWith({ type: 'notification-click', url: TARGET_URL }, [
      channel.port2
    ]);
    expect(focus).toHaveBeenCalledOnce();
    expect(postMessage.mock.invocationCallOrder[0]).toBeLessThan(focus.mock.invocationCallOrder[0]);
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('keeps the single window action for openWindow when no window acknowledges', async () => {
    const focus = vi.fn(async () => client);
    const postMessage = vi.fn();
    const client: NotificationClickClient = { focus, postMessage };
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 1,
      createMessageChannel: createAcknowledgingMessageChannel
    });

    expect(result).toBe('open');
    expect(postMessage).toHaveBeenCalledOnce();
    expect(focus).not.toHaveBeenCalled();
    expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
  });

  it('asks focused, then visible, then hidden windows and stops at the first acknowledgement', async () => {
    const order: string[] = [];
    function windowClient(
      name: string,
      state: Pick<NotificationClickClient, 'focused' | 'visibilityState'>,
      acknowledges: boolean
    ): NotificationClickClient {
      const client: NotificationClickClient = {
        ...state,
        focus: vi.fn(async () => client),
        postMessage: vi.fn((_message, transfer) => {
          order.push(name);
          if (!acknowledges) return;
          const ackPort = transfer?.[0] as { postMessage: (message: unknown) => void };
          ackPort.postMessage({ type: 'notification-click-ack' });
        })
      };
      return client;
    }
    const hidden = windowClient('hidden', { visibilityState: 'hidden' }, true);
    const visible = windowClient('visible', { visibilityState: 'visible' }, true);
    const focused = windowClient('focused', { focused: true, visibilityState: 'visible' }, false);
    const clients = clientsWith([hidden, visible, focused]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 1,
      createMessageChannel: createAcknowledgingMessageChannel
    });

    expect(result).toBe('client');
    expect(order).toEqual(['focused', 'visible']);
    expect(focused.focus).not.toHaveBeenCalled();
    expect(visible.focus).toHaveBeenCalledOnce();
    expect(hidden.postMessage).not.toHaveBeenCalled();
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('opens a new window when no window client exists', async () => {
    const clients = clientsWith([]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients);

    expect(result).toBe('open');
    expect(clients.matchAll).toHaveBeenCalledWith({
      type: 'window',
      includeUncontrolled: true
    });
    expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
  });

  it('accepts room, thread, and DM notification route URLs', async () => {
    const routes = [
      `${ORIGIN}/chat/-/dm-room`,
      `${ORIGIN}/chat/-/room-1?highlight=event-1`,
      `${ORIGIN}/chat/-/room-1/thread-root?highlight=reply-event`
    ];

    for (const url of routes) {
      const clients = clientsWith([]);

      await expect(routeNotificationClick(url, ORIGIN, clients)).resolves.toBe('open');
      expect(clients.openWindow).toHaveBeenCalledWith(url);
    }
  });

  it('maps cross-origin chat payload URLs onto the service worker origin', async () => {
    const clients = clientsWith([]);

    await expect(
      routeNotificationClick(
        'https://configured.example/chat/-/room-1?highlight=event-1#message',
        ORIGIN,
        clients
      )
    ).resolves.toBe('open');

    expect(clients.openWindow).toHaveBeenCalledWith(
      `${ORIGIN}/chat/-/room-1?highlight=event-1#message`
    );
  });

  it('falls back to the chat entry point for malformed or non-chat cross-origin URLs', async () => {
    const malformedClients = clientsWith([]);
    const crossOriginClients = clientsWith([]);

    await expect(routeNotificationClick('http://[', ORIGIN, malformedClients)).resolves.toBe(
      'open'
    );
    await expect(
      routeNotificationClick('https://other.example/settings', ORIGIN, crossOriginClients)
    ).resolves.toBe('open');

    expect(malformedClients.openWindow).toHaveBeenCalledWith(`${ORIGIN}/chat`);
    expect(crossOriginClients.openWindow).toHaveBeenCalledWith(`${ORIGIN}/chat`);
  });
});
