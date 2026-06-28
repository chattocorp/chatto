/**
 * Push notifications module.
 *
 * Manages Web Push subscriptions for receiving notifications even when
 * the browser is completely closed. Uses the Service Worker and Web Push API.
 */

import { createPushNotificationAPI } from '$lib/api/pushNotifications';
import {
  NOTIFICATION_CLICK_ACK_MESSAGE_TYPE,
  NOTIFICATION_CLICK_MESSAGE_TYPE
} from '$lib/pwa/notificationClick.worker';
import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';

type EnsureRegisteredOptions = {
  prompt: boolean;
};

/**
 * Check if push notifications are supported in this browser.
 * Requires Service Worker and Push API support.
 */
export function isSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    'serviceWorker' in navigator &&
    'PushManager' in window &&
    'Notification' in window
  );
}

/**
 * Get the current service worker registration.
 */
async function getServiceWorkerRegistration(): Promise<ServiceWorkerRegistration | null> {
  if (!('serviceWorker' in navigator)) {
    return null;
  }

  try {
    return await navigator.serviceWorker.ready;
  } catch {
    return null;
  }
}

/**
 * Get the current push subscription, if any.
 */
export async function getSubscription(): Promise<PushSubscription | null> {
  const registration = await getServiceWorkerRegistration();
  if (!registration) {
    return null;
  }

  try {
    return await registration.pushManager.getSubscription();
  } catch {
    return null;
  }
}

/**
 * Check if push notifications are currently subscribed.
 */
export async function isSubscribed(): Promise<boolean> {
  const subscription = await getSubscription();
  return subscription !== null;
}

export function getPermission(): NotificationPermission | null {
  if (!isSupported()) {
    return null;
  }
  return Notification.permission;
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
export async function ensureRegistered(
  vapidPublicKey: string,
  options: EnsureRegisteredOptions
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
  }

  if (permission !== 'granted') {
    console.warn('Notification permission denied');
    return false;
  }

  const registration = await getServiceWorkerRegistration();
  if (!registration) {
    console.error('No service worker registration');
    return false;
  }

  try {
    let subscription = await registration.pushManager.getSubscription();
    let createdSubscription = false;

    if (!subscription) {
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(vapidPublicKey)
      });
      createdSubscription = true;
    }

    // Extract subscription details
    const json = subscription.toJSON();
    if (!json.endpoint || !json.keys?.p256dh || !json.keys?.auth) {
      console.error('Invalid push subscription');
      return false;
    }

    const saved = await originPushAPI().subscribe({
      endpoint: json.endpoint,
      p256dh: json.keys.p256dh,
      auth: json.keys.auth,
      userAgent: navigator.userAgent
    });

    if (!saved) {
      console.error('Failed to save push subscription');
      if (createdSubscription) {
        await subscription.unsubscribe();
      }
      return false;
    }

    return true;
  } catch (error) {
    console.error('Failed to subscribe to push:', error);
    return false;
  }
}

/**
 * Subscribe to push notifications after an explicit user action.
 *
 * @param vapidPublicKey - The server's VAPID public key
 * @returns true if subscription was successful
 */
export async function subscribe(vapidPublicKey: string): Promise<boolean> {
  return ensureRegistered(vapidPublicKey, { prompt: true });
}

/**
 * Unsubscribe from push notifications.
 * This will:
 * 1. Remove the subscription from the server
 * 2. Unsubscribe from the browser's push service
 *
 * @returns true if unsubscription was successful
 */
export async function unsubscribe(): Promise<boolean> {
  const subscription = await getSubscription();
  if (!subscription) {
    // Already unsubscribed
    return true;
  }

  try {
    // Remove from server first
    const removed = await originPushAPI().unsubscribe(subscription.endpoint);

    if (!removed) {
      console.error('Failed to remove push subscription from server');
      // Continue to unsubscribe from browser anyway
    }

    // Unsubscribe from browser
    await subscription.unsubscribe();
    return true;
  } catch (error) {
    console.error('Failed to unsubscribe from push:', error);
    return false;
  }
}

function originPushAPI() {
  const origin = serverConnectionManager.originClient;
  return createPushNotificationAPI({
    baseUrl: origin.connectBaseUrl,
    bearerToken: origin.bearerToken
  });
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
