<!--
@component

Monitors for app updates and notifies the user via toast. When a new version
is detected, shows a toast with a reload button and force-reconnects the
WebSocket to recover real-time updates immediately. If the user dismisses the
toast and navigates, the page reloads anyway to prevent stale chunk errors.

Include this component once at the root layout level.
-->
<script lang="ts">
  import { onNavigate } from '$app/navigation';
  import { updated } from '$app/state';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { toast } from '$lib/ui/toast';

  // Track whether we've already shown the update toast
  let updateToastShown = false;

  // Show a toast when a new version is detected
  $effect(() => {
    if (updated.current && !updateToastShown) {
      updateToastShown = true;
      toast.info('A new version is available', 0, {
        label: 'Reload',
        onClick: () => location.reload()
      });

      // Force-reconnect the WebSocket — a deploy means the old connection
      // is stale even if the client thinks it's still connected
      graphqlClientManager.originClient.forceReconnect('app update detected');
    }
  });

  // Fallback: if user dismisses the toast and navigates, reload anyway
  // to prevent stale chunk errors
  onNavigate(() => {
    if (updated.current) {
      location.reload();
    }
  });
</script>
