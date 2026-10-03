/**
 * Push notifications module.
 *
 * Manages Web Push subscriptions for receiving notifications outside an open
 * Chatto page. Uses the Service Worker and Web Push API; platform delivery is
 * still treated as a notification trigger rather than authoritative app state.
 */

import { createPushNotificationAPI } from '$lib/api/pushNotifications';
import type { PushNotificationAPI } from '$lib/api/pushNotifications';
import { isBackendCapableOrigin } from '@chatto/client/util/runtimeOrigin';
import { serverConnectionManager, serverRegistry } from '$lib/client';
import { SvelteMap } from 'svelte/reactivity';
import {
  completePushRegistrationRefresh,
  enqueuePushRegistration,
  hasDurablePushCoordinationStorage,
  isPushRegistrationSuspended,
  onPushRegistrationRefresh,
  pendingPushRegistrationRefresh,
  requestPushRegistrationRefresh,
  shouldInvalidateCancelledPushRegistration,
  suspendPushRegistrationBeforeLeaving
} from './pushRegistrationCoordinator';
import { notificationPermission } from './pushPermission.svelte';

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
const pushScopePrefix = '/__chatto/push/';

/**
 * The production worker is one classic script, which also runs in browsers
 * without module service workers. The development server serves the worker
 * as an ES module.
 */
const serviceWorkerType: WorkerType = import.meta.env.DEV ? 'module' : 'classic';

/**
 * How long a successful save stays current before this page saves the
 * subscription again. The server expires a subscription 180 days after its
 * most recent save, so daily refreshes keep every device in use active.
 */
export const PUSH_REGISTRATION_REFRESH_INTERVAL_MS = 24 * 60 * 60 * 1000;

type SavedRegistration = {
  userId: string;
  vapidPublicKey: string;
  /** Browser push endpoint that the save stored on the server. */
  endpoint: string;
  savedAt: number;
};

/** Why a refresh saves a server's subscription. Logged for debugging. */
type RefreshReason =
  | 'requested-by-another-tab'
  | 'first-save-in-page'
  | 'account-changed'
  | 'vapid-key-changed'
  | 'refresh-interval-elapsed'
  | 'browser-subscription-changed';

/**
 * The most recent successful save for each server in this page. Reactive, so
 * settings can show when a server stores this device's subscription.
 */
const savedRegistrations = new SvelteMap<string, SavedRegistration>();
/**
 * Technical reason of the latest failed save for each server in this page,
 * recorded only while permission is granted. The browser's push service or
 * the server can reject a subscription. Reactive, so settings can explain the
 * failure.
 */
const failedRegistrations = new SvelteMap<string, { reason: string; failedAt: number }>();
/** Servers that a refresh in this page is saving now. */
const refreshesInFlight = new Set<string>();
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
 * Every server, including the origin server, uses a stable narrow scope. Each
 * scope binds a subscription to that server's own VAPID key, and push does not
 * depend on the root registration that owns the offline application shell.
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

  const server = serverRegistry.getServer(serverId);
  if (!server) return null;

  const scopeKey = await stableScopeKey(new URL(server.url).origin);
  const scope = `${pushScopePrefix}${scopeKey}/`;
  if (!options.create) {
    return await findExactServiceWorkerRegistration(
      new URL(scope, window.location.origin).toString()
    );
  }
  const registration = await navigator.serviceWorker.register(serviceWorkerScriptPath, {
    scope,
    type: serviceWorkerType
  });
  await waitForActiveWorker(registration);
  return registration;
}

/**
 * Earlier versions stored the origin server's subscription on the root
 * registration. Returns that registration so its subscription can be retired.
 */
