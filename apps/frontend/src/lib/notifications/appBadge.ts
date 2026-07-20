export const APP_BADGE_REFRESH_MESSAGE_TYPE = 'app-badge-refresh';

export type AppBadgeIntent =
  | { kind: 'clear' }
  | { kind: 'flag' }
  | { kind: 'count'; count: number };

type AppBadgeRefreshMessage = {
  type: typeof APP_BADGE_REFRESH_MESSAGE_TYPE;
};

/** Updates the installed app badge from Chatto's authoritative notification state. */
export async function updateAppBadge(intent: AppBadgeIntent): Promise<void> {
  if (typeof navigator === 'undefined') return;

  try {
    switch (intent.kind) {
      case 'count':
        await navigator.setAppBadge?.(intent.count);
        break;
      case 'flag':
        await navigator.setAppBadge?.();
        break;
      case 'clear':
        await navigator.clearAppBadge?.();
        break;
    }
  } catch {
    // Badge support and permission vary by browser and installation context.
  }
}

/** Replays the visible page's aggregate badge when a regular push may have replaced it. */
export function listenForAppBadgeRefresh(refresh: () => void): () => void {
  if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return () => {};

  const handler = (event: MessageEvent<unknown>) => {
    if (!isAppBadgeRefreshMessage(event.data)) return;
    refresh();
  };

  navigator.serviceWorker.addEventListener('message', handler);
  return () => navigator.serviceWorker.removeEventListener('message', handler);
}

function isAppBadgeRefreshMessage(value: unknown): value is AppBadgeRefreshMessage {
  return (
    typeof value === 'object' &&
    value !== null &&
    'type' in value &&
    value.type === APP_BADGE_REFRESH_MESSAGE_TYPE
  );
}
