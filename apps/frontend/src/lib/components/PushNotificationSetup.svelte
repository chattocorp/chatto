<!--
@component

Refreshes the server-side Web Push subscription record when a browser already
has notification permission. SvelteKit registers the service worker in production.

Only active for authenticated servers that have push notifications enabled.
Include this component once in the chat root.
-->
<script lang="ts">
  import {
    getPushRegistrationTargets,
    refreshPushSubscriptions
  } from '$lib/notifications/pushNotifications';

  $effect(() => {
    const servers = getPushRegistrationTargets();
    if (servers.length === 0) return;

    void refreshPushSubscriptions(servers);
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;

    const refresh = () => void refreshPushSubscriptions();
    navigator.serviceWorker.addEventListener('controllerchange', refresh);
    return () => {
      navigator.serviceWorker.removeEventListener('controllerchange', refresh);
    };
  });
</script>
