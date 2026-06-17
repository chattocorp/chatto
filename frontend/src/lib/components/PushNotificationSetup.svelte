<!--
@component

Refreshes the server-side Web Push subscription record when a browser already
has notification permission. SvelteKit registers the service worker in production.

Only active when push notifications are enabled in the server config.
Include this component once in the authenticated layout.
-->
<script lang="ts">
  import { ensureRegistered } from '$lib/notifications/pushNotifications';
  import { serverRegistry } from '$lib/state/server/registry.svelte';

  const originId = serverRegistry.originServer?.id ?? '';
  const originServerInfo = originId ? serverRegistry.getStore(originId).serverInfo : undefined;

  $effect(() => {
    if (!originServerInfo?.pushNotificationsEnabled) return;
    if (!originServerInfo.vapidPublicKey) return;

    void ensureRegistered(originServerInfo.vapidPublicKey, { prompt: false });
  });
</script>
