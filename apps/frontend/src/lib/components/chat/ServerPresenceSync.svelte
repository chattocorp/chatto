<script lang="ts">
  import { usePresenceChange, useProjectionEvent } from '$lib/hooks/useEvent.svelte';
  import { getPresenceCache } from '$lib/state/presenceCache.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';

  // Capture route and presence contexts during component initialization.
  const serverScope = useServerScope();
  const presenceCache = getPresenceCache();

  // Populate global presence cache from server events so that any UserAvatar
  // (including newly-mounted ones like popovers) sees the latest presence.
  usePresenceChange((userId, status) => {
    presenceCache.update({ serverId: serverScope.serverId, userId }, status);
  });

  // Cursor-bounded user reads include current presence for each returned user.
  useProjectionEvent((event) => {
    if (event.reset) {
      presenceCache.replaceServer(serverScope.serverId, new Map());
      return;
    }
    if (event.resource?.case !== 'users') return;
    const statuses = new Map(
      event.resource.value.users.flatMap((member) =>
        member.user?.id && member.user.presenceStatus !== undefined
          ? [[member.user.id, member.user.presenceStatus] as const]
          : []
      )
    );
    if (event.replaceResource) {
      presenceCache.replaceServer(serverScope.serverId, statuses);
      return;
    }
    for (const [userId, status] of statuses) {
      presenceCache.update({ serverId: serverScope.serverId, userId }, status);
    }
  });
</script>

<div data-testid="server-subscription-active" class="hidden"></div>
