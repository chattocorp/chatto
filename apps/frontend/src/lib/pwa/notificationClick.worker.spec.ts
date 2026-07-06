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

  it('focuses the window before using acknowledged SPA routing', async () => {
    const channel = createAcknowledgingMessageChannel();
    const focus = vi.fn(async () => client);
    const navigate = vi.fn(async () => client);
    const postMessage = vi.fn((_message, transfer) => {
      const ackPort = transfer?.[0] as { postMessage: (message: unknown) => void };
      ackPort.postMessage({ type: 'notification-click-ack' });
    });
    const client: NotificationClickClient = {
      focus,
      navigate,
      postMessage
    };
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      createMessageChannel: () => channel
    });

    expect(result).toBe('client');
    expect(focus).toHaveBeenCalledOnce();
    expect(postMessage).toHaveBeenCalledWith({ type: 'notification-click', url: TARGET_URL }, [
      channel.port2
    ]);
    expect(focus.mock.invocationCallOrder[0]).toBeLessThan(postMessage.mock.invocationCallOrder[0]);
    expect(navigate).not.toHaveBeenCalled();
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('falls back to WindowClient.navigate before focusing when the SPA does not acknowledge', async () => {
    const focus = vi.fn(async () => client);
    const navigate = vi.fn(async () => client);
    const postMessage = vi.fn();
    const client: NotificationClickClient = {
      focus,
      navigate,
      postMessage
    };
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 1,
      createMessageChannel: createAcknowledgingMessageChannel
    });

    expect(result).toBe('navigate');
    expect(postMessage).toHaveBeenCalledOnce();
    expect(navigate).toHaveBeenCalledWith(TARGET_URL);
    expect(focus).toHaveBeenCalledOnce();
    expect(focus.mock.invocationCallOrder[0]).toBeLessThan(postMessage.mock.invocationCallOrder[0]);
    expect(focus.mock.invocationCallOrder[0]).toBeLessThan(navigate.mock.invocationCallOrder[0]);
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('tries later window clients when an earlier client cannot route or navigate', async () => {
    const staleClient: NotificationClickClient = {
      focus: vi.fn(async () => staleClient),
      navigate: vi.fn(async () => null),
      postMessage: vi.fn()
    };
    const activeClient: NotificationClickClient = {
      focus: vi.fn(async () => activeClient),
      navigate: vi.fn(async () => activeClient),
      postMessage: vi.fn((_message, transfer) => {
        const ackPort = transfer?.[0] as { postMessage: (message: unknown) => void };
        ackPort.postMessage({ type: 'notification-click-ack' });
      })
    };
    const clients = clientsWith([staleClient, activeClient]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 1,
      createMessageChannel: createAcknowledgingMessageChannel
    });

    expect(result).toBe('client');
    expect(staleClient.postMessage).toHaveBeenCalledOnce();
    expect(staleClient.navigate).toHaveBeenCalledWith(TARGET_URL);
    expect(staleClient.focus).toHaveBeenCalledOnce();
    expect(activeClient.postMessage).toHaveBeenCalledOnce();
    expect(activeClient.focus).toHaveBeenCalledOnce();
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
