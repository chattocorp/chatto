import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import PushNotificationSync from './PushNotificationSync.svelte';
import { NotificationStore } from '$lib/state/server/notifications.svelte';
import type { NotificationAPI, NotificationOccurrencePage } from '$lib/api-client/notifications';

const { refreshHandlers } = vi.hoisted(() => ({ refreshHandlers: new Set<() => void>() }));
vi.mock('$lib/notifications/appBadge', () => ({
  listenForAppBadgeRefresh: (handler: () => void) => {
    refreshHandlers.add(handler);
    return () => refreshHandlers.delete(handler);
  }
}));
const emptyPage: NotificationOccurrencePage = {
  occurrences: [],
  consumedCount: 0,
  totalCount: 0,
  hasMore: false,
  unreadCount: 0,
  importantUnreadCount: 0,
  roomUnreadCounts: {},
  roomImportantUnreadCounts: {}
};
function setup() {
  const close = vi.fn();
  const item = {
    data: {
      notificationId: 'handled',
      serverOrigin: 'https://chat.example.com',
      recipientId: 'user'
    },
    close
  };
  const getNotifications = vi.fn(async () => [item]);
  vi.spyOn(navigator.serviceWorker, 'getRegistrations').mockResolvedValue([
    { getNotifications } as unknown as ServiceWorkerRegistration
  ]);
  const list = vi.fn(async (): Promise<NotificationOccurrencePage> => emptyPage);
  const notifications = new NotificationStore({
    listNotificationOccurrences: list
  } as unknown as NotificationAPI);
  const view = render(PushNotificationSync, {
    serverUrl: 'https://chat.example.com',
    recipientId: 'user',
    notifications
  });
  return { close, list, notifications, view, getNotifications };
}
beforeEach(() => refreshHandlers.clear());
afterEach(() => vi.restoreAllMocks());

describe('PushNotificationSync', () => {
  it('reconciles on mount, authoritative updates, focus, and worker refresh', async () => {
    const { close, notifications } = setup();
    await expect.poll(() => close.mock.calls.length).toBe(1);
    flushSync(() => notifications.replaceOccurrenceProjection(emptyPage));
    await expect.poll(() => close.mock.calls.length).toBe(2);
    window.dispatchEvent(new Event('focus'));
    await expect.poll(() => close.mock.calls.length).toBe(3);
    for (const refresh of refreshHandlers) refresh();
    await expect.poll(() => close.mock.calls.length).toBe(4);
  });

  it('cancels an in-flight read when the account component unmounts', async () => {
    const { close, list, view } = setup();
    let resolve!: (page: NotificationOccurrencePage) => void;
    list.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    await expect.poll(() => list.mock.calls.length).toBe(1);
    await view.unmount();
    resolve(emptyPage);
    await new Promise((done) => setTimeout(done, 0));
    expect(close).not.toHaveBeenCalled();
    expect(refreshHandlers.size).toBe(0);
  });

  it('serializes refreshes and retries after state changes during a read', async () => {
    const { close, list, notifications } = setup();
    let resolve!: (page: NotificationOccurrencePage) => void;
    list.mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    await expect.poll(() => list.mock.calls.length).toBe(1);
    flushSync(() => notifications.replaceOccurrenceProjection(emptyPage));
    window.dispatchEvent(new Event('focus'));
    expect(list).toHaveBeenCalledTimes(1);
    resolve(emptyPage);
    await expect.poll(() => close.mock.calls.length).toBe(1);
    expect(list).toHaveBeenCalledTimes(2);
  });
});
