<!--
@component

Handles automatic Web Push re-subscription when the browser renews or
invalidates subscriptions. SvelteKit registers the service worker in production.

Only active when push notifications are enabled in the server config.
Include this component once in the authenticated layout.
-->
<script lang="ts">
  import {
    onSubscriptionChange,
    subscribe,
    isSubscribed
  } from '$lib/notifications/pushNotifications';
  import { serverRegistry } from '$lib/state/server/registry.svelte';

  const originId = serverRegistry.originServer?.id ?? '';
  const originServerInfo = originId ? serverRegistry.getStore(originId).serverInfo : undefined;

  $effect(() => {
    if (!originServerInfo?.pushNotificationsEnabled) return;
    if (!('serviceWorker' in navigator)) return;

    // Listen for subscription changes (e.g., when browser renews subscription)
    // and attempt automatic re-subscription
    return onSubscriptionChange(async () => {
      console.log('Push subscription changed, attempting re-subscription...');

      // Check if we're still subscribed (browser may have renewed automatically)
      const stillSubscribed = await isSubscribed();
      if (stillSubscribed) {
        console.log('Push subscription still valid');
        return;
      }

      // Attempt to re-subscribe
      const vapidKey = originServerInfo.vapidPublicKey;
      if (!vapidKey) {
        console.warn('Cannot re-subscribe: VAPID key not available');
        return;
      }

      const success = await subscribe(vapidKey);
      if (success) {
        console.log('Push subscription renewed successfully');
      } else {
        console.warn('Failed to renew push subscription');
      }
    });
  });
</script>
