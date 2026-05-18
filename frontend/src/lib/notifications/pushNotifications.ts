/**
 * Push notifications module.
 *
 * Manages Web Push subscriptions for receiving notifications even when
 * the browser is completely closed. Uses the Service Worker and Web Push API.
 */

import { graphql } from '$lib/gql';
import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';

// GraphQL mutations
const SubscribeToPushMutationDoc = graphql(`
  mutation SubscribeToPush($input: PushSubscriptionInput!) {
    subscribeToPush(input: $input)
  }
`);

const UnsubscribeFromPushMutationDoc = graphql(`
  mutation UnsubscribeFromPush($input: UnsubscribeFromPushInput!) {
    unsubscribeFromPush(input: $input)
  }
`);

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
 * Subscribe to push notifications.
 * This will:
 * 1. Request notification permission if needed
 * 2. Create a push subscription with the browser
 * 3. Send the subscription to the server
 *
 * @param vapidPublicKey - The server's VAPID public key
 * @returns true if subscription was successful
 */
export async function subscribe(vapidPublicKey: string): Promise<boolean> {
  if (!isSupported()) {
    console.warn('Push notifications not supported');
    return false;
  }

  // Request notification permission
  const permission = await Notification.requestPermission();
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
    // Create push subscription
    const subscription = await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidPublicKey)
    });

    // Extract subscription details
    const json = subscription.toJSON();
    if (!json.endpoint || !json.keys?.p256dh || !json.keys?.auth) {
      console.error('Invalid push subscription');
      return false;
    }

    // Send to server
    const result = await graphqlClientManager.originClient.client
      .mutation(SubscribeToPushMutationDoc, {
        input: {
          endpoint: json.endpoint,
          p256dh: json.keys.p256dh,
          auth: json.keys.auth,
          userAgent: navigator.userAgent
        }
      })
      .toPromise();

    if (result.error) {
      console.error('Failed to save push subscription:', result.error);
      // Unsubscribe from browser since server save failed
      await subscription.unsubscribe();
      return false;
    }

    return result.data?.subscribeToPush ?? false;
  } catch (error) {
    console.error('Failed to subscribe to push:', error);
    return false;
  }
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
    const result = await graphqlClientManager.originClient.client
      .mutation(UnsubscribeFromPushMutationDoc, {
        input: { endpoint: subscription.endpoint }
      })
      .toPromise();

    if (result.error) {
      console.error('Failed to remove push subscription from server:', result.error);
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

/**
 * Listen for push subscription changes from the service worker.
 * Call this on app mount to handle subscription expiration/revocation.
 */
export function onSubscriptionChange(callback: () => void): () => void {
  if (!('serviceWorker' in navigator)) {
    return () => {};
  }

  const handler = (event: MessageEvent) => {
    if (event.data?.type === 'push-subscription-changed') {
      callback();
    }
  };

  navigator.serviceWorker.addEventListener('message', handler);
  return () => navigator.serviceWorker.removeEventListener('message', handler);
}

/**
 * Listen for notification-click messages from the service worker.
 * The SW posts these instead of calling `WindowClient.navigate()` so the
 * SPA can route via `goto()` (client-side navigation, no full reload).
 */
export function onNotificationClick(callback: (url: string) => void): () => void {
  if (!('serviceWorker' in navigator)) {
    return () => {};
  }

  const handler = (event: MessageEvent) => {
    if (event.data?.type === 'notification-click' && typeof event.data.url === 'string') {
      callback(event.data.url);
    }
  };

  navigator.serviceWorker.addEventListener('message', handler);
  return () => navigator.serviceWorker.removeEventListener('message', handler);
}
