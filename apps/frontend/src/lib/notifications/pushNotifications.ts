/**
 * Push notifications module.
 *
 * Manages Web Push subscriptions for receiving notifications outside an open
 * Chatto page. Uses the Service Worker and Web Push API; platform delivery is
 * still treated as a notification trigger rather than authoritative app state.
 */

import { createPushNotificationAPI } from '$lib/api-client/pushNotifications';
import type { PushNotificationAPI } from '$lib/api-client/pushNotifications';
import { isBackendCapableOrigin } from '$lib/runtimeOrigin';
import {
  NOTIFICATION_CLICK_ACK_MESSAGE_TYPE,
  NOTIFICATION_CLICK_MESSAGE_TYPE
} from '$lib/pwa/notificationClick.worker';
import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import {
  completePushRegistrationRefresh,
  enqueuePushRegistration,
  hasDurablePushCoordinationStorage,
  isPushRegistrationSuspended,
  onPushRegistrationRefresh,
  pendingPushRegistrationRefresh,
  requestPushRegistrationRefresh,
  resumePushRegistration,
  shouldInvalidateCancelledPushRegistration,
  suspendPushRegistration,
  suspendPushRegistrationBeforeLeaving
} from './pushRegistrationCoordinator';

type EnsureRegisteredOptions = {
  prompt: boolean;
};

export type PushRegistrationTarget = {
  serverId: string;
  userId: string;
  vapidPublicKey: string;
};

export type PushRegistrationResult = PushRegistrationTarget & {
  registered: boolean;
};

export type EnablePushOnAllServersResult = {
  permission: NotificationPermission | null;
  registrations: PushRegistrationResult[];
};

export type PushCapability = 'supported' | 'ios_home_screen_required' | 'unsupported';

type StandaloneNavigator = Navigator & {
  standalone?: boolean;
};

const serviceWorkerScriptPath = '/service-worker.js';
const remotePushScopePrefix = '/__chatto/push/';
let enableAllInFlight: Promise<EnablePushOnAllServersResult> | null = null;

function isIosBrowserContext(): boolean {
  if (typeof navigator === 'undefined') return false;

  const platform = navigator.platform;
  const userAgent = navigator.userAgent;
  const touchCapableMac = platform === 'MacIntel' && navigator.maxTouchPoints > 1;
  return /iPad|iPhone|iPod/.test(userAgent) || touchCapableMac;
}

function isStandaloneDisplayMode(): boolean {
  if (typeof window === 'undefined') return false;

  return (
    window.matchMedia?.('(display-mode: standalone)').matches === true ||
    (navigator as StandaloneNavigator).standalone === true
  );
}

export function getPushCapability(): PushCapability {
  if (!isBrowserWebPushRuntime()) return 'unsupported';

  if (
    typeof window === 'undefined' ||
    !('serviceWorker' in navigator) ||
    !('locks' in navigator) ||
    !hasDurablePushCoordinationStorage()
  ) {
    return 'unsupported';
  }

  if (isIosBrowserContext() && !isStandaloneDisplayMode()) {
    return 'ios_home_screen_required';
  }

  if ('PushManager' in window && 'Notification' in window) {
    return 'supported';
  }

  return 'unsupported';
}

/**
 * Check if push notifications are supported in this browser.
 * Requires Service Worker, Push, Web Locks, and durable local storage support.
 */
export function isSupported(): boolean {
  return getPushCapability() === 'supported';
}

/** Browser Web Push belongs to HTTP(S) PWA origins, never native app origins. */
export function isBrowserWebPushRuntime(): boolean {
  return typeof window !== 'undefined' && isBackendCapableOrigin(window.location);
}

/**
 * Get the service worker registration that owns a server's push subscription.
 * The origin server retains the historical root registration. Remote servers
 * use stable narrow scopes so each can bind a subscription to its own VAPID
 * key without changing which worker controls the application page.
 */
async function getServiceWorkerRegistration(
  serverId: string,
  options: { create: boolean }
): Promise<ServiceWorkerRegistration | null> {
  try {
    return await lookupServiceWorkerRegistration(serverId, options);
  } catch {
    return null;
  }
}

