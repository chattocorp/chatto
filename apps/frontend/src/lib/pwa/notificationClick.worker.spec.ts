import { afterEach, describe, expect, it, vi } from 'vitest';
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
  it('normalizes click targets', () => {
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
    vi.restoreAllMocks();
  });

  function windowClient(
    state: Partial<Pick<NotificationClickClient, 'url' | 'focused' | 'visibilityState'>>
  ) {
    const client = {
      url: `${ORIGIN}/chat/-/room-2`,
      focused: false,
      visibilityState: 'visible' as DocumentVisibilityState,
      ...state,
      focus: vi.fn(async () => client),
      postMessage: vi.fn()
    };
    return client;
  }

  it('focuses the open window, then sends it the click', async () => {
    const client = windowClient({ focused: true });
    const clients = clientsWith([client]);

    await routeNotificationClick(TARGET_URL, ORIGIN, clients);

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

    const clients = clientsWith([hidden]);
    await routeNotificationClick(TARGET_URL, ORIGIN, clients);
    expect(hidden.focus).toHaveBeenCalledOnce();
    expect(hidden.postMessage).toHaveBeenCalledOnce();
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('ignores windows that do not run the app', async () => {
    const attachment = windowClient({ url: `${ORIGIN}/assets/files/asset-1`, focused: true });
    const loadedAtRoot = windowClient({ url: `${ORIGIN}/`, visibilityState: 'hidden' });

    await routeNotificationClick(TARGET_URL, ORIGIN, clientsWith([attachment, loadedAtRoot]));
    expect(attachment.focus).not.toHaveBeenCalled();
    expect(loadedAtRoot.postMessage).toHaveBeenCalledOnce();

    const clients = clientsWith([attachment]);
    await routeNotificationClick(TARGET_URL, ORIGIN, clients);
    expect(attachment.focus).not.toHaveBeenCalled();
    expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
  });

  it('opens a new window instead when focusing fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const client = windowClient({});
    client.focus.mockRejectedValue(new Error('Not allowed to focus a window.'));
    const clients = clientsWith([client]);

    await routeNotificationClick(TARGET_URL, ORIGIN, clients);

    expect(warn).toHaveBeenCalledOnce();
    expect(client.postMessage).not.toHaveBeenCalled();
    expect(clients.openWindow).toHaveBeenCalledWith(TARGET_URL);
  });

  it('does not open a second window when sending to the focused window fails', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const client = windowClient({});
    client.postMessage.mockImplementation(() => {
      throw new Error('client gone');
    });
    const clients = clientsWith([client]);

    await routeNotificationClick(TARGET_URL, ORIGIN, clients);

    expect(warn).toHaveBeenCalledOnce();
    expect(clients.openWindow).not.toHaveBeenCalled();
  });

  it('opens a new window when no window client exists', async () => {
    const clients = clientsWith([]);

    await routeNotificationClick(TARGET_URL, ORIGIN, clients);

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

      await routeNotificationClick(url, ORIGIN, clients);
      expect(clients.openWindow).toHaveBeenCalledWith(url);
    }
  });

  it('maps cross-origin chat payload URLs onto the service worker origin', async () => {
    const clients = clientsWith([]);

    await routeNotificationClick(
      'https://configured.example/chat/-/room-1?highlight=event-1#message',
      ORIGIN,
      clients
    );

    expect(clients.openWindow).toHaveBeenCalledWith(
      `${ORIGIN}/chat/-/room-1?highlight=event-1#message`
    );
  });

  it('falls back to the chat entry point for missing, malformed, or non-chat cross-origin URLs', async () => {
    for (const rawUrl of [undefined, 'http://[', 'https://other.example/settings']) {
      const clients = clientsWith([]);

      await routeNotificationClick(rawUrl, ORIGIN, clients);
      expect(clients.openWindow).toHaveBeenCalledWith(`${ORIGIN}/chat`);
    }
  });
});
