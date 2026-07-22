<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { Hint, PaneContent } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
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
      resolve('/chat/[serverId]/manage/server/permissions/[name]', {
        serverId: serverSegment,
        name: role.roleName
      })
    );
  }
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({ title: m['admin.permissions.title']() })}
/>

<div class="pane-page">
  <PaneHeader
    title={m['admin.permissions.title']()}
    subtitle={m['admin.permissions.subtitle']()}
    showMobileNav
  />

  <PaneContent fillHeight>
    <div class="flex min-h-0 flex-1 flex-col gap-6">
      {#if error}
        <Hint tone="danger">{error}</Hint>
      {:else}
        <PermissionMatrix
          onRoleClick={openRoleDetail}
          isRoleClickable={() => canManageRolesFull}
          newRoleHref={
            canManageRoles
              ? resolve('/chat/[serverId]/manage/server/permissions/new', { serverId: serverSegment })
              : undefined
          }
          fillHeight
        >
          {#snippet subtitle()}
            {m['admin.permissions.server_tier_intro']()}
            <a
              href={resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment })}
              class="link">{m['admin.permissions.server_tier_rooms_hint']()}</a
            >
          {/snippet}
        </PermissionMatrix>
      {/if}
    </div>
  </PaneContent>
</div>