async function lookupServiceWorkerRegistration(
  serverId: string,
  options: { create: boolean }
): Promise<ServiceWorkerRegistration | null> {
  if (!('serviceWorker' in navigator)) {
    return null;
  }

  if (serverRegistry.isOriginServer(serverId)) {
    if (options.create) return await navigator.serviceWorker.ready;
    return await findExactServiceWorkerRegistration(window.location.origin + '/');
  }

  const server = serverRegistry.getServer(serverId);
  if (!server) return null;

  const scopeKey = await stableScopeKey(new URL(server.url).origin);
  const scope = `${remotePushScopePrefix}${scopeKey}/`;
  if (!options.create) {
    return await findExactServiceWorkerRegistration(
      new URL(scope, window.location.origin).toString()
    );
  }
  const registration = await navigator.serviceWorker.register(serviceWorkerScriptPath, {
    scope,
    type: 'module'
  });
  await waitForActiveWorker(registration);
  return registration;
}

async function findExactServiceWorkerRegistration(
  expectedScope: string
): Promise<ServiceWorkerRegistration | null> {
  const registrations = await navigator.serviceWorker.getRegistrations();
  return registrations.find((registration) => registration.scope === expectedScope) ?? null;
}

async function stableScopeKey(value: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value));
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
}

async function waitForActiveWorker(registration: ServiceWorkerRegistration): Promise<void> {
  if (registration.active) return;

  const worker = registration.installing ?? registration.waiting;
  if (!worker) throw new Error('Service worker did not install');

  await new Promise<void>((resolve, reject) => {
    const timeout = window.setTimeout(() => {
      cleanup();
      reject(new Error('Service worker activation timed out'));
    }, 15_000);
    const onStateChange = () => {
      if (worker.state === 'activated') {
        cleanup();
        resolve();
      } else if (worker.state === 'redundant') {
        cleanup();
        reject(new Error('Service worker became redundant'));
      }
    };
    const cleanup = () => {
      window.clearTimeout(timeout);
      worker.removeEventListener('statechange', onStateChange);
    };

    worker.addEventListener('statechange', onStateChange);
    onStateChange();
  });
}

/**
 * Get the current push subscription, if any.
 */
export async function getSubscription(serverId: string): Promise<PushSubscription | null> {
  const registration = await getServiceWorkerRegistration(serverId, { create: false });
  if (!registration) {
    return null;
  }

  try {
    return await registration.pushManager.getSubscription();
  } catch {
    return null;
  }
}

/** Looks up a subscription for a privacy boundary and preserves browser errors. */
async function getSubscriptionForCleanup(serverId: string): Promise<PushSubscription | null> {
  const registration = await lookupServiceWorkerRegistration(serverId, { create: false });
  if (!registration) return null;
  return registration.pushManager.getSubscription();
}

/**
 * Check if push notifications are currently subscribed.
 */
export async function isSubscribed(serverId: string): Promise<boolean> {
  const subscription = await getSubscription(serverId);
  return subscription !== null;
}

/** Sends a real Web Push notification to this browser's current subscription. */
export async function sendTestNotification(serverId: string): Promise<boolean> {
  return pushAPI(serverId).sendTestNotification();
}

export function getPermission(): NotificationPermission | null {
  if (!isSupported()) {
    return null;
  }
  return Notification.permission;
}

/** Return authenticated servers that can accept this client's Web Push route. */
export function getPushRegistrationTargets(): PushRegistrationTarget[] {
  if (!isBrowserWebPushRuntime()) return [];

  return serverRegistry.servers.flatMap((server) => {
    const store = serverRegistry.tryGetStore(server.id);
    const info = store?.serverInfo;
    const userId = store?.currentUser.user?.id;
    if (
      !store?.isAuthenticated ||
      !userId ||
      !info?.pushNotificationsEnabled ||
      !info.vapidPublicKey
    ) {
      return [];
    }
    return [{ serverId: server.id, userId, vapidPublicKey: info.vapidPublicKey }];
  });
}

/**
 * Ask for notification permission directly from a user interaction, then
 * register every eligible server.
 *
 * The permission request must happen before registration enters its async
 * coordination queue. Some browsers require the call itself to retain the
 * current user activation.
 *
 * A failure on one server does not prevent the other servers from registering.
 */
export function enablePushOnAllServers(): Promise<EnablePushOnAllServersResult> {
  if (enableAllInFlight) return enableAllInFlight;

  const operation = enablePushOnAllServersOnce();
  enableAllInFlight = operation;
  const clear = () => {
    if (enableAllInFlight === operation) enableAllInFlight = null;
  };
  void operation.then(clear, clear);
  return operation;
}

