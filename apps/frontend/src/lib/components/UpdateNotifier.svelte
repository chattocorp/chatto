<!--
@component

Monitors for app updates and reloads the page automatically as soon as the
user is idle (not typing, not in a call — see `idleState.canSafelyReload`).
While the user is busy, a toast offers a manual reload. As a final fallback,
the next navigation triggers a reload to avoid stale chunk errors.

Include this component once at the root layout level.
-->
<script lang="ts">
  import { onNavigate } from '$app/navigation';
  import { updated } from '$app/state';
  import { idleState } from '$lib/state/idle.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { toast } from '$lib/ui/toast';
  import * as m from '$lib/i18n/messages';

  let updateToastShown = false;

  $effect(() => {
    if (!updated.current) return;

    if (!updateToastShown) {
      updateToastShown = true;
      toast.info(m['ui.update_available'](), 0, {
        label: m['ui.reload'](),
        onClick: () => location.reload()
      });

      // Force-reconnect the WebSocket — a deploy means the old connection
      // is stale even if the client thinks it's still connected
      serverConnectionManager.originClient.forceReconnect('app update detected');
    }

    // Auto-reload as soon as it's safe. The effect re-runs when the user
    // blurs the composer / leaves a call, so a busy user just sees the
    // toast until they idle out.
    if (idleState.canSafelyReload) {
      location.reload();
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
