import { describe, expect, it, vi } from 'vitest';
import { routeNotificationClick, type NotificationClickClient } from './notificationClick.worker';

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
  it('uses acknowledged SPA routing without navigating the window', async () => {
    const channel = createAcknowledgingMessageChannel();
    const client: NotificationClickClient = {
      focus: vi.fn(async () => client),
      navigate: vi.fn(async () => client),
      postMessage: vi.fn((_message, transfer) => {
        const ackPort = transfer?.[0] as { postMessage: (message: unknown) => void };
        ackPort.postMessage({ type: 'notification-click-ack' });
      })
    };
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      createMessageChannel: () => channel
    });

    expect(result).toBe('client');
    expect(client.focus).toHaveBeenCalledOnce();
    expect(client.postMessage).toHaveBeenCalledWith(
      { type: 'notification-click', url: TARGET_URL },
      [channel.port2]
    );
    expect(client.navigate).not.toHaveBeenCalled();
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('falls back to WindowClient.navigate when the SPA does not acknowledge', async () => {
    const client: NotificationClickClient = {
      focus: vi.fn(async () => client),
      navigate: vi.fn(async () => client),
      postMessage: vi.fn()
    };
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 1,
      createMessageChannel: createAcknowledgingMessageChannel
    });

    expect(result).toBe('navigate');
    expect(client.postMessage).toHaveBeenCalledOnce();
    expect(client.navigate).toHaveBeenCalledWith(TARGET_URL);
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

  it('rejects malformed and cross-origin notification URLs', async () => {
    const clients = clientsWith([]);

    await expect(routeNotificationClick('http://[', ORIGIN, clients)).resolves.toBe('ignored');
    await expect(
      routeNotificationClick('https://other.example/chat', ORIGIN, clients)
    ).resolves.toBe('ignored');

    expect(clients.matchAll).not.toHaveBeenCalled();
    expect(clients.openWindow).not.toHaveBeenCalled();
  });
});
