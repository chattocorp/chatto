import { deleteRetiredCaches } from './retiredCaches';

/**
 * Retire the old root shell registration without losing a legacy push subscription.
 * Fresh installations never register a root worker. A subscribed root receives the
 * push-only worker instead; a later visit removes it after the existing push flow
 * has migrated its subscription. Scoped push workers and other applications stay
 * untouched. Failures reject; callers can retry this idempotent cleanup on a later
 * visit. Cache deletion runs even when the worker API or update is unavailable.
 */
export async function retireOfflineShell(): Promise<void> {
  try {
    if (!('serviceWorker' in navigator)) return;
    const rootScope = `${window.location.origin}/`;
    const scriptURL = new URL('/service-worker.js', rootScope).href;
    const registrations = await navigator.serviceWorker.getRegistrations();
    const root = registrations.find((registration) => {
      if (registration.scope !== rootScope) return false;
      const workers = [registration.active, registration.waiting, registration.installing].filter(
        (worker) => worker !== null
      );
      return workers.length > 0 && workers.every((worker) => worker.scriptURL === scriptURL);
    });
    if (!root) return;

    if (await root.pushManager.getSubscription()) {
      // Existing push migration saves the scoped subscription before retiring this one.
      await root.update();
    } else {
      await root.unregister();
    }
  } finally {
    await deleteRetiredCaches();
  }
}
