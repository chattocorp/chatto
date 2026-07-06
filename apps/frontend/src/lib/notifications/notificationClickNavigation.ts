import type { AppUiState } from '$lib/state/appUi.svelte';
import { prepareUiForNotificationPath } from './notificationNavigationUi';

export const NOTIFICATION_CLICK_NAVIGATION_DEDUPE_MS = 2_000;

type NotificationClickUiController = Pick<AppUiState, 'disableRoomCallWideFor'>;

type NotificationClickUrlHandlerOptions = {
  appUi: NotificationClickUiController;
  clearPendingUrl?: (target: { clickId?: string; expectedUrl: string }) => Promise<void>;
  navigate: (path: string) => Promise<void>;
  now?: () => number;
  origin?: string;
};

type HandleNotificationClickUrlOptions = {
  clickId?: string;
  pendingAlreadyConsumed?: boolean;
};

export function createNotificationClickUrlHandler(options: NotificationClickUrlHandlerOptions) {
  let lastUrl: string | null = null;
  let lastHandledAt = 0;

  return async function handleNotificationClickUrl(
    url: string,
    handleOptions: HandleNotificationClickUrlOptions = {}
  ): Promise<boolean> {
    const origin = options.origin ?? window.location.origin;
    let target: URL;
    try {
      target = new URL(url);
    } catch {
      return false;
    }
    if (target.origin !== origin) return false;

    const now = options.now?.() ?? Date.now();
    if (lastUrl === target.href && now - lastHandledAt < NOTIFICATION_CLICK_NAVIGATION_DEDUPE_MS) {
      if (!handleOptions.pendingAlreadyConsumed) {
        await options.clearPendingUrl?.({
          clickId: handleOptions.clickId,
          expectedUrl: target.href
        });
      }
      return false;
    }
    lastUrl = target.href;
    lastHandledAt = now;

    if (!handleOptions.pendingAlreadyConsumed) {
      await options.clearPendingUrl?.({
        clickId: handleOptions.clickId,
        expectedUrl: target.href
      });
    }

    prepareUiForNotificationPath(options.appUi, target.pathname);
    await options.navigate(target.pathname + target.search + target.hash);
    return true;
  };
}
