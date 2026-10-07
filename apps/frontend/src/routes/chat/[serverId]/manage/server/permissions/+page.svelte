<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { HeaderIconButton, Hint, PaneContent, PaneHeader, PageTitle } from '$lib/ui';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { m } from '$lib/i18n/messages';
  import type { TierRole } from '@chatto/client/api/permissions';

  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));

  // Role detail pages require admin.manage-roles, and the role order allows
  // edits only to roles below the viewer's highest role. Gate the column-header
  // click so other viewers see plain text.
  const canManageRoles = $derived(serverScope.store.permissions.canAdminManageRoles);
  const error = $derived(null);

  function isRoleClickable(role: TierRole): boolean {
    return canManageRoles && role.ranksBelowViewer;
  }

  function openRoleDetail(role: TierRole) {
    goto(
      resolve('/chat/[serverId]/manage/server/permissions/[name]', {
        serverId: serverSegment,
        name: role.roleName
      })
    );
  }
</script>

<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.permissions.title') })}
/>

<div class="pane-page">
  <PaneHeader title={m('admin.permissions.title')} subtitle={m('admin.permissions.subtitle')}>
    {#snippet actions()}
      {#if canManageRoles}
        <HeaderIconButton
          icon="icon-[uil--sort-amount-down]"
          label={m('admin.permissions.role_order.title')}
          href={resolve('/chat/[serverId]/manage/server/role-order', {
            serverId: serverSegment
          })}
        />
      {/if}
    {/snippet}
  </PaneHeader>

  <PaneContent fillHeight>
    <div class="flex min-h-0 flex-1 flex-col gap-6">
      {#if error}
        <Hint tone="danger">{error}</Hint>
      {:else}
        <PermissionMatrix
          onRoleClick={openRoleDetail}
          {isRoleClickable}
          newRoleHref={canManageRoles
            ? resolve('/chat/[serverId]/manage/server/permissions/new', { serverId: serverSegment })
            : undefined}
          fillHeight
        >
          {#snippet subtitle()}
            {m('admin.permissions.server_tier_intro')}
            <a
              href={resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment })}
              class="link">{m('admin.permissions.server_tier_rooms_hint')}</a
            >
          {/snippet}
        </PermissionMatrix>
      {/if}
    </div>
  </PaneContent>
</div>
