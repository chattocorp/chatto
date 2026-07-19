export interface BadgeCapableNavigator {
  setAppBadge?: (contents?: number) => Promise<void>;
  clearAppBadge?: () => Promise<void>;
}

export type ServiceWorkerBadgeIntent =
  | { kind: 'clear' }
  | { kind: 'flag' }
  | { kind: 'count'; count: number };

export interface NotificationListingRegistration {
  getNotifications(): Promise<readonly unknown[]>;
}

function normalizeBadgeCount(count: number): number {
  if (!Number.isFinite(count)) return 0;
  return Math.max(0, Math.floor(count));
}

/** Applies a badge hint received as part of a Web Push notification. */
export async function applyPushBadgeIntent(
  badgeNavigator: BadgeCapableNavigator,
  badgeIntent: ServiceWorkerBadgeIntent
): Promise<void> {
  switch (badgeIntent.kind) {
    case 'count': {
      const count = normalizeBadgeCount(badgeIntent.count);
      if (count > 0) {
        await (badgeNavigator.setAppBadge?.(count).catch(() => {}) ?? Promise.resolve());
      } else {
        await (badgeNavigator.clearAppBadge?.().catch(() => {}) ?? Promise.resolve());
      }
      break;
    }
    case 'flag':
      await (badgeNavigator.setAppBadge?.().catch(() => {}) ?? Promise.resolve());
      break;
    case 'clear':
      await (badgeNavigator.clearAppBadge?.().catch(() => {}) ?? Promise.resolve());
      break;
  }
}

/** Reconciles the push-owned badge after a native notification is removed. */
export async function reconcilePushBadge(
  registration: NotificationListingRegistration,
  badgeNavigator: BadgeCapableNavigator
): Promise<void> {
  let notifications: readonly unknown[];
  try {
    notifications = await registration.getNotifications();
  } catch {
    return;
  }

  await applyPushBadgeIntent(
    badgeNavigator,
    notifications.length > 0 ? { kind: 'flag' } : { kind: 'clear' }
  );
}
