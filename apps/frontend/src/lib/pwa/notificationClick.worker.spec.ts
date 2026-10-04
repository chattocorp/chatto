import { afterEach, describe, expect, it, vi } from 'vitest';
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

  afterEach(() => {
    vi.useRealTimers();
  });

  function acknowledgingPostMessage() {
    return vi.fn((_message: unknown, transfer?: unknown[]) => {
      const ackPort = transfer?.[0] as { postMessage: (message: unknown) => void };
      ackPort.postMessage({ type: 'notification-click-ack' });
    });
  }

  it('focuses a window only after it acknowledges SPA routing', async () => {
    const channel = createAcknowledgingMessageChannel();
    const focus = vi.fn(async () => client);
    const postMessage = acknowledgingPostMessage();
    const client: NotificationClickClient = { visibilityState: 'visible', focus, postMessage };
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

  it('reports client routing when focusing the acknowledging window fails', async () => {
    const client: NotificationClickClient = {
      visibilityState: 'visible',
      focus: vi.fn(async () => {
        throw new Error('Not allowed to focus a window.');
      }),
      postMessage: acknowledgingPostMessage()
    };
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      createMessageChannel: createAcknowledgingMessageChannel,
      logger: { warn: vi.fn() }
    });

    expect(result).toBe('client');
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('keeps the window action for openWindow when a visible window does not acknowledge', async () => {
    vi.useFakeTimers();
    const focus = vi.fn(async () => client);
    const postMessage = vi.fn();
    const client: NotificationClickClient = { visibilityState: 'visible', focus, postMessage };
    const clients = clientsWith([client]);

    const routed = routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 750,
      createMessageChannel: createAcknowledgingMessageChannel
    });
    await vi.advanceTimersByTimeAsync(750);
    const result = await routed;

    expect(result).toBe('open');
    expect(postMessage).toHaveBeenCalledOnce();
    expect(focus).not.toHaveBeenCalled();
    expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
  });

  it('focuses a hidden window that does not acknowledge so it routes when it resumes', async () => {
    vi.useFakeTimers();
    const focus = vi.fn(async () => client);
    const postMessage = vi.fn();
    const client: NotificationClickClient = { visibilityState: 'hidden', focus, postMessage };
    const clients = clientsWith([client]);

    const routed = routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 750,
      createMessageChannel: createAcknowledgingMessageChannel
    });
    await vi.advanceTimersByTimeAsync(750);
    const result = await routed;

    expect(result).toBe('focus');
    expect(postMessage).toHaveBeenCalledOnce();
    expect(focus).toHaveBeenCalledOnce();
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('opens a new window when the hidden window cannot be focused', async () => {
    vi.useFakeTimers();
    const client: NotificationClickClient = {
      visibilityState: 'hidden',
      focus: vi.fn(async () => null),
      postMessage: vi.fn()
    };
    const clients = clientsWith([client]);

    const routed = routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 750,
      createMessageChannel: createAcknowledgingMessageChannel
    });
    await vi.advanceTimersByTimeAsync(750);
    const result = await routed;

    expect(result).toBe('open');
    expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
  });

  it('shares one acknowledgement deadline across windows', async () => {
    vi.useFakeTimers();
    const first: NotificationClickClient = { visibilityState: 'visible', postMessage: vi.fn() };
    const second: NotificationClickClient = { visibilityState: 'visible', postMessage: vi.fn() };
    const clients = clientsWith([first, second]);

    const routed = routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 750,
      createMessageChannel: createAcknowledgingMessageChannel
    });
    await vi.advanceTimersByTimeAsync(750);

    await expect(routed).resolves.toBe('open');
    expect(first.postMessage).toHaveBeenCalledOnce();
    expect(second.postMessage).not.toHaveBeenCalled();
    expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
  });

  it('ignores an acknowledgement that arrives after the deadline', async () => {
    vi.useFakeTimers();
    let ackPort: { postMessage: (message: unknown) => void } | undefined;
    const client: NotificationClickClient = {
      visibilityState: 'visible',
      focus: vi.fn(async () => client),
      postMessage: vi.fn((_message: unknown, transfer?: unknown[]) => {
        ackPort = transfer?.[0] as typeof ackPort;
      })
    };
    const clients = clientsWith([client]);

    const routed = routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      ackTimeoutMs: 750,
      createMessageChannel: createAcknowledgingMessageChannel
    });
    await vi.advanceTimersByTimeAsync(750);
    ackPort?.postMessage({ type: 'notification-click-ack' });

    await expect(routed).resolves.toBe('open');
    expect(client.focus).not.toHaveBeenCalled();
  });

  it('tries the next window when posting to a window throws', async () => {
    const broken: NotificationClickClient = {
      focused: true,
      visibilityState: 'visible',
      postMessage: vi.fn(() => {
        throw new Error('client gone');
      })
    };
    const working: NotificationClickClient = {
      visibilityState: 'visible',
      focus: vi.fn(async () => working),
      postMessage: acknowledgingPostMessage()
    };
    const clients = clientsWith([working, broken]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      createMessageChannel: createAcknowledgingMessageChannel
    });

    expect(result).toBe('client');
    expect(broken.postMessage).toHaveBeenCalledOnce();
    expect(working.focus).toHaveBeenCalledOnce();
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
          if (!acknowledges) throw new Error('window cannot route');
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