async function findLegacyOriginRegistration(
  serverId: string
): Promise<ServiceWorkerRegistration | null> {
  if (!('serviceWorker' in navigator) || !serverRegistry.isOriginServer(serverId)) return null;
  return findExactServiceWorkerRegistration(window.location.origin + '/');
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

/** Looks up a server's subscriptions for a privacy boundary and preserves browser errors. */
async function getSubscriptionsForCleanup(serverId: string): Promise<PushSubscription[]> {
  const registrations = [
    await lookupServiceWorkerRegistration(serverId, { create: false }),
    await findLegacyOriginRegistration(serverId)
  ];
  const subscriptions: PushSubscription[] = [];
  for (const registration of registrations) {
    const subscription = await registration?.pushManager.getSubscription();
    if (subscription) subscriptions.push(subscription);
  }
  return subscriptions;
}

/**
 * Whether this page saved the current browser subscription for the account on
 * a server. Reactive.
 */
export function hasSavedPushRegistration(serverId: string, userId: string | null): boolean {
  const saved = savedRegistrations.get(serverId);
  return saved !== undefined && saved.userId === userId;
}

/**
 * The technical reason why the latest save of this page for a server failed,
 * or null. Reactive. The reason comes from the browser or the server and is
 * not translated.
 */
export function pushRegistrationFailure(serverId: string): string | null {
  return failedRegistrations.get(serverId)?.reason ?? null;
}

/** Formats a browser or transport error for a technical failure reason. */
function technicalReason(error: unknown): string {
  if (typeof error === 'object' && error !== null && 'message' in error) {
    const name = 'name' in error ? String(error.name) : 'Error';
    return `${name}: ${String(error.message)}`;
  }
  return String(error);
}

/**
 * Asks a server to send a test notification to the account's registered
 * devices. Rejects with the server's error, for example when the account
 * sends tests too often.
 */
export function sendTestNotification(serverId: string): Promise<boolean> {
  return pushAPI(serverId).sendTestNotification();
}

/**
 * The browser's notification permission, or null when this browser cannot
 * use Web Push. The value is reactive and follows changes made outside this
 * page.
 */
export function getPermission(): NotificationPermission | null {
  if (!isSupported()) {
    return null;
  }
  return notificationPermission.current;
}

/** Return authenticated servers that can accept this client's Web Push route. */
export function getPushRegistrationTargets(): PushRegistrationTarget[] {
  if (!isBrowserWebPushRuntime()) return [];

  return serverRegistry.servers.flatMap((server) => {
    const store = serverRegistry.tryGetStore(server.id);
    const info = store?.serverInfo;
    const userId = store?.accountId;
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
 * Ask for notification permission, then register every eligible server. This
 * is the only place where Chatto asks the browser for notification
 * permission. `PushNotificationSetup` calls it automatically, once, while the
 * page has user activation.
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
      await Notification.requestPermission();
    } catch (error) {
      console.error('Failed to request notification permission:', error);
    }
    permission = notificationPermission.refresh();
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
        return { ...target, registered: await ensureRegistered(target) };
      } catch (error) {
        console.error('Failed to enable push notifications:', error);
        return { ...target, registered: false };
      }
    })
  );
  return { permission, registrations };
}

/**
 * Returns why a server's subscription must be saved now, or null when the
 * save from this page is still current.
 */
async function refreshReason(
  target: PushRegistrationTarget,
  requestId: string | null,
  now: number
): Promise<RefreshReason | null> {
  // Chrome keeps a push service registration for every failed attempt and
  // refuses new ones once a profile holds about 1,000. Retrying a failed save
  // on every focus would fill that limit, so automatic retries wait as long
  // as a refresh. This also applies to a stored request from another tab: it
  // stays until a save succeeds. The cross-tab listener still handles a new
  // request at once, and `retryPushRegistration` retries at once.
  const failure = failedRegistrations.get(target.serverId);
  if (failure && now - failure.failedAt < PUSH_REGISTRATION_REFRESH_INTERVAL_MS) return null;
  if (requestId) return 'requested-by-another-tab';
  const saved = savedRegistrations.get(target.serverId);
  if (!saved) return 'first-save-in-page';
  if (saved.userId !== target.userId) return 'account-changed';
  if (saved.vapidPublicKey !== target.vapidPublicKey) return 'vapid-key-changed';
  if (now - saved.savedAt >= PUSH_REGISTRATION_REFRESH_INTERVAL_MS) {
    return 'refresh-interval-elapsed';
  }
  // The browser can drop or replace a subscription, for example when the
  // user revokes and grants permission again. A local lookup finds this
  // without a server request.
  if ((await currentBrowserEndpoint(target.serverId)) !== saved.endpoint) {
    return 'browser-subscription-changed';
  }
  return null;
}

async function currentBrowserEndpoint(serverId: string): Promise<string | null> {
  const registration = await getServiceWorkerRegistration(serverId, { create: false });
  try {
    return (await registration?.pushManager.getSubscription())?.endpoint ?? null;
  } catch {
    return null;
  }
}

