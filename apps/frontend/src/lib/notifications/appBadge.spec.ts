import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  APP_BADGE_REFRESH_MESSAGE_TYPE,
  listenForAppBadgeRefresh,
  updateAppBadge
} from './appBadge';

describe('updateAppBadge', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sets and clears the app badge with the notification count', async () => {
    const setAppBadge = vi.fn(async () => {});
    vi.stubGlobal('navigator', { setAppBadge });

    await updateAppBadge(3);
    await updateAppBadge(0);

    expect(setAppBadge).toHaveBeenNthCalledWith(1, 3);
    expect(setAppBadge).toHaveBeenNthCalledWith(2, 0);
  });

  it('silently ignores badge failures', async () => {
    vi.stubGlobal('navigator', {
      setAppBadge: vi.fn(async () => {
        throw new Error('badging unavailable');
      })
    });

    await expect(updateAppBadge(1)).resolves.toBeUndefined();
  });
});

describe('listenForAppBadgeRefresh', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('listens only for app badge refresh messages and cleans up', () => {
    const serviceWorker = new EventTarget();
    vi.stubGlobal('navigator', { serviceWorker });
    const refresh = vi.fn();
    const cleanup = listenForAppBadgeRefresh(refresh);

    serviceWorker.dispatchEvent(new MessageEvent('message', { data: { type: 'unrelated' } }));
    serviceWorker.dispatchEvent(
      new MessageEvent('message', { data: { type: APP_BADGE_REFRESH_MESSAGE_TYPE } })
    );
    cleanup();
    serviceWorker.dispatchEvent(
      new MessageEvent('message', { data: { type: APP_BADGE_REFRESH_MESSAGE_TYPE } })
    );

    expect(refresh).toHaveBeenCalledOnce();
  });
});
