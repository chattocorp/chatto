export interface BadgeCapableNavigator {
  setAppBadge?: (contents?: number) => Promise<void>;
  clearAppBadge?: () => Promise<void>;
}

export interface NativeNotificationLike {
  close?: () => void;
}

export interface NotificationListingRegistration {
  getNotifications(options?: { tag?: string }): Promise<readonly NativeNotificationLike[]>;
}

export interface ForegroundNotificationCountStorage {
  readForegroundNotificationCount(): Promise<number | null>;
  readServiceWorkerAppBadgeEnabled(): Promise<boolean>;
  writeForegroundNotificationState(
    notificationCount: number,
    serviceWorkerAppBadgeEnabled: boolean
  ): Promise<void>;
  clearForegroundNotificationCount(): Promise<void>;
}

const FOREGROUND_NOTIFICATION_COUNT_REQUEST = '/__chatto/foreground-notification-count';

function normalizeBadgeCount(notificationCount: number): number {
  if (!Number.isFinite(notificationCount)) return 0;
  return Math.max(0, Math.floor(notificationCount));
}

interface StoredForegroundBadgeState {
  notificationCount: number | null;
  serviceWorkerAppBadgeEnabled: boolean;
}

function normalizeStoredForegroundBadgeState(value: unknown): StoredForegroundBadgeState {
  if (!value || typeof value !== 'object') {
    return { notificationCount: null, serviceWorkerAppBadgeEnabled: false };
  }

  const state = value as {
    notificationCount?: unknown;
    serviceWorkerAppBadgeEnabled?: unknown;
  };
  return {
    notificationCount:
      typeof state.notificationCount === 'number'
        ? normalizeBadgeCount(state.notificationCount)
        : null,
    serviceWorkerAppBadgeEnabled: state.serviceWorkerAppBadgeEnabled === true
  };
}

export function createCacheForegroundNotificationCountStorage(
  caches: CacheStorage,
  cacheName: string
): ForegroundNotificationCountStorage {
  async function readState(): Promise<StoredForegroundBadgeState> {
    try {
      const cache = await caches.open(cacheName);
      const response = await cache.match(FOREGROUND_NOTIFICATION_COUNT_REQUEST);
      if (!response) return { notificationCount: null, serviceWorkerAppBadgeEnabled: false };

      return normalizeStoredForegroundBadgeState(await response.json());
    } catch {
      return { notificationCount: null, serviceWorkerAppBadgeEnabled: false };
    }
  }

  async function writeState(state: StoredForegroundBadgeState): Promise<void> {
    try {
      const cache = await caches.open(cacheName);
      await cache.put(
        FOREGROUND_NOTIFICATION_COUNT_REQUEST,
        new Response(JSON.stringify(state), {
          headers: { 'content-type': 'application/json' }
        })
      );
    } catch {
      // Badge state persistence is best-effort; foreground messages still update
      // the current worker instance and the visible app badge.
    }
  }

  return {
    async readForegroundNotificationCount() {
      return (await readState()).notificationCount;
    },
    async readServiceWorkerAppBadgeEnabled() {
      return (await readState()).serviceWorkerAppBadgeEnabled;
    },
    async writeForegroundNotificationState(notificationCount, serviceWorkerAppBadgeEnabled) {
      await writeState({
        notificationCount: normalizeBadgeCount(notificationCount),
        serviceWorkerAppBadgeEnabled
      });
    },
    async clearForegroundNotificationCount() {
      const state = await readState();
      await writeState({ ...state, notificationCount: null });
    }
  };
}

export class BadgeStateVersionGate {
  #version = 0;

  next(): () => boolean {
    const version = ++this.#version;
    return () => version === this.#version;
  }

  invalidate(): void {
    this.#version++;
  }
}

export async function syncBadgeFromNativeNotifications(
  registration: NotificationListingRegistration,
  badgeNavigator: BadgeCapableNavigator,
  options: { minimumNotificationCount?: number } = {}
): Promise<void> {
  const minimumNotificationCount = normalizeBadgeCount(options.minimumNotificationCount ?? 0);
  let notifications: readonly NativeNotificationLike[];
  try {
    notifications = await registration.getNotifications();
  } catch {
    if (minimumNotificationCount > 0) {
      await (badgeNavigator.setAppBadge?.(minimumNotificationCount).catch(() => {}) ??
        Promise.resolve());
    }
    return;
  }

  const count = Math.max(notifications.length, minimumNotificationCount);
  if (count > 0) {
    await (badgeNavigator.setAppBadge?.(count).catch(() => {}) ?? Promise.resolve());
  } else {
    await (badgeNavigator.clearAppBadge?.().catch(() => {}) ?? Promise.resolve());
  }
}

