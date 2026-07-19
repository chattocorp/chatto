import { describe, expect, it, vi } from 'vitest';
import { applyPushBadgeIntent, reconcilePushBadge } from './notificationBadge.worker';

describe('applyPushBadgeIntent', () => {
  it('sets numeric and flag badges from push intents', async () => {
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await applyPushBadgeIntent(badgeNavigator, { kind: 'count', count: 3 });
    await applyPushBadgeIntent(badgeNavigator, { kind: 'flag' });

    expect(badgeNavigator.setAppBadge).toHaveBeenNthCalledWith(1, 3);
    expect(badgeNavigator.setAppBadge).toHaveBeenNthCalledWith(2);
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('normalizes non-positive counts to a clear', async () => {
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await applyPushBadgeIntent(badgeNavigator, { kind: 'count', count: -1 });

    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
  });
});

describe('reconcilePushBadge', () => {
  it('keeps a flag while native push notifications remain', async () => {
    const registration = { getNotifications: vi.fn(async () => [{}]) };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await reconcilePushBadge(registration, badgeNavigator);

    expect(badgeNavigator.setAppBadge).toHaveBeenCalledWith();
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('clears the badge when no native push notifications remain', async () => {
    const registration = { getNotifications: vi.fn(async () => []) };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await reconcilePushBadge(registration, badgeNavigator);

    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
  });

  it('leaves the badge unchanged when native notification listing fails', async () => {
    const registration = {
      getNotifications: vi.fn(async () => {
        throw new Error('notification store unavailable');
      })
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await reconcilePushBadge(registration, badgeNavigator);

    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });
});
