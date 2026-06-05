<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { graphql } from '$lib/gql';
  import { useQuery } from '$lib/hooks';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import { Panel } from '$lib/components/admin';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';

  // Lightweight query just to gate UI on viewerCanManageRoles. The heavy
  // lifting is done by PermissionMatrix's own tierRoles query.
  const SpaceRolesGateQuery = graphql(`
    query SpaceRolesGate {
      server {
        viewerCanManageRoles
      }
    }
  `);

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));

  const gateQuery = useQuery(SpaceRolesGateQuery, () => ({}));
  const canManageRoles = $derived(gateQuery.data?.server?.viewerCanManageRoles ?? false);
  const error = $derived(
    gateQuery.error ?? (!gateQuery.loading && !gateQuery.data?.server ? 'Server not found' : null)
  );

  // Role detail pages require admin.manage-roles. Gate the column-header
  // click so non-admins see plain text.
  const serverPerms = getServerPermissions();
  const canManageRolesFull = $derived(serverPerms.current.canAdminManageRoles);

  function openRoleDetail(role: { roleName: string }) {
    goto(
      resolve('/chat/[serverId]/server-admin/permissions/[name]', {
        serverId: serverSegment,
        name: role.roleName
      })
    );
  }
</script>

<PageTitle title="Permissions | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Permissions" subtitle="Manage space roles and permissions" showMobileNav />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      {#if canManageRoles}
        <Panel title="Role Presets">
          <p class="mb-4 text-muted">
            Roles bundle permissions for groups of users — for example a "moderator" role that
            can moderate messages, or a "dev team" role with access to engineering rooms.
            Assign roles to members from each user's profile.
          </p>
          <Button
            variant="primary"
            size="sm"
            href={resolve('/chat/[serverId]/server-admin/permissions/new', {
              serverId: serverSegment
            })}
          >
            Create Role
          </Button>
        </Panel>
      {/if}
      <Hint>
        The settings on this page act as <strong>server-wide defaults</strong>. You can override individual permissions for each room
        or room group via the
        <a
          href={resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })}
          class="link">Rooms</a
        > page.
      </Hint>
      <PermissionMatrix
        onRoleClick={openRoleDetail}
        isRoleClickable={() => canManageRolesFull}
      />
    {/if}
  </div>
</div>
