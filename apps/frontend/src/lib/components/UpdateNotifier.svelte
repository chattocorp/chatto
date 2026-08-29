<!--
@component

Monitors for app updates and shows a persistent toast with a manual reload
action. The app never reloads automatically after it detects an update.

Include this component once at the root layout level.
-->
<script lang="ts">
  import { updated } from '$app/state';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { toast } from '$lib/ui/toast';
  import { m } from '$lib/i18n/messages';

  let { reloadApp = () => location.reload() }: { reloadApp?: () => void } = $props();

  let updateToastShown = false;

  $effect(() => {
    if (!updated.current || updateToastShown) return;

    updateToastShown = true;
    toast.info(m('ui.update_available'), 0, {
      label: m('ui.reload'),
      onClick: reloadApp
    });

    // Force-reconnect the WebSocket — a deploy means the old connection
    // is stale even if the client thinks it's still connected.
    serverConnectionManager.originClient.forceReconnect('app update detected');
  });
</script>
