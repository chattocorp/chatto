import { describe, expect, it, vi } from 'vitest';
import type { ServerStateStore } from '@chatto/client/server/store';
import type { NotificationOccurrenceItem } from '@chatto/client/api/notifications';
import { navigateNotification } from './notificationNavigationUi';

function fixture() {
  let endSession = () => {};
  let reset = (_event: { privacy: boolean }) => {};
  const unsubscribe = vi.fn();
  const markOccurrenceRead = vi.fn().mockResolvedValue(undefined);
  const store = {
    accountId: 'viewer',
    isAuthenticated: true,
    onSessionEnded: (callback: () => void) => {
      endSession = callback;
      return unsubscribe;
    },
    onReset: (callback: typeof reset) => {
      reset = callback;
      return unsubscribe;
    },
    notifications: { markOccurrenceRead }
  } as unknown as ServerStateStore;
  const occurrence = {
    id: 'invite',
    room: { id: 'room', name: 'Room' },
    eventId: '',
    unread: true
  } as NotificationOccurrenceItem;
  return {
    store,
    occurrence,
    markOccurrenceRead,
    unsubscribe,
    endSession: () => endSession(),
    reset: () => reset({ privacy: true })
  };
}

describe('room-only notification navigation', () => {
  it('reads only after navigation succeeds and releases boundary subscriptions', async () => {
    const f = fixture();
    let finish!: () => void;
    const pending = navigateNotification(
      f.store,
      f.occurrence,
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    expect(f.markOccurrenceRead).not.toHaveBeenCalled();
    finish();
    await pending;
    expect(f.markOccurrenceRead).toHaveBeenCalledExactlyOnceWith('invite');
    expect(f.unsubscribe).toHaveBeenCalledTimes(2);
  });

  it('does not read after failed navigation', async () => {
    const f = fixture();
    await expect(
      navigateNotification(f.store, f.occurrence, async () => {
        throw new Error('Navigation failed');
      })
    ).rejects.toThrow('Navigation failed');
    expect(f.markOccurrenceRead).not.toHaveBeenCalled();
    expect(f.unsubscribe).toHaveBeenCalledTimes(2);
  });

  it.each(['endSession', 'reset'] as const)(
    'does not read after %s during navigation',
    async (boundary) => {
      const f = fixture();
      await navigateNotification(f.store, f.occurrence, async () => {
        f[boundary]();
      });
      expect(f.markOccurrenceRead).not.toHaveBeenCalled();
    }
  );

  it('leaves message notifications to timeline highlight confirmation', async () => {
    const f = fixture();
    await navigateNotification(f.store, { ...f.occurrence, eventId: 'message' }, async () => {});
    expect(f.markOccurrenceRead).not.toHaveBeenCalled();
    expect(f.unsubscribe).not.toHaveBeenCalled();
  });
});