export async function applyAuthoritativeBadgeState(
  registration: NotificationListingRegistration,
  badgeNavigator: BadgeCapableNavigator,
  notificationCount: number,
  options: { isCurrent?: () => boolean } = {}
): Promise<void> {
  const count = normalizeBadgeCount(notificationCount);
  if (count > 0) {
    if (options.isCurrent && !options.isCurrent()) return;
    await (badgeNavigator.setAppBadge?.(count).catch(() => {}) ?? Promise.resolve());
    return;
  }

  let notifications: readonly NativeNotificationLike[] = [];
  try {
    notifications = await registration.getNotifications();
  } catch {
    // Still clear the badge below; the foreground app's zero count is the
    // authoritative notification state even if native listing is unavailable.
  }

  if (options.isCurrent && !options.isCurrent()) return;
  for (const notification of notifications) {
    notification.close?.();
  }
  await (badgeNavigator.clearAppBadge?.().catch(() => {}) ?? Promise.resolve());
}

export class ServiceWorkerBadgeCoordinator {
  #foregroundNotificationCount: number | null = null;
  #serviceWorkerAppBadgeEnabled: boolean | null = null;
  #gate = new BadgeStateVersionGate();

  constructor(
    private readonly registration: NotificationListingRegistration,
    private readonly badgeNavigator: BadgeCapableNavigator,
    private readonly foregroundCountStorage?: ForegroundNotificationCountStorage
  ) {}

  async applyForegroundNotificationCount(
    notificationCount: number,
    options: { serviceWorkerAppBadgeEnabled?: boolean } = {}
  ): Promise<void> {
    const count = normalizeBadgeCount(notificationCount);
    this.#foregroundNotificationCount = count;
    if (options.serviceWorkerAppBadgeEnabled !== undefined) {
      this.#serviceWorkerAppBadgeEnabled = options.serviceWorkerAppBadgeEnabled;
    }
    const isCurrent = this.#gate.next();
    await this.foregroundCountStorage?.writeForegroundNotificationState(
      count,
      await this.isServiceWorkerAppBadgeEnabled()
    );
    await applyAuthoritativeBadgeState(
      this.registration,
      await this.badgeNavigatorIfEnabled(),
      count,
      {
        isCurrent
      }
    );
  }

  recordRegularPush(): void {
    this.#gate.invalidate();
  }

  async reconcileAfterDismissPush(): Promise<void> {
    this.#gate.invalidate();
    this.#foregroundNotificationCount = null;
    await this.foregroundCountStorage?.clearForegroundNotificationCount();
    await syncBadgeFromNativeNotifications(this.registration, await this.badgeNavigatorIfEnabled());
  }

  async reconcileAfterNotificationClick(): Promise<void> {
    this.#gate.invalidate();
    const persistedForegroundCount =
      (await this.foregroundCountStorage?.readForegroundNotificationCount()) ?? 0;
    await syncBadgeFromNativeNotifications(
      this.registration,
      await this.badgeNavigatorIfEnabled(),
      {
        minimumNotificationCount: Math.max(
          this.#foregroundNotificationCount ?? 0,
          persistedForegroundCount
        )
      }
    );
  }

  async setProvisionalPushFlagBadge(): Promise<void> {
    const badgeNavigator = await this.badgeNavigatorIfEnabled();
    await (badgeNavigator.setAppBadge?.().catch(() => {}) ?? Promise.resolve());
  }

  async setPushAppBadgeCount(notificationCount: number): Promise<void> {
    this.#gate.invalidate();
    const count = normalizeBadgeCount(notificationCount);

    const badgeNavigator = await this.badgeNavigatorIfEnabled();
    if (count > 0) {
      await (badgeNavigator.setAppBadge?.(count).catch(() => {}) ?? Promise.resolve());
    } else {
      await (badgeNavigator.clearAppBadge?.().catch(() => {}) ?? Promise.resolve());
    }
  }

  private async isServiceWorkerAppBadgeEnabled(): Promise<boolean> {
    if (this.#serviceWorkerAppBadgeEnabled !== null) return this.#serviceWorkerAppBadgeEnabled;
    if (!this.foregroundCountStorage) return true;

    this.#serviceWorkerAppBadgeEnabled =
      await this.foregroundCountStorage.readServiceWorkerAppBadgeEnabled();
    return this.#serviceWorkerAppBadgeEnabled;
  }

  private async badgeNavigatorIfEnabled(): Promise<BadgeCapableNavigator> {
    return (await this.isServiceWorkerAppBadgeEnabled()) ? this.badgeNavigator : {};
  }
}
