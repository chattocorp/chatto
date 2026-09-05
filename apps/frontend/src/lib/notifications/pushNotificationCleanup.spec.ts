import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanupPushNotifications } from './pushNotificationCleanup';

const owner = { serverOrigin: 'https://chat.example.com', recipientId: 'user-1' };
function notification(notificationId: string, data: object = owner) {
  return { data: { notificationId, ...data }, close: vi.fn() };
}
function registrations(...groups: ReturnType<typeof notification>[][]) {
  vi.stubGlobal('navigator', {
    serviceWorker: {
      getRegistrations: vi.fn(async () =>
        groups.map((items) => ({
          getNotifications: vi.fn(async () => items)
        }))
      )
    }
  });
}
afterEach(() => vi.unstubAllGlobals());

describe('cleanupPushNotifications', () => {
  it('closes only handled IDs for the exact account across registrations', async () => {
    const handled = notification('handled');
    const unread = notification('unread');
    const otherUser = notification('handled', { ...owner, recipientId: 'user-2' });
    const otherServer = notification('handled', {
      ...owner,
      serverOrigin: 'https://other.example.com'
    });
    const legacy = notification('handled', {});
    const remote = notification('remote');
    registrations([handled, unread, otherUser, otherServer, legacy], [remote]);
    const read = vi.fn(async () => new Set(['handled', 'remote']));
    await cleanupPushNotifications(owner, read, () => true);
    expect(read).toHaveBeenCalledWith(['handled', 'unread', 'remote']);
    expect(handled.close).toHaveBeenCalledOnce();
    expect(remote.close).toHaveBeenCalledOnce();
    for (const item of [unread, otherUser, otherServer, legacy])
      expect(item.close).not.toHaveBeenCalled();
  });

  it('enumerates before reading and does not close pushes that arrive during the read', async () => {
    const old = notification('old');
    const late = notification('late');
    const displayed = [old];
    registrations(displayed);
    await cleanupPushNotifications(
      owner,
      async (ids) => {
        expect(ids).toEqual(['old']);
        displayed.push(late);
        return new Set(['old', 'late']);
      },
      () => true
    );
    expect(old.close).toHaveBeenCalledOnce();
    expect(late.close).not.toHaveBeenCalled();
  });

  it('discards results after account or state invalidation', async () => {
    const item = notification('handled');
    registrations([item]);
    let current = true;
    await cleanupPushNotifications(
      owner,
      async () => {
        current = false;
        return new Set(['handled']);
      },
      () => current
    );
    expect(item.close).not.toHaveBeenCalled();
  });

  it('does not query the server without matching displayed notifications', async () => {
    registrations([notification('legacy', {})]);
    const read = vi.fn();
    await cleanupPushNotifications(owner, read, () => true);
    expect(read).not.toHaveBeenCalled();
  });

  it('keeps notifications on read failures and tolerates missing browser APIs', async () => {
    const item = notification('handled');
    registrations([item]);
    await cleanupPushNotifications(
      owner,
      async () => {
        throw new Error('offline');
      },
      () => true
    );
    expect(item.close).not.toHaveBeenCalled();
    vi.stubGlobal('navigator', {});
    await expect(cleanupPushNotifications(owner, vi.fn(), () => true)).resolves.toBeUndefined();
  });
});
