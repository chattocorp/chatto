export interface BadgeCapableNavigator {
  setAppBadge?: (contents?: number) => Promise<void>;
  clearAppBadge?: () => Promise<void>;
}

export type ServiceWorkerBadgeIntent =
  | { kind: 'clear' }
  | { kind: 'flag' }
  | { kind: 'count'; count: number };

export interface NativeNotificationLike {
  close?: () => void;
}

export interface NotificationListingRegistration {
  getNotifications(options?: { tag?: string }): Promise<readonly NativeNotificationLike[]>;
}

export interface ForegroundBadgeIntentStorage {
  readForegroundBadgeIntent(): Promise<ServiceWorkerBadgeIntent | null>;
  readServiceWorkerAppBadgeEnabled(): Promise<boolean>;
  writeForegroundNotificationState(
    badgeIntent: ServiceWorkerBadgeIntent,
    serviceWorkerAppBadgeEnabled: boolean
  ): Promise<void>;
  clearForegroundBadgeIntent(): Promise<void>;
}

const FOREGROUND_BADGE_INTENT_REQUEST = '/__chatto/foreground-badge-intent';
const LEGACY_FOREGROUND_NOTIFICATION_COUNT_REQUEST = '/__chatto/foreground-notification-count';

function normalizeBadgeCount(notificationCount: number): number {
  if (!Number.isFinite(notificationCount)) return 0;
  return Math.max(0, Math.floor(notificationCount));
}

export function normalizeBadgeIntent(intent: ServiceWorkerBadgeIntent): ServiceWorkerBadgeIntent {
  if (intent.kind !== 'count') return intent;
  const count = normalizeBadgeCount(intent.count);
  return count > 0 ? { kind: 'count', count } : { kind: 'clear' };
}

interface StoredForegroundBadgeState {
  badgeIntent: ServiceWorkerBadgeIntent | null;
  serviceWorkerAppBadgeEnabled: boolean;
}

function normalizeStoredForegroundBadgeState(value: unknown): StoredForegroundBadgeState {
  if (!value || typeof value !== 'object') {
    return { badgeIntent: null, serviceWorkerAppBadgeEnabled: false };
  }

  const state = value as {
    badgeIntent?: unknown;
    notificationCount?: unknown;
    serviceWorkerAppBadgeEnabled?: unknown;
  };

  const badgeIntent = normalizeUnknownBadgeIntent(state.badgeIntent);
  const legacyCount =
    typeof state.notificationCount === 'number'
      ? badgeIntentFromCount(state.notificationCount)
      : null;
  return {
    badgeIntent: badgeIntent ?? legacyCount,
    serviceWorkerAppBadgeEnabled: state.serviceWorkerAppBadgeEnabled === true
  };
}

export function normalizeUnknownBadgeIntent(value: unknown): ServiceWorkerBadgeIntent | null {
  if (!value || typeof value !== 'object') return null;

  const intent = value as { kind?: unknown; count?: unknown };
  switch (intent.kind) {
    case 'clear':
      return { kind: 'clear' };
    case 'flag':
      return { kind: 'flag' };
    case 'count':
      return typeof intent.count === 'number'
        ? normalizeBadgeIntent({ kind: 'count', count: intent.count })
        : null;
    default:
      return null;
  }
}

function badgeIntentFromCount(notificationCount: number): ServiceWorkerBadgeIntent {
  const count = normalizeBadgeCount(notificationCount);
  return count > 0 ? { kind: 'count', count } : { kind: 'clear' };
}