/**
 * Saves one server's subscription again at once, also after a recent failure.
 * For an explicit user retry; automatic refreshes back off after failures.
 */
export async function retryPushRegistration(serverId: string): Promise<boolean> {
  if (getPermission() !== 'granted') return false;
  const target = getPushRegistrationTargets().find((candidate) => candidate.serverId === serverId);
  return target ? ensureRegistered(target, { manual: true }) : false;
}

/**
 * Saves the subscription of every eligible server whose save from this page
 * is missing or out of date. It never asks for permission: it does nothing
 * until the browser has granted notification permission. A server that
 * becomes eligible later, for example after it is added, is saved on the
 * next call.
 */
export async function refreshPushSubscriptions(): Promise<void> {
  if (enableAllInFlight) await enableAllInFlight;
  if (getPermission() !== 'granted') return;

  const now = Date.now();
  await Promise.all(
    getPushRegistrationTargets().map(async (target) => {
      const requestId = pendingPushRegistrationRefresh(target.serverId);
      // Startup, focus, and reactive updates often arrive together. One save
      // per server at a time is enough.
      if (refreshesInFlight.has(target.serverId)) return;
      refreshesInFlight.add(target.serverId);
      try {
        const reason = await refreshReason(target, requestId, now);
        if (!reason) return;

        console.debug('[push] Refreshing push subscription', {
          serverId: target.serverId,
          reason
        });
        const registered = await ensureRegistered(target);
        console.debug('[push] Push subscription refresh finished', {
          serverId: target.serverId,
          reason,
          registered
        });
        if (registered && requestId) completePushRegistrationRefresh(target.serverId, requestId);
      } catch (error) {
        console.error('Failed to refresh push notifications:', error);
      } finally {
        refreshesInFlight.delete(target.serverId);
      }
    })
  );
}

onPushRegistrationRefresh((serverId, requestId) => {
  const target = getPushRegistrationTargets().find((candidate) => candidate.serverId === serverId);
  if (!target) return;
  void ensureRegistered(target).then((registered) => {
    if (registered) completePushRegistrationRefresh(serverId, requestId);
  });
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
 * The failure of the browser's push service in this page session, or null.
 * The browser reports "push service error" when it cannot register with its
 * push service, for example when the profile holds too many registrations.
 * After such a failure, automatic saves create no new browser subscriptions
 * for any server until an explicit retry succeeds or the page reloads; each
 * further attempt could leave another registration behind.
 */
let pushServiceFailure: string | null = null;
/** Serializes browser subscribe calls so that one failure stops the others. */
let browserSubscribeTail: Promise<unknown> = Promise.resolve();

function isPushServiceFailure(reason: string): boolean {
  return /push service error/i.test(reason);
}

/** Creates a browser subscription, one at a time across all servers. */
function subscribeInBrowser(
  registration: ServiceWorkerRegistration,
  applicationServerKey: Uint8Array<ArrayBuffer>,
  manual: boolean
): Promise<PushSubscription> {
  const operation = browserSubscribeTail.then(async () => {
    if (pushServiceFailure && !manual) throw new Error(pushServiceFailure);
    try {
      const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey
      });
      pushServiceFailure = null;
      return subscription;
    } catch (error) {
      const reason = technicalReason(error);
      if (isPushServiceFailure(reason)) pushServiceFailure = reason;
      throw error;
    }
  });
  browserSubscribeTail = operation.catch(() => undefined);
  return operation;
}

/**
 * Ensure the current browser push subscription is stored on the server.
 * Browser/OS permission is the user-facing source of truth. This never asks
 * for permission; without a granted permission it returns false. Saves for
 * one server run one after another.
 *
 * @param options.manual - An explicit user retry. It may create a browser
 *   subscription even after the browser's push service failed in this page.
 */
