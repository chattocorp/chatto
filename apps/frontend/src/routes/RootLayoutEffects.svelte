<script lang="ts">
  import { afterNavigate, goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { usePinchZoomPrevention, useVisualViewport } from '$lib/hooks';
  import { chatRoomIdFromRoute } from '$lib/navigation/chatRoomRoute';
  import { onNotificationClick } from '$lib/notifications/pushNotifications';
  import { prepareUiForNotificationPath } from '$lib/notifications/notificationNavigationUi';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import type { AppUiState } from '$lib/state/appUi.svelte';
  import { sidebarNav } from '$lib/state/globals.svelte';

  let { appUi }: { appUi: AppUiState } = $props();

  useVisualViewport();
  usePinchZoomPrevention();

  const activeServerId = $derived(getActiveServer());
  const activeRoomId = $derived(chatRoomIdFromRoute(page.route.id, page.params.roomId));

  $effect(() => {
    if (typeof activeRoomId === 'string' && activeRoomId) {
      appUi.setActiveRoomScope(activeServerId, activeRoomId);
      return;
    }
    appUi.setActiveServer(activeServerId);
  });

  // Route push-notification clicks via SvelteKit's client-side navigation
  // instead of letting the SW do a full document navigation. Same-URL
  // clicks become a no-op; cross-URL clicks just update the route.
  $effect(() =>
    onNotificationClick((url) => {
      try {
        const target = new URL(url);
        if (target.origin !== window.location.origin) return;
        prepareUiForNotificationPath(appUi, target.pathname);
        return goto(resolve((target.pathname + target.search + target.hash) as '/'));
      } catch {
        // Ignore malformed URLs from the SW.
      }
    })
  );

  $effect(() => sidebarNav.initViewportTracking());
  afterNavigate(() => {
    if (sidebarNav.isMobile) sidebarNav.close();
  });
</script>
