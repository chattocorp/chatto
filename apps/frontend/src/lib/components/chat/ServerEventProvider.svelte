<script lang="ts">
  import { provideEventBus } from '$lib/eventBus.svelte';
  import { usePresenceChange, useProjectionEvent } from '$lib/hooks';
  import { apiPresenceStatus } from '$lib/api-client/memberDirectory';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import type { Snippet } from 'svelte';

  let { children }: { children: Snippet } = $props();

  // The myEvents subscription was started by the registry when this server
  // got connected; here we just expose its bus via Svelte context so
  // descendant components can register handlers without going through the
  // manager directly. The getter form keeps the bus reactive across
  // `[serverId]` URL changes — typed-event consumers below
  // automatically follow the active server.
  provideEventBus(getActiveServer);

  // Capture presence cache during init (context must be read synchronously)
  const presenceCache = getPresenceCache();

  // Per-server stores (rooms list, room directory, …) self-manage their
  // refresh and event-ingestion lifecycles from inside `ServerStateStore`
  // — every server keeps itself in sync with its own bus, so consumers
  // here and below just read `serverRegistry.getStore(...)` and don't
  // wire any additional `$effect` for that purpose.

  // Populate global presence cache from server events so that any UserAvatar
  // (including newly-mounted ones like popovers) sees the latest presence.
  usePresenceChange((userId, status) => {
    presenceCache.update({ serverId: getActiveServer(), userId }, status);
  });

  // Presence is transient rather than EVT-backed. Every subscription sends a
  // complete latest-value reconciliation before caught_up so returning to a
  // retained server cannot display transitions missed while it was dormant.
  useProjectionEvent((event) => {
    for (const operation of event.operations) {
      if (operation.operation.case !== 'presencesReplace') continue;
      presenceCache.replaceServer(
        getActiveServer(),
        new Map(
          Object.entries(operation.operation.value.statuses).map(([userId, status]) => [
            userId,
            apiPresenceStatus(status)
          ])
        )
      );
    }
  });
</script>

<div data-testid="server-subscription-active" class="hidden"></div>
{@render children()}
