import { afterEach, describe, expect, it, vi } from 'vitest';
import { scheduleCustomStatusExpiry } from './userProfiles.svelte';

describe('custom status expiry scheduling', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it('fires when a custom status expires', async () => {
    vi.useFakeTimers();
    const now = new Date('2026-06-24T12:00:00.000Z').getTime();
    const dateNow = vi.spyOn(Date, 'now').mockReturnValue(now);
    const onExpire = vi.fn();

    const cleanup = scheduleCustomStatusExpiry(
      {
        emoji: '🍜',
        text: 'Lunch',
        expiresAt: '2026-06-24T12:01:00.000Z'
      },
      onExpire
    );

    await vi.advanceTimersByTimeAsync(59_999);
    expect(onExpire).not.toHaveBeenCalled();

    dateNow.mockReturnValue(now + 60_000);
    await vi.advanceTimersByTimeAsync(1);
    await vi.advanceTimersByTimeAsync(0);
    expect(onExpire).toHaveBeenCalledTimes(1);

    cleanup();
  });

  it('cancels a scheduled custom status expiry', async () => {
    vi.useFakeTimers();
    vi.spyOn(Date, 'now').mockReturnValue(new Date('2026-06-24T12:00:00.000Z').getTime());
    const onExpire = vi.fn();

    const cleanup = scheduleCustomStatusExpiry(
      {
        emoji: '🍜',
        text: 'Lunch',
        expiresAt: '2026-06-24T12:01:00.000Z'
      },
      onExpire
    );

    cleanup();
    await vi.advanceTimersByTimeAsync(60_000);

    expect(onExpire).not.toHaveBeenCalled();
  });
});
