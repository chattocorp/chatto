/**
 * Serializes browser push creation across this origin's tabs. An attempt is
 * stored before creation starts and removed only after success. Failures and
 * interrupted pages therefore require an explicit recovery after a reload.
 * Existing subscriptions and server API saves do not use this gate.
 */
const attemptKey = 'chatto.push-registration.browser-attempt.v1';
const lockName = 'chatto.push-registration.browser';

/** The failed attempt that one explicit user action is allowed to retry. */
export type BrowserPushRecovery = { readonly attempt: string | null };

/** Capture once per user action, before its servers enter their queues. */
export function captureBrowserPushRecovery(): BrowserPushRecovery {
  return { attempt: window.localStorage.getItem(attemptKey) };
}

/** A creation attempt was skipped, not rejected again by the push service. */
export class BrowserPushRegistrationPaused extends Error {
  constructor() {
    super('Automatic push setup is paused. Try again in notification settings.');
    this.name = 'BrowserPushRegistrationPaused';
  }
}

/**
 * Creates at most one subscription at a time across all servers on this
 * origin. The recovery snapshot permits the stored failure, but not a new
 * failure from another server or tab. Storage errors stop creation.
 *
 * Rechecks cancellation and existing browser state after acquiring the lock.
 * Keep the lock until the browser call settles: subscribe cannot be aborted.
 */
export function createBrowserPushSubscription(options: {
  recovery?: BrowserPushRecovery;
  cancelled: () => boolean;
  existing: () => Promise<PushSubscription | null>;
  subscribe: () => Promise<PushSubscription>;
}): Promise<PushSubscription | null> {
  return navigator.locks.request(lockName, async () => {
    if (options.cancelled()) return null;
    const existing = await options.existing();
    if (options.cancelled()) return null;
    if (existing) return existing;

    const storage = window.localStorage;
    const blocked = storage.getItem(attemptKey);
    if (blocked !== null && blocked !== options.recovery?.attempt) {
      throw new BrowserPushRegistrationPaused();
    }

    const attempt = crypto.randomUUID();
    storage.setItem(attemptKey, attempt);
    if (storage.getItem(attemptKey) !== attempt) {
      throw new Error('Could not store the push registration attempt');
    }
    let subscription: PushSubscription;
    try {
      subscription = await options.subscribe();
    } catch (error) {
      // A user action captured while this call was pending must not bypass
      // its new failure. If storage fails, the pre-call stop still survives.
      try {
        storage.setItem(attemptKey, `failed:${attempt}`);
      } catch {
        // Retain the original browser error and the stored pending attempt.
      }
      throw error;
    }
    storage.removeItem(attemptKey);
    return subscription;
  });
}