export async function ensureRegistered(
  target: PushRegistrationTarget,
  options: { manual?: boolean } = {}
): Promise<boolean> {
  const saved = { endpoint: null as string | null, failure: 'Unknown failure' };
  let registered = false;
  try {
    registered = await enqueuePushRegistration(target.serverId, (signal) =>
      ensureRegisteredOnce(
        target.serverId,
        target.vapidPublicKey,
        signal,
        options.manual === true,
        {
          saved: (endpoint) => {
            saved.endpoint = endpoint;
          },
          failed: (reason) => {
            saved.failure = reason;
          }
        }
      )
    );
  } catch (error) {
    saved.failure = technicalReason(error);
    throw error;
  } finally {
    if (registered && saved.endpoint) {
      savedRegistrations.set(target.serverId, {
        userId: target.userId,
        vapidPublicKey: target.vapidPublicKey,
        endpoint: saved.endpoint,
        savedAt: Date.now()
      });
      failedRegistrations.delete(target.serverId);
    } else if (!isPushRegistrationSuspended(target.serverId) && getPermission() === 'granted') {
      // Leaving the server and missing permission are not failures.
      failedRegistrations.set(target.serverId, {
        reason: saved.failure,
        failedAt: Date.now()
      });
    }
  }
  return registered;
}

async function ensureRegisteredOnce(
  serverId: string,
  vapidPublicKey: string,
  signal: AbortSignal,
  manual: boolean,
  report: { saved: (endpoint: string) => void; failed: (reason: string) => void }
): Promise<boolean> {
  if (!isSupported()) {
    console.warn('Push notifications not supported');
    return false;
  }

  if (Notification.permission !== 'granted') {
    return false;
  }

  let registration: ServiceWorkerRegistration | null;
  try {
    registration = await lookupServiceWorkerRegistration(serverId, { create: true });
  } catch (error) {
    console.error('Failed to register the push service worker:', error);
    report.failed(technicalReason(error));
    return false;
  }
  if (isPushRegistrationSuspended(serverId, signal)) return false;
  if (!registration) {
    console.error('No service worker registration');
    report.failed('No service worker registration');
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
      subscription = await subscribeInBrowser(registration, applicationServerKey, manual);
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
      report.failed('Invalid push subscription');
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
      report.failed('The server did not save the subscription');
      if (createdSubscription) {
        await invalidateSubscription(serverId, subscription);
      }
      return false;
    }

    report.saved(subscription.endpoint);
    await retireLegacyOriginSubscription(serverId, subscription.endpoint);
    return true;
  } catch (error) {
    console.error('Failed to subscribe to push:', error);
    report.failed(technicalReason(error));
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
 * Removes the origin server's subscription from the root registration that
 * earlier versions used, after the scoped subscription is saved. Best-effort:
 * the push service also reports the retired endpoint as gone, and the server
 * then removes it.
 */
async function retireLegacyOriginSubscription(
  serverId: string,
  currentEndpoint: string
): Promise<void> {
  try {
    const registration = await findLegacyOriginRegistration(serverId);
    const legacy = await registration?.pushManager.getSubscription();
    if (!legacy || legacy.endpoint === currentEndpoint) return;
    await invalidateSubscription(serverId, legacy);
  } catch {
    // A later save tries again.
  }
}

/** Establishes a local or server-side delivery fence before navigation. */
export function unsubscribeBeforeLeaving(serverId: string): Promise<void> {
  savedRegistrations.delete(serverId);
  failedRegistrations.delete(serverId);
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
  const subscriptions = await getSubscriptionsForCleanup(serverId);
  if (subscriptions.length === 0) {
    return { removedFromBrowser: true, removeFromServer: Promise.resolve(true) };
  }
  if (!shouldInvalidateCancelledPushRegistration(serverId)) {
    return { removedFromBrowser: true, removeFromServer: Promise.resolve(true) };
  }

  let removedFromBrowser = true;
  for (const subscription of subscriptions) {
    try {
      removedFromBrowser = (await subscription.unsubscribe()) && removedFromBrowser;
    } catch (error) {
      console.error('Failed to unsubscribe from browser push:', error);
      removedFromBrowser = false;
    }
  }

  if (!shouldInvalidateCancelledPushRegistration(serverId)) {
    return { removedFromBrowser, removeFromServer: Promise.resolve(true) };
  }

  const removeFromServer = Promise.all(
    subscriptions.map((subscription) =>
      api.unsubscribe(subscription.endpoint).then(
        (removed) => {
          if (!removed) console.error('Failed to remove push subscription from server');
          return removed;
        },
        (error) => {
          console.error('Failed to remove push subscription from server:', error);
          return false;
        }
      )
    )
  ).then((results) => results.every(Boolean));
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
