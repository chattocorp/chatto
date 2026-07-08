import { describe, expect, it, vi } from 'vitest';
import {
  applyAuthoritativeBadgeState,
  BadgeStateVersionGate,
  type ForegroundNotificationCountStorage,
  ServiceWorkerBadgeCoordinator,
  syncBadgeFromNativeNotifications
} from './notificationBadge.worker';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

function createMemoryForegroundCountStorage(): ForegroundNotificationCountStorage {
  let notificationCount: number | null = null;
  let serviceWorkerAppBadgeEnabled = false;
  return {
    async readForegroundNotificationCount() {
      return notificationCount;
    },
    async readServiceWorkerAppBadgeEnabled() {
      return serviceWorkerAppBadgeEnabled;
    },
    async writeForegroundNotificationState(count, enabled) {
      notificationCount = count;
      serviceWorkerAppBadgeEnabled = enabled;
    },
    async clearForegroundNotificationCount() {
      notificationCount = null;
    }
  };
}

describe('syncBadgeFromNativeNotifications', () => {
  it('sets the app badge to the remaining native notification count', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [{}, {}])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await syncBadgeFromNativeNotifications(registration, badgeNavigator);

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).toHaveBeenCalledWith(2);
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('uses the foreground notification count as a lower bound when requested', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await syncBadgeFromNativeNotifications(registration, badgeNavigator, {
      minimumNotificationCount: 3
    });

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).toHaveBeenCalledWith(3);
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('clears the app badge when no native notifications remain', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await syncBadgeFromNativeNotifications(registration, badgeNavigator);

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
  });

  it('does not update the app badge when native notification listing fails', async () => {
    const registration = {
      getNotifications: vi.fn(async () => {
        throw new Error('notification store unavailable');
      })
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await syncBadgeFromNativeNotifications(registration, badgeNavigator);

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('preserves the foreground lower bound when native notification listing fails', async () => {
    const registration = {
      getNotifications: vi.fn(async () => {
        throw new Error('notification store unavailable');
      })
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await syncBadgeFromNativeNotifications(registration, badgeNavigator, {
      minimumNotificationCount: 3
    });

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).toHaveBeenCalledWith(3);
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });
});

describe('applyAuthoritativeBadgeState', () => {
  it('sets a numeric app badge when the authoritative notification count is positive', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [{ close: vi.fn() }])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await applyAuthoritativeBadgeState(registration, badgeNavigator, 3);

    expect(badgeNavigator.setAppBadge).toHaveBeenCalledWith(3);
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
    expect(registration.getNotifications).not.toHaveBeenCalled();
  });

  it('skips a stale positive badge update when a newer state arrived', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await applyAuthoritativeBadgeState(registration, badgeNavigator, 3, {
      isCurrent: () => false
    });

    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
    expect(registration.getNotifications).not.toHaveBeenCalled();
  });

  it('closes stale native notifications and clears the app badge when count is zero', async () => {
    const nativeNotifications = [{ close: vi.fn() }, { close: vi.fn() }];
    const registration = {
      getNotifications: vi.fn(async () => nativeNotifications)
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await applyAuthoritativeBadgeState(registration, badgeNavigator, 0);

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(nativeNotifications[0].close).toHaveBeenCalledOnce();
    expect(nativeNotifications[1].close).toHaveBeenCalledOnce();
    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
  });

  it('does not close notifications or clear the badge when a zero update becomes stale', async () => {
    const nativeNotifications = [{ close: vi.fn() }, { close: vi.fn() }];
    const registration = {
      getNotifications: vi.fn(async () => nativeNotifications)
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await applyAuthoritativeBadgeState(registration, badgeNavigator, 0, {
      isCurrent: () => false
    });

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(nativeNotifications[0].close).not.toHaveBeenCalled();
    expect(nativeNotifications[1].close).not.toHaveBeenCalled();
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
  });

  it('does not close a pushed notification when a pending zero update is invalidated', async () => {
    const nativeNotifications = [{ close: vi.fn() }];
    const listing = deferred<typeof nativeNotifications>();
    const registration = {
      getNotifications: vi.fn(() => listing.promise)
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const gate = new BadgeStateVersionGate();

    const pending = applyAuthoritativeBadgeState(registration, badgeNavigator, 0, {
      isCurrent: gate.next()
    });
    gate.invalidate();
    listing.resolve(nativeNotifications);
    await pending;

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(nativeNotifications[0].close).not.toHaveBeenCalled();
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
  });

  it('still clears the app badge when native notification listing fails for zero count', async () => {
    const registration = {
      getNotifications: vi.fn(async () => {
        throw new Error('notification store unavailable');
      })
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };

    await applyAuthoritativeBadgeState(registration, badgeNavigator, 0);

    expect(registration.getNotifications).toHaveBeenCalledOnce();
    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
  });
});

describe('ServiceWorkerBadgeCoordinator', () => {
  it('preserves the authoritative foreground count after clicking the only native notification', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const coordinator = new ServiceWorkerBadgeCoordinator(registration, badgeNavigator);

    await coordinator.applyForegroundNotificationCount(3);
    await coordinator.reconcileAfterNotificationClick();

    expect(badgeNavigator.setAppBadge).toHaveBeenLastCalledWith(3);
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('preserves the authoritative foreground count after a service worker restart', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const foregroundCountStorage = createMemoryForegroundCountStorage();

    await new ServiceWorkerBadgeCoordinator(
      registration,
      badgeNavigator,
      foregroundCountStorage
    ).applyForegroundNotificationCount(3, { serviceWorkerAppBadgeEnabled: true });

    await new ServiceWorkerBadgeCoordinator(
      registration,
      badgeNavigator,
      foregroundCountStorage
    ).reconcileAfterNotificationClick();

    expect(badgeNavigator.setAppBadge).toHaveBeenLastCalledWith(3);
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('does not call the worker Badging API when foreground reports a browser tab context', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const foregroundCountStorage = createMemoryForegroundCountStorage();

    const coordinator = new ServiceWorkerBadgeCoordinator(
      registration,
      badgeNavigator,
      foregroundCountStorage
    );

    await coordinator.applyForegroundNotificationCount(3, { serviceWorkerAppBadgeEnabled: false });
    await coordinator.reconcileAfterNotificationClick();
    await coordinator.setProvisionalPushFlagBadge();

    expect(badgeNavigator.setAppBadge).not.toHaveBeenCalled();
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('clears a push-only badge after clicking the only native notification', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const coordinator = new ServiceWorkerBadgeCoordinator(registration, badgeNavigator);

    coordinator.recordRegularPush();
    await coordinator.setProvisionalPushFlagBadge();
    await coordinator.reconcileAfterNotificationClick();

    expect(badgeNavigator.setAppBadge).toHaveBeenCalledOnce();
    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
  });

  it('sets push app badge counts immediately without preserving them after click', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const coordinator = new ServiceWorkerBadgeCoordinator(registration, badgeNavigator);

    await coordinator.setPushAppBadgeCount(1);
    await coordinator.reconcileAfterNotificationClick();

    expect(badgeNavigator.setAppBadge).toHaveBeenCalledWith(1);
    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
  });

  it('does not persist push app badge counts as foreground state', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const foregroundCountStorage = createMemoryForegroundCountStorage();

    const firstCoordinator = new ServiceWorkerBadgeCoordinator(
      registration,
      badgeNavigator,
      foregroundCountStorage
    );
    await firstCoordinator.applyForegroundNotificationCount(0, {
      serviceWorkerAppBadgeEnabled: true
    });
    badgeNavigator.clearAppBadge.mockClear();
    badgeNavigator.setAppBadge.mockClear();

    await firstCoordinator.setPushAppBadgeCount(1);

    await new ServiceWorkerBadgeCoordinator(
      registration,
      badgeNavigator,
      foregroundCountStorage
    ).reconcileAfterNotificationClick();

    expect(badgeNavigator.setAppBadge).toHaveBeenCalledWith(1);
    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
  });

  it('does not preserve a cached foreground count after a dismiss push', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const coordinator = new ServiceWorkerBadgeCoordinator(registration, badgeNavigator);

    await coordinator.applyForegroundNotificationCount(1);
    await coordinator.reconcileAfterDismissPush();

    expect(badgeNavigator.clearAppBadge).toHaveBeenCalledOnce();
    expect(badgeNavigator.setAppBadge).toHaveBeenCalledTimes(1);
  });

  it('clears the persisted foreground count after a dismiss push', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const foregroundCountStorage = createMemoryForegroundCountStorage();

    const coordinator = new ServiceWorkerBadgeCoordinator(
      registration,
      badgeNavigator,
      foregroundCountStorage
    );
    await coordinator.applyForegroundNotificationCount(3, { serviceWorkerAppBadgeEnabled: true });
    await coordinator.reconcileAfterDismissPush();

    await new ServiceWorkerBadgeCoordinator(
      registration,
      badgeNavigator,
      foregroundCountStorage
    ).reconcileAfterNotificationClick();

    expect(badgeNavigator.clearAppBadge).toHaveBeenLastCalledWith();
    expect(badgeNavigator.setAppBadge).toHaveBeenCalledTimes(1);
  });

  it('invalidates a pending zero-count foreground reconciliation when a regular push arrives', async () => {
    const nativeNotifications = [{ close: vi.fn() }];
    const listing = deferred<typeof nativeNotifications>();
    const registration = {
      getNotifications: vi.fn(() => listing.promise)
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const coordinator = new ServiceWorkerBadgeCoordinator(registration, badgeNavigator);

    const pending = coordinator.applyForegroundNotificationCount(0);
    coordinator.recordRegularPush();
    listing.resolve(nativeNotifications);
    await pending;

    expect(nativeNotifications[0].close).not.toHaveBeenCalled();
    expect(badgeNavigator.clearAppBadge).not.toHaveBeenCalled();
  });

  it('does not treat regular pushes as exact increments of the foreground count', async () => {
    const registration = {
      getNotifications: vi.fn(async () => [])
    };
    const badgeNavigator = {
      setAppBadge: vi.fn(async () => {}),
      clearAppBadge: vi.fn(async () => {})
    };
    const coordinator = new ServiceWorkerBadgeCoordinator(registration, badgeNavigator);

    await coordinator.applyForegroundNotificationCount(3);
    coordinator.recordRegularPush();
    await coordinator.reconcileAfterNotificationClick();

    expect(badgeNavigator.setAppBadge).toHaveBeenLastCalledWith(3);
  });
});