async function enablePushOnAllServersOnce(): Promise<EnablePushOnAllServersResult> {
  if (getPushRegistrationTargets().length === 0) {
    return { permission: getPermission(), registrations: [] };
  }

  let permission = getPermission();
  if (permission === 'default') {
    try {
      permission = await Notification.requestPermission();
    } catch (error) {
      console.error('Failed to request notification permission:', error);
      permission = getPermission();
    }
  }

  // The server list can change while the browser or operating system displays
  // its permission prompt. Register only the current authenticated accounts.
  const targets = getPushRegistrationTargets();
  if (permission !== 'granted') {
    return {
      permission,
      registrations: targets.map((target) => ({ ...target, registered: false }))
    };
  }

  const registrations = await Promise.all(
    targets.map(async (target): Promise<PushRegistrationResult> => {
      try {
        return {
          ...target,
          registered: await ensureRegistered(target.serverId, target.vapidPublicKey, {
            prompt: true
          })
        };
      } catch (error) {
        console.error('Failed to enable push notifications:', error);
        return {
          ...target,
          registered: false
        };
      }
    })
  );
  return { permission, registrations };
}

/** Refresh every configured server after permission or worker lifecycle changes. */
export async function refreshPushSubscriptions(
  targets?: PushRegistrationTarget[]
): Promise<void> {
  if (enableAllInFlight) {
    await enableAllInFlight;
    // Eligibility can change while explicit activation is in progress. Read
    // the registry again so the change is not lost behind the shared request.
    targets = getPushRegistrationTargets();
  }
  if (getPermission() !== 'granted') return;

  await Promise.all(
    (targets ?? getPushRegistrationTargets()).map(async ({ serverId, vapidPublicKey }) => {
      try {
        const requestId = pendingPushRegistrationRefresh(serverId);
        const registered = await ensureRegistered(serverId, vapidPublicKey, { prompt: false });
        if (registered && requestId) completePushRegistrationRefresh(serverId, requestId);
      } catch (error) {
        console.error('Failed to refresh push notifications:', error);
      }
    })
  );
}

onPushRegistrationRefresh((serverId, requestId) => {
  const target = getPushRegistrationTargets().find((candidate) => candidate.serverId === serverId);
  if (!target) return;
  void ensureRegistered(target.serverId, target.vapidPublicKey, { prompt: false }).then(
    (registered) => {
      if (registered) completePushRegistrationRefresh(serverId, requestId);
    }
  );
});

/** Creates a 128-bit capability that identifies one server save generation. */
function createPushCleanupToken(): string {
  const bytes = globalThis.crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

/**
 * Convert base64url string to Uint8Array (for VAPID key).
 */
function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');

  const rawData = window.atob(base64);
  const buffer = new ArrayBuffer(rawData.length);
  const outputArray = new Uint8Array(buffer);

  for (let i = 0; i < rawData.length; ++i) {
    outputArray[i] = rawData.charCodeAt(i);
  }
  return outputArray;
}

/**
 * Ensure the current browser push subscription is stored on the server.
 * Browser/OS permission is the user-facing source of truth. When permission is
 * already granted, this refreshes the server-side delivery cache without
 * prompting the user.
 */
export function ensureRegistered(
  serverId: string,
  vapidPublicKey: string,
  options: EnsureRegisteredOptions
): Promise<boolean> {
  if (options.prompt) resumePushRegistration(serverId);
  return enqueuePushRegistration(serverId, (signal) =>
    ensureRegisteredOnce(serverId, vapidPublicKey, options, signal)
  );
}