export function createCacheForegroundBadgeIntentStorage(
  caches: CacheStorage,
  cacheName: string
): ForegroundBadgeIntentStorage {
  async function readState(): Promise<StoredForegroundBadgeState> {
    try {
      const cache = await caches.open(cacheName);
      const response =
        (await cache.match(FOREGROUND_BADGE_INTENT_REQUEST)) ??
        (await cache.match(LEGACY_FOREGROUND_NOTIFICATION_COUNT_REQUEST));
      if (!response) return { badgeIntent: null, serviceWorkerAppBadgeEnabled: false };

      return normalizeStoredForegroundBadgeState(await response.json());
    } catch {
      return { badgeIntent: null, serviceWorkerAppBadgeEnabled: false };
    }
  }

  async function writeState(state: StoredForegroundBadgeState): Promise<void> {
    try {
      const cache = await caches.open(cacheName);
      await cache.put(
        FOREGROUND_BADGE_INTENT_REQUEST,
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
    async readForegroundBadgeIntent() {
      return (await readState()).badgeIntent;
    },
    async readServiceWorkerAppBadgeEnabled() {
      return (await readState()).serviceWorkerAppBadgeEnabled;
    },
    async writeForegroundNotificationState(badgeIntent, serviceWorkerAppBadgeEnabled) {
      await writeState({
        badgeIntent: normalizeBadgeIntent(badgeIntent),
        serviceWorkerAppBadgeEnabled
      });
    },
    async clearForegroundBadgeIntent() {
      const state = await readState();
      await writeState({ ...state, badgeIntent: null });
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
  options: { minimumBadgeIntent?: ServiceWorkerBadgeIntent } = {}
): Promise<void> {
  const minimumBadgeIntent = options.minimumBadgeIntent
    ? normalizeBadgeIntent(options.minimumBadgeIntent)
    : null;
  let notifications: readonly NativeNotificationLike[];
  try {
    notifications = await registration.getNotifications();
  } catch {
    if (minimumBadgeIntent) {
      await applyBadgeIntent(badgeNavigator, minimumBadgeIntent);
    }
    return;
  }

  const listedMinimumBadgeIntent = minimumBadgeIntent?.kind === 'clear' ? null : minimumBadgeIntent;
  if (listedMinimumBadgeIntent) {
    await applyBadgeIntent(badgeNavigator, listedMinimumBadgeIntent);
  } else if (notifications.length > 0) {
    await applyBadgeIntent(badgeNavigator, { kind: 'flag' });
  } else {
    await applyBadgeIntent(badgeNavigator, { kind: 'clear' });
  }
}

export async function applyAuthoritativeBadgeState(
  registration: NotificationListingRegistration,
  badgeNavigator: BadgeCapableNavigator,
  badgeIntent: ServiceWorkerBadgeIntent,
  options: { isCurrent?: () => boolean } = {}
): Promise<void> {
  const intent = normalizeBadgeIntent(badgeIntent);
  if (intent.kind !== 'clear') {
    if (options.isCurrent && !options.isCurrent()) return;
    await applyBadgeIntent(badgeNavigator, intent);
    return;
  }

  let notifications: readonly NativeNotificationLike[] = [];
  try {
    notifications = await registration.getNotifications();
  } catch {
    // Still clear the badge below; the foreground app's clear intent is the
    // authoritative notification state even if native listing is unavailable.
  }

  if (options.isCurrent && !options.isCurrent()) return;
  for (const notification of notifications) {
    notification.close?.();
  }
  await applyBadgeIntent(badgeNavigator, intent);
}

export async function applyBadgeIntent(
  badgeNavigator: BadgeCapableNavigator,
  badgeIntent: ServiceWorkerBadgeIntent
): Promise<void> {
  const intent = normalizeBadgeIntent(badgeIntent);
  switch (intent.kind) {
    case 'count':
      await (badgeNavigator.setAppBadge?.(intent.count).catch(() => {}) ?? Promise.resolve());
      break;
    case 'flag':
      await (badgeNavigator.setAppBadge?.().catch(() => {}) ?? Promise.resolve());
      break;
    case 'clear':
      await (badgeNavigator.clearAppBadge?.().catch(() => {}) ?? Promise.resolve());
      break;
  }
}

export class ServiceWorkerBadgeCoordinator {
  #foregroundBadgeIntent: ServiceWorkerBadgeIntent | null = null;
  #serviceWorkerAppBadgeEnabled: boolean | null = null;
  #gate = new BadgeStateVersionGate();

  constructor(
    private readonly registration: NotificationListingRegistration,
    private readonly badgeNavigator: BadgeCapableNavigator,
    private readonly foregroundBadgeIntentStorage?: ForegroundBadgeIntentStorage
  ) {}

  async applyForegroundNotificationCount(
    notificationCount: number,
    options: { serviceWorkerAppBadgeEnabled?: boolean } = {}
  ): Promise<void> {
    await this.applyForegroundBadgeIntent(badgeIntentFromCount(notificationCount), options);
  }

  async applyForegroundBadgeIntent(
    badgeIntent: ServiceWorkerBadgeIntent,
    options: { serviceWorkerAppBadgeEnabled?: boolean } = {}
  ): Promise<void> {
    const intent = normalizeBadgeIntent(badgeIntent);
    this.#foregroundBadgeIntent = intent;
    if (options.serviceWorkerAppBadgeEnabled !== undefined) {
      this.#serviceWorkerAppBadgeEnabled = options.serviceWorkerAppBadgeEnabled;
    }
    const isCurrent = this.#gate.next();
    await this.foregroundBadgeIntentStorage?.writeForegroundNotificationState(
      intent,
      await this.isServiceWorkerAppBadgeEnabled()
    );
    await applyAuthoritativeBadgeState(
      this.registration,
      await this.badgeNavigatorIfEnabled(),
      intent,
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
    this.#foregroundBadgeIntent = null;
    await this.foregroundBadgeIntentStorage?.clearForegroundBadgeIntent();
    await syncBadgeFromNativeNotifications(this.registration, await this.badgeNavigatorIfEnabled());
  }

  async reconcileAfterNotificationClick(): Promise<void> {
    this.#gate.invalidate();
    const persistedForegroundIntent =
      (await this.foregroundBadgeIntentStorage?.readForegroundBadgeIntent()) ?? null;
    await syncBadgeFromNativeNotifications(
      this.registration,
      await this.badgeNavigatorIfEnabled(),
      {
        minimumBadgeIntent: this.#foregroundBadgeIntent ?? persistedForegroundIntent ?? undefined
      }
    );
  }

  async setProvisionalPushFlagBadge(): Promise<void> {
    await applyBadgeIntent(await this.badgeNavigatorIfEnabled(), { kind: 'flag' });
  }

  async setPushAppBadgeCount(notificationCount: number): Promise<void> {
    this.#gate.invalidate();
    await applyBadgeIntent(
      await this.badgeNavigatorIfEnabled(),
      badgeIntentFromCount(notificationCount)
    );
  }

  private async isServiceWorkerAppBadgeEnabled(): Promise<boolean> {
    if (this.#serviceWorkerAppBadgeEnabled !== null) return this.#serviceWorkerAppBadgeEnabled;
    if (!this.foregroundBadgeIntentStorage) return true;

    this.#serviceWorkerAppBadgeEnabled =
      await this.foregroundBadgeIntentStorage.readServiceWorkerAppBadgeEnabled();
    return this.#serviceWorkerAppBadgeEnabled;
  }

  private async badgeNavigatorIfEnabled(): Promise<BadgeCapableNavigator> {
    return (await this.isServiceWorkerAppBadgeEnabled()) ? this.badgeNavigator : {};
  }
}
