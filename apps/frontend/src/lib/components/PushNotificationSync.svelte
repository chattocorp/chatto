<!-- @component Closes handled OS notifications for one authenticated account. -->
<script lang="ts">
  import { untrack } from 'svelte';
  import type { NotificationStore } from '$lib/state/server/notifications.svelte';
  import { cleanupPushNotifications } from '$lib/notifications/pushNotificationCleanup';
  import { listenForAppBadgeRefresh } from '$lib/notifications/appBadge';

  let {
    serverUrl,
    recipientId,
    notifications
  }: {
    serverUrl: string;
    recipientId: string;
    notifications: NotificationStore;
  } = $props();

  let disposed = false;
  let running = false;
  let requested = false;

  // One pass at a time; updates during browser/server reads request another pass.
  async function sync() {
    requested = true;
    if (running || disposed) return;
    running = true;
    try {
      while (requested && !disposed) {
        requested = false;
        const store = notifications;
        const revision = store.pushRevision;
        const user = recipientId;
        const url = serverUrl;
        await cleanupPushNotifications(
          { serverOrigin: new URL(url).origin, recipientId: user },
          (ids) => store.handledPushNotificationIds(ids),
          () =>
            !disposed &&
            store === notifications &&
            revision === store.pushRevision &&
            user === recipientId &&
            url === serverUrl
        );
      }
    } finally {
      running = false;
    }
  }

  $effect(() => {
    void notifications.pushRevision;
    void serverUrl;
    void recipientId;
    untrack(() => void sync());
  });

  $effect(() => {
    disposed = false;
    const stop = listenForAppBadgeRefresh(() => void sync());
    return () => {
      disposed = true;
      stop();
    };
  });
</script>

<svelte:window onfocus={() => void sync()} ononline={() => void sync()} />
<svelte:document
  onvisibilitychange={() => {
    if (document.visibilityState === 'visible') void sync();
  }}
/>