async function ensureRegisteredOnce(
  serverId: string,
  vapidPublicKey: string,
  options: EnsureRegisteredOptions,
  signal: AbortSignal
): Promise<boolean> {
  if (!isSupported()) {
    console.warn('Push notifications not supported');
    return false;
  }

  let permission = Notification.permission;
  if (permission === 'default') {
    if (!options.prompt) {
      return false;
    }
    permission = await Notification.requestPermission();
    if (isPushRegistrationSuspended(serverId, signal)) return false;
  }

  if (permission !== 'granted') {
    console.warn('Notification permission denied');
    return false;
  }

  const registration = await getServiceWorkerRegistration(serverId, { create: true });
  if (isPushRegistrationSuspended(serverId, signal)) return false;
  if (!registration) {
    console.error('No service worker registration');
    return false;
  }

  let subscription: PushSubscription | null = null;
  let subscriptionAuth: string | null = null;
  let cleanupToken: string | null = null;
  let createdSubscription = false;
  let api: PushNotificationAPI | null = null;

  try {
    const applicationServerKey = urlBase64ToUint8Array(vapidPublicKey);
    subscription = await registration.pushManager.getSubscription();
    if (isPushRegistrationSuspended(serverId, signal)) {
      if (subscription && shouldInvalidateCancelledPushRegistration(serverId)) {
        await invalidateSubscription(serverId, subscription);
      }
      return false;
    }

    if (
      subscription?.options.applicationServerKey &&
      !arrayBuffersEqual(subscription.options.applicationServerKey, applicationServerKey)
    ) {
      await subscription.unsubscribe();
      void pushAPI(serverId)
        .unsubscribe(subscription.endpoint)
        .catch(() => false);
      subscription = null;
      if (isPushRegistrationSuspended(serverId, signal)) return false;
    }

    if (!subscription) {
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey
      });
      createdSubscription = true;
      if (isPushRegistrationSuspended(serverId, signal)) {
        if (shouldInvalidateCancelledPushRegistration(serverId)) {
          await invalidateSubscription(serverId, subscription);
        }
        return false;
      }
    }

    // Extract subscription details
    const json = subscription.toJSON();
    if (!json.endpoint || !json.keys?.p256dh || !json.keys?.auth) {
      console.error('Invalid push subscription');
      return false;
    }
    subscriptionAuth = json.keys.auth;
    cleanupToken = createPushCleanupToken();

    const input = {
      endpoint: json.endpoint,
      p256dh: json.keys.p256dh,
      auth: json.keys.auth,
      clientHost: window.location.host,
      cleanupToken,
      userAgent: navigator.userAgent
    };
    api = pushAPI(serverId);
    if (isPushRegistrationSuspended(serverId, signal)) {
      if (shouldInvalidateCancelledPushRegistration(serverId)) {
        await invalidateSubscription(serverId, subscription);
      }
      return false;
    }
    const saved = await api.subscribe(input, { signal });

    if (isPushRegistrationSuspended(serverId, signal)) {
      if (shouldInvalidateCancelledPushRegistration(serverId)) {
        await invalidateSubscription(serverId, subscription);
      } else {
        await removeStaleServerSubscription(
          serverId,
          api,
          subscription.endpoint,
          input.auth,
          input.cleanupToken
        );
      }
      return false;
    }

    if (!saved.subscribed) {
      console.error('Failed to save push subscription');
      if (createdSubscription) {
        await invalidateSubscription(serverId, subscription);
      }
      return false;
    }

    return true;
  } catch (error) {
    console.error('Failed to subscribe to push:', error);
    const cancelled = isPushRegistrationSuspended(serverId, signal);
    const activeSuspension = shouldInvalidateCancelledPushRegistration(serverId);
    if (subscription) {
      if (activeSuspension || (!cancelled && createdSubscription)) {
        await invalidateSubscription(serverId, subscription);
      } else if (cancelled && api) {
        if (subscriptionAuth && cleanupToken) {
          await removeStaleServerSubscription(
            serverId,
            api,
            subscription.endpoint,
            subscriptionAuth,
            cleanupToken
          );
        }
      }
    }
    return false;
  }
}

/**
 * Removes only the exact subscription created by the cancelled save. Cleanup
 * uses the browser Push API auth secret plus a random per-save token rather than
 * the current account session. Cookie changes, revoked bearer tokens, and later
 * saves of the same browser subscription therefore cannot redirect cleanup.
 */
async function removeStaleServerSubscription(
  serverId: string,
  api: PushNotificationAPI,
  endpoint: string,
  auth: string,
  cleanupToken: string
): Promise<void> {
  try {
    await api.deleteByCapability(endpoint, auth, cleanupToken);
  } catch {
    // Browser invalidation during suspension still makes the endpoint unusable.
  } finally {
    requestPushRegistrationRefresh(serverId);
  }
}

