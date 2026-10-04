import { describe, expect, it, vi } from 'vitest';
import {
  normalizeNotificationClickUrl,
  routeNotificationClick,
  type NotificationClickClient
} from './notificationClick.worker';

const ORIGIN = 'https://chatto.example';
const TARGET_URL = `${ORIGIN}/chat/-/room-1?highlight=event-1`;

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

  function windowClient(
    state: Pick<NotificationClickClient, 'url' | 'focused' | 'visibilityState'>
  ): NotificationClickClient & {
    focus: ReturnType<typeof vi.fn>;
    postMessage: ReturnType<typeof vi.fn>;
  } {
    const client = {
      url: `${ORIGIN}/chat/-/room-2`,
      ...state,
      focus: vi.fn(async () => client),
      postMessage: vi.fn()
    };
    return client;
  }

  it('focuses the open window, then sends it the click', async () => {
    const client = windowClient({ focused: true, visibilityState: 'visible' });
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients);

    expect(result).toBe('client');
    expect(client.focus).toHaveBeenCalledOnce();
    expect(client.postMessage).toHaveBeenCalledWith({
      type: 'notification-click',
      url: TARGET_URL
    });
    expect(client.focus.mock.invocationCallOrder[0]).toBeLessThan(
      client.postMessage.mock.invocationCallOrder[0]
    );
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('chooses a focused window, then a visible one, then a hidden one', async () => {
    const hidden = windowClient({ visibilityState: 'hidden' });
    const visible = windowClient({ visibilityState: 'visible' });
    const focused = windowClient({ focused: true, visibilityState: 'visible' });

    await routeNotificationClick(TARGET_URL, ORIGIN, clientsWith([hidden, visible, focused]));
    expect(focused.postMessage).toHaveBeenCalledOnce();
    expect(visible.postMessage).not.toHaveBeenCalled();
    expect(hidden.postMessage).not.toHaveBeenCalled();

    await routeNotificationClick(TARGET_URL, ORIGIN, clientsWith([hidden, visible]));
    expect(visible.postMessage).toHaveBeenCalledOnce();
    expect(hidden.postMessage).not.toHaveBeenCalled();

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clientsWith([hidden]));
    expect(result).toBe('client');
    expect(hidden.focus).toHaveBeenCalledOnce();
    expect(hidden.postMessage).toHaveBeenCalledOnce();
  });

  it('ignores windows outside the chat app', async () => {
    const oauth = windowClient({
      url: `${ORIGIN}/servers/authorize?mode=popup`,
      focused: true,
      visibilityState: 'visible'
    });
    const attachment = windowClient({
      url: `${ORIGIN}/assets/file.png`,
      visibilityState: 'visible'
    });
    const chat = windowClient({ url: `${ORIGIN}/chat`, visibilityState: 'hidden' });

    await expect(
      routeNotificationClick(TARGET_URL, ORIGIN, clientsWith([oauth, attachment, chat]))
    ).resolves.toBe('client');
    expect(chat.postMessage).toHaveBeenCalledOnce();

    const clients = clientsWith([oauth, attachment]);
    await expect(routeNotificationClick(TARGET_URL, ORIGIN, clients)).resolves.toBe('open');
    expect(oauth.focus).not.toHaveBeenCalled();
    expect(attachment.focus).not.toHaveBeenCalled();
    expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
  });

  it('opens a new window instead when focusing fails', async () => {
    const warn = vi.fn();
    const rejecting = windowClient({ visibilityState: 'visible' });
    rejecting.focus.mockRejectedValue(new Error('Not allowed to focus a window.'));
    const unfocusable = windowClient({ visibilityState: 'hidden' });
    unfocusable.focus.mockResolvedValue(null);

    for (const client of [rejecting, unfocusable]) {
      const clients = clientsWith([client]);
      const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
        logger: { warn }
      });

      expect(result).toBe('open');
      expect(client.postMessage).not.toHaveBeenCalled();
      expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
    }
    expect(warn).toHaveBeenCalledOnce();
  });

  it('does not open a second window when sending to the focused window fails', async () => {
    const warn = vi.fn();
    const client = windowClient({ visibilityState: 'visible' });
    client.postMessage.mockImplementation(() => {
      throw new Error('client gone');
    });
    const clients = clientsWith([client]);

    const result = await routeNotificationClick(TARGET_URL, ORIGIN, clients, {
      logger: { warn }
    });

    expect(result).toBe('client');
    expect(warn).toHaveBeenCalledOnce();
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
