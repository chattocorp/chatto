<script lang="ts">
  import { usePresenceChange, useProjectionEvent } from '$lib/hooks/useEvent.svelte';
  import { apiPresenceStatus } from '$lib/api-client/memberDirectory';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { Snippet } from 'svelte';

  let { children }: { children: Snippet } = $props();

  // Capture route and presence contexts during component initialization.
  const serverScope = useServerScope();
  const presenceCache = getPresenceCache();

  // Per-server stores (rooms list, room directory, …) self-manage their
  // refresh and event-ingestion lifecycles from inside `ServerStateStore`
  // — every server keeps itself in sync with its own bus, so consumers
  // here and below just read `serverRegistry.getStore(...)` and don't
  // wire any additional `$effect` for that purpose.

  // Populate global presence cache from server events so that any UserAvatar
  // (including newly-mounted ones like popovers) sees the latest presence.
  usePresenceChange((userId, status) => {
    presenceCache.update({ serverId: serverScope.serverId, userId }, status);
  });

  // Presence is transient rather than EVT-backed. Every subscription sends a
  // complete latest-value reconciliation before caught_up so returning to a
  // retained server cannot display transitions missed while it was dormant.
  useProjectionEvent((event) => {
    for (const operation of event.operations) {
      if (operation.operation.case !== 'presencesReplace') continue;
      presenceCache.replaceServer(
        serverScope.serverId,
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
