import { describe, expect, it, vi } from 'vitest';
import { createNotificationClickUrlHandler } from './notificationClickNavigation';

const appUi = {
  disableRoomCallWideFor: vi.fn()
};

const mocks = vi.hoisted(() => ({
  segmentToServerId: vi.fn((segment: string) => (segment === '-' ? 'origin' : null))
}));

vi.mock('$lib/navigation', () => ({
  segmentToServerId: mocks.segmentToServerId
}));

describe('createNotificationClickUrlHandler', () => {
  it('clears the pending target, prepares room UI, and navigates same-origin URLs', async () => {
    const clearPendingUrl = vi.fn(async () => {});
    const navigate = vi.fn(async () => {});
    appUi.disableRoomCallWideFor.mockClear();
    const handleNotificationClickUrl = createNotificationClickUrlHandler({
      appUi,
      clearPendingUrl,
      navigate,
      now: () => 100,
      origin: 'https://chatto.example'
    });

    await expect(
      handleNotificationClickUrl('https://chatto.example/chat/-/room-1?highlight=event-1')
    ).resolves.toBe(true);

    expect(clearPendingUrl).toHaveBeenCalledWith(
      'https://chatto.example/chat/-/room-1?highlight=event-1'
    );
    expect(appUi.disableRoomCallWideFor).toHaveBeenCalledWith('origin', 'room-1');
    expect(navigate).toHaveBeenCalledWith('/chat/-/room-1?highlight=event-1');
    expect(clearPendingUrl.mock.invocationCallOrder[0]).toBeLessThan(
      navigate.mock.invocationCallOrder[0]
    );
  });

  it('does not clear again when the pending target was already consumed', async () => {
    const clearPendingUrl = vi.fn(async () => {});
    const navigate = vi.fn(async () => {});
    const handleNotificationClickUrl = createNotificationClickUrlHandler({
      appUi,
      clearPendingUrl,
      navigate,
      origin: 'https://chatto.example'
    });

    await expect(
      handleNotificationClickUrl('https://chatto.example/chat/-/room-1', {
        pendingAlreadyConsumed: true
      })
    ).resolves.toBe(true);

    expect(clearPendingUrl).not.toHaveBeenCalled();
    expect(navigate).toHaveBeenCalledWith('/chat/-/room-1');
  });

  it('rejects malformed and cross-origin URLs', async () => {
    const clearPendingUrl = vi.fn(async () => {});
    const navigate = vi.fn(async () => {});
    const handleNotificationClickUrl = createNotificationClickUrlHandler({
      appUi,
      clearPendingUrl,
      navigate,
      origin: 'https://chatto.example'
    });

    await expect(handleNotificationClickUrl('http://[')).resolves.toBe(false);
    await expect(handleNotificationClickUrl('https://other.example/chat/-/room-1')).resolves.toBe(
      false
    );

    expect(clearPendingUrl).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
  });

  it('dedupes the same URL briefly so message ACKs and focus drains do not double-route', async () => {
    let now = 100;
    const clearPendingUrl = vi.fn(async () => {});
    const navigate = vi.fn(async () => {});
    const handleNotificationClickUrl = createNotificationClickUrlHandler({
      appUi,
      clearPendingUrl,
      navigate,
      now: () => now,
      origin: 'https://chatto.example'
    });

    await expect(handleNotificationClickUrl('https://chatto.example/chat/-/room-1')).resolves.toBe(
      true
    );
    now = 250;
    await expect(handleNotificationClickUrl('https://chatto.example/chat/-/room-1')).resolves.toBe(
      false
    );
    now = 3_000;
    await expect(handleNotificationClickUrl('https://chatto.example/chat/-/room-1')).resolves.toBe(
      true
    );

    expect(navigate).toHaveBeenCalledTimes(2);
    expect(clearPendingUrl).toHaveBeenCalledTimes(3);
  });
});
