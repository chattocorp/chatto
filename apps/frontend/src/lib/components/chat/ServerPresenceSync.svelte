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

  // User snapshot resources include each visible user's current presence.
  useProjectionEvent((event) => {
    if (event.snapshot?.resource.case !== 'users') return;
    presenceCache.replaceServer(
      serverScope.serverId,
      new Map(
        event.snapshot.resource.value.users.flatMap((member) =>
          member.user?.id && member.user.presenceStatus !== undefined
            ? [[member.user.id, member.user.presenceStatus] as const]
            : []
        )
      )
    );
  });
</script>

<div data-testid="server-subscription-active" class="hidden"></div>
