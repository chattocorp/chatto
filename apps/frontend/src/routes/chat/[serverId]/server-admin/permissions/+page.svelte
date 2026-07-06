<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { Hint } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import { Panel } from '$lib/components/admin';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import * as m from '$lib/i18n/messages';

  const serverSegment = $derived(serverIdToSegment(getActiveServer()));

  // Role detail pages require admin.manage-roles. Gate the column-header
  // click so non-admins see plain text.
  const serverPerms = getServerPermissions();
  const canManageRolesFull = $derived(serverPerms.current.canAdminManageRoles);
  const canManageRoles = $derived(canManageRolesFull);
  const error = $derived(null);

  function openRoleDetail(role: { roleName: string }) {
    goto(
      resolve('/chat/[serverId]/server-admin/permissions/[name]', {
        serverId: serverSegment,
        name: role.roleName
      })
    );
  }
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({ title: m['admin.permissions.title']() })}
/>

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.permissions.title']()}
    subtitle={m['admin.permissions.subtitle']()}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if error}
      <Hint tone="danger">{error}</Hint>
    {:else}
      {#if canManageRoles}
        <Panel title={m['admin.permissions.role_presets']()}>
          <p class="mb-4 text-muted">
            {m['admin.permissions.role_presets_intro']()}
          </p>
          <Button
            variant="primary"
            size="sm"
            href={resolve('/chat/[serverId]/server-admin/permissions/new', {
              serverId: serverSegment
            })}
          >
            {m['admin.permissions.create_role_action']()}
          </Button>
        </Panel>
      {/if}
      <Hint>
        <div class="space-y-2">
          <p>
            {m['admin.permissions.server_tier_intro']()}
          </p>
          <p>
            {m['admin.permissions.server_tier_rooms_hint']()}
            <a
              href={resolve('/chat/[serverId]/server-admin/rooms', { serverId: serverSegment })}
              class="link">{m['admin.common.rooms']()}</a
            >
          </p>
          <p>
            {m['admin.permissions.server_tier_denies_hint']()}
          </p>
        </div>
      </Hint>
      <PermissionMatrix onRoleClick={openRoleDetail} isRoleClickable={() => canManageRolesFull} />
    {/if}
  </div>
</div>