async function invalidateSubscription(
  serverId: string,
  subscription: PushSubscription
): Promise<void> {
  try {
    await subscription.unsubscribe();
  } catch {
    // The subscription is already unusable from this client's perspective.
  }
  try {
    void pushAPI(serverId)
      .unsubscribe(subscription.endpoint)
      .catch(() => undefined);
  } catch {
    // Constructing the API is also best-effort after local invalidation.
  }
}

/**
 * Subscribe to push notifications after an explicit user action.
 *
 * @param vapidPublicKey - The server's VAPID public key
 * @returns true if subscription was successful
 */
export async function subscribe(serverId: string, vapidPublicKey: string): Promise<boolean> {
  return ensureRegistered(serverId, vapidPublicKey, { prompt: true });
}

/**
 * Unsubscribe from push notifications.
 * This will:
 * 1. Unsubscribe from the browser's push service
 * 2. Remove the subscription from the server
 *
 * @returns true if unsubscription was successful
 */
export async function unsubscribe(serverId: string): Promise<boolean> {
  let result = false;
  await suspendPushRegistration(serverId, async () => {
    const cleanup = await beginUnsubscribe(serverId);
    result = cleanup.removedFromBrowser && (await cleanup.removeFromServer);
  });
  return result;
}

/** Establishes a local or server-side delivery fence before navigation. */
export function unsubscribeBeforeLeaving(serverId: string): Promise<void> {
  return suspendPushRegistrationBeforeLeaving(serverId, async () => {
    const cleanup = await beginUnsubscribe(serverId);
    if (cleanup.removedFromBrowser) {
      void cleanup.removeFromServer;
      return;
    }
    if (!(await cleanup.removeFromServer)) {
      throw new Error('Push delivery could not be disabled before leaving the server');
    }
  });
}

async function beginUnsubscribe(serverId: string): Promise<{
  removedFromBrowser: boolean;
  removeFromServer: Promise<boolean>;
}> {
  const api = pushAPI(serverId);
  const subscription = await getSubscriptionForCleanup(serverId);
  if (!subscription) {
    return { removedFromBrowser: true, removeFromServer: Promise.resolve(true) };
  }
  if (!shouldInvalidateCancelledPushRegistration(serverId)) {
    return { removedFromBrowser: true, removeFromServer: Promise.resolve(true) };
  }

  let removedFromBrowser = false;
  try {
    removedFromBrowser = await subscription.unsubscribe();
  } catch (error) {
    console.error('Failed to unsubscribe from browser push:', error);
  }

  if (!shouldInvalidateCancelledPushRegistration(serverId)) {
    return { removedFromBrowser, removeFromServer: Promise.resolve(true) };
  }

  const removeFromServer = api.unsubscribe(subscription.endpoint).then(
    (removed) => {
      if (!removed) console.error('Failed to remove push subscription from server');
      return removed;
    },
    (error) => {
      console.error('Failed to remove push subscription from server:', error);
      return false;
    }
  );
  return { removedFromBrowser, removeFromServer };
}

function arrayBuffersEqual(left: ArrayBuffer, right: Uint8Array<ArrayBuffer>): boolean {
  const leftBytes = new Uint8Array(left);
  if (leftBytes.length !== right.length) return false;
  return leftBytes.every((byte, index) => byte === right[index]);
}

function pushAPI(serverId: string) {
  return serverConnectionManager.getClient(serverId).getAPI(createPushNotificationAPI);
}

/**
 * Listen for notification-click messages from the service worker.
 * The SW posts these instead of calling `WindowClient.navigate()` so the
 * SPA can route via `goto()` (client-side navigation, no full reload).
 */
export function onNotificationClick(callback: (url: string) => void | Promise<void>): () => void {
  if (!('serviceWorker' in navigator)) {
    return () => {};
  }

  const handler = (event: MessageEvent) => {
    if (
      event.data?.type === NOTIFICATION_CLICK_MESSAGE_TYPE &&
      typeof event.data.url === 'string'
    ) {
      const responsePort = event.ports[0];
      void (async () => {
        try {
          await callback(event.data.url);
          responsePort?.postMessage({ type: NOTIFICATION_CLICK_ACK_MESSAGE_TYPE });
        } catch {
          // Leave the service worker unacknowledged so it can fall back to
          // WindowClient.navigate() after its timeout.
        }
      })();
    }
  };

  navigator.serviceWorker.addEventListener('message', handler);
  return () => navigator.serviceWorker.removeEventListener('message', handler);
}
