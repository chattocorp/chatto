<!--
@component

Keeps every eligible server's Web Push subscription current after the user
grants notification permission. SvelteKit registers the service worker in
production.

Only active for authenticated servers that have push notifications enabled.
Include this component once in the chat root.
-->
<script lang="ts">
  import {
    getPermission,
    getPushRegistrationTargets,
    refreshPushSubscriptions
  } from '$lib/notifications/pushNotifications';

  function refreshCurrentSubscriptions(): void {
    void refreshPushSubscriptions();
  }

  $effect(() => {
    const targets = getPushRegistrationTargets();
    if (getPermission() !== 'granted' || targets.length === 0) return;

    void refreshPushSubscriptions(targets);
  });

  $effect(() => {
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) return;

    navigator.serviceWorker.addEventListener('controllerchange', refreshCurrentSubscriptions);
    return () => {
      navigator.serviceWorker.removeEventListener('controllerchange', refreshCurrentSubscriptions);
    };
  });
</script>

<svelte:window onfocus={refreshCurrentSubscriptions} />
