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
  import * as m from '$lib/i18n/messages';

  const groupId = $derived(page.params.groupId!);
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  const backHref = $derived(
    resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })
  );

  // Lightweight lookup for the group's display name (the matrix itself
  // fetches its own data via admin.rbac.rolePermissionTierMatrix).
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
  const group = $derived(nameQuery.data?.server?.roomGroups.find((g) => g.id === groupId) ?? null);
  const pageTitle = $derived(
    group
      ? m['admin.rooms_admin.permissions_page_title']({ name: group.name })
      : m['admin.rooms_admin.group_permissions_title_fallback']()
  );
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: pageTitle })} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={group?.name ?? ''}
    subtitle={m['admin.rooms_admin.group_permissions_subtitle']()}
    {backHref}
    backLabel={m['admin.rooms_admin.back_to_rooms']()}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <Hint>{m['admin.rooms_admin.group_permissions_hint']()}</Hint>
    <PermissionMatrix {groupId} />
  </div>
</div>
