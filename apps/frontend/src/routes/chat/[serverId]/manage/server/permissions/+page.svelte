<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { PaneContent, PaneHeader, PageTitle } from '$lib/ui';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { m } from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const serverSegment = $derived(serverIdToSegment(serverScope.serverId));

  const canManageRoles = $derived(serverScope.store.permissions.canAdminManageRoles);

  function openRoleDetail(role: { roleName: string }) {
    goto(
      resolve('/chat/[serverId]/manage/server/roles/[name]', {
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
  <PaneHeader
    title={m('admin.permissions.title')}
    subtitle={canManageRoles ? m('admin.permissions.subtitle') : undefined}
  />

  <PaneContent fillHeight>
    <div class="flex min-h-0 flex-1 flex-col gap-6">
      {#if canManageRoles}
        <PermissionMatrix
          onRoleClick={openRoleDetail}
          isRoleClickable={() => canManageRoles}
          newRoleHref={canManageRoles
            ? resolve('/chat/[serverId]/manage/server/roles/new', { serverId: serverSegment })
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
