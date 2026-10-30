import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  desktopNotifications,
  showDesktopNotification,
  clearDesktopNotificationOwner,
  listenForDesktopNotificationClicks
} from './notifications.svelte';

afterEach(() => {
  desktopNotifications.enabled = false;
  delete window.chattoDesktop;
});

describe('desktop notification bridge', () => {
  it('uses one global capacity and clears an owner without clearing another account', async () => {
    const clear = vi.fn();
    window.chattoDesktop = {
      notifications: { show: async () => {}, clear, onClick: () => () => {} }
    };
    const firstOwner = {};
    const secondOwner = {};
    for (let index = 0; index < 65; index++) {
      await showDesktopNotification(String(index), 'Notifications', () => {}, {
        owner: index < 64 ? firstOwner : secondOwner,
        occurrenceId: String(index)
      });
    }
    expect(clear).toHaveBeenCalledWith(['0']);
    clear.mockClear();
    clearDesktopNotificationOwner(firstOwner);
    expect(clear).toHaveBeenCalledOnce();
    expect(clear.mock.calls[0][0]).toHaveLength(63);
    expect(clear.mock.calls[0][0]).not.toContain('64');
    clearDesktopNotificationOwner(secondOwner);
    expect(clear).toHaveBeenLastCalledWith(['64']);
  });
  it('routes opaque IDs once and invalidates all targets when consent is removed', async () => {
    let clicked: ((id: string) => void) | undefined;
    const show = vi.fn(async () => {});
    const clear = vi.fn();
    const stop = vi.fn();
    window.chattoDesktop = {
      notifications: {
        show,
        clear,
        onClick: (callback) => {
          clicked = callback;
          return stop;
        }
      }
    };
    const dispose = listenForDesktopNotificationClicks();
    const first = vi.fn();
    const second = vi.fn();
    desktopNotifications.enabled = true;
    await showDesktopNotification('first', 'Notifications', first);
    clicked?.('first');
    clicked?.('first');
    expect(first).toHaveBeenCalledOnce();
    await showDesktopNotification('second', 'Notifications', second);
    desktopNotifications.enabled = false;
    clicked?.('second');
    expect(second).not.toHaveBeenCalled();
    expect(clear).toHaveBeenCalledWith(['second']);
    dispose();
    expect(stop).toHaveBeenCalledOnce();
  });

  it('removes failed notification targets', async () => {
    let clicked: ((id: string) => void) | undefined;
    window.chattoDesktop = {
      notifications: {
        show: async () => {
          throw new Error('denied');
        },
        clear: vi.fn(),
        onClick: (callback) => {
          clicked = callback;
          return () => {};
        }
      }
    };
    const dispose = listenForDesktopNotificationClicks();
    const callback = vi.fn();
    await expect(showDesktopNotification('failed', 'Notifications', callback)).rejects.toThrow(
      'denied'
    );
    clicked?.('failed');
    expect(callback).not.toHaveBeenCalled();
    dispose();
  });
});
