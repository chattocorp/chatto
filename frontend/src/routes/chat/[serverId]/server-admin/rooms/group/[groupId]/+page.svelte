<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';

  const groupId = $derived(page.params.groupId!);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })
  );

  // Lightweight lookup for the group's display name (the matrix itself
  // fetches its own data via tierRoles).
  const GroupNameQuery = graphql(`
    query AdminGroupPermissionsName {
      server {
        roomGroups {
          id
          name
        }
      }
    }
  `);

  const nameQuery = useQuery(GroupNameQuery, () => ({}));
  const group = $derived(
    nameQuery.data?.server?.roomGroups.find((g) => g.id === groupId) ?? null
  );
  const pageTitle = $derived(group ? `Permissions — ${group.name}` : 'Group permissions');
</script>

<PageTitle title={`${pageTitle} | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={group?.name ?? ''}
    subtitle="Per-group role permission grants and denials"
    {backHref}
    backLabel="Back to rooms"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <Hint>
      Per-group overrides for the channel rooms in this group. Defaults inherited from the server scope.
      Individual rooms can further override permissions from their
      own permissions page.
    </Hint>
    <PermissionMatrix {groupId} />
  </div>
</div>
