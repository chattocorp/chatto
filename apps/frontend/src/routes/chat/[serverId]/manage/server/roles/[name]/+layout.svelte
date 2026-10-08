<!--
@component

Shared frame of the role detail sections. It loads the role, renders the
header and the section tabs, and provides the role to the General,
Permissions, and Members pages through `roleDetailContext`.

A role that does not rank below the viewer's highest role opens read-only.
-->
<script lang="ts">
  import { type Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createRoleAPI } from '@chatto/client/api/roles';
  import type { Role } from '$lib/components/rbac';
  import { m } from '$lib/i18n/messages';
  import { captureMutationCompletion } from '$lib/navigation/mutationCompletion';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import {
    Hint,
    LoadingFog,
    PageTitle,
    PaneContent,
    PaneHeader,
    TabNav,
    type TabNavItem
  } from '$lib/ui';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { provideRoleDetail, type RoleMutationScope } from './roleDetailContext';

  let { children }: { children: Snippet } = $props();

  const serverScope = useServerScope();
  const serverSegment = serverIdToSegment(serverScope.serverId);
  const roleName = $derived(page.params.name!);
  const session = createSessionGuard(serverScope);
  let scrollContainer = $state<HTMLDivElement>();

  const roleQuery = createQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    const targetRoleName = roleName;
    return {
      queryKey: adminQueryKeys.role(serverId, connection, targetRoleName),
      queryFn: ({ signal }) => connection.getAPI(createRoleAPI).getRole(targetRoleName, { signal })
    };
  });

  const roleDetails = $derived(roleQuery.data ?? null);
  const role = $derived<Role | null>(roleDetails?.role ?? null);
  const canManageRoles = $derived(roleDetails?.viewerCanManageRoles ?? false);
  const canAssignRoles = $derived(roleDetails?.viewerCanAssignRoles ?? false);
  // Role managers change every role except owner, which only owners change.
  const canChangeRole = $derived(
    serverScope.store.roleCatalog.canChangeRole(roleName, canManageRoles)
  );
  const canEditRole = $derived(canManageRoles && canChangeRole);
  const loading = $derived(roleQuery.isPending);
  const rolesHref = resolve('/chat/[serverId]/manage/server/roles', { serverId: serverSegment });

  function isCurrentRole(target: RoleMutationScope | undefined): target is RoleMutationScope {
    return session.isCurrent(target) && target.roleName === roleName;
  }

  provideRoleDetail({
    get roleName() {
      return roleName;
    },
    get role() {
      return role!;
    },
    get canAssignRoles() {
      return canAssignRoles;
    },
    get canEditRole() {
      return canEditRole;
    },
    get scrollContainer() {
      return scrollContainer;
    },
    mutationScope() {
      const snapshot = session.snapshot();
      return {
        ...snapshot,
        roleName,
        queryKey: adminQueryKeys.role(snapshot.serverId, snapshot.connection, roleName),
        api: snapshot.connection.getAPI(createRoleAPI),
        canComplete: captureMutationCompletion(serverScope)
      };
    },
    isCurrentRole
  });

  const sectionTabs = $derived.by<TabNavItem[]>(() => {
    if (!role || !canManageRoles) return [];
    const params = { serverId: serverSegment, name: roleName };
    const routeId = page.route.id;
    const base = '/chat/[serverId]/manage/server/roles/[name]';
    const items: TabNavItem[] = [
      {
        href: resolve('/chat/[serverId]/manage/server/roles/[name]', params),
        label: m('admin.permissions.role_tabs.general'),
        icon: 'icon-[uil--sliders-v-alt]',
        current: routeId === base
      },
      {
        href: resolve('/chat/[serverId]/manage/server/roles/[name]/permissions', params),
        label: m('admin.permissions.role_tabs.permissions'),
        icon: 'icon-[uil--shield-check]',
        current: routeId === `${base}/permissions`
      }
    ];
    // Everyone holds the everyone role implicitly, so it has no member list.
    if (canAssignRoles && role.name !== 'everyone') {
      items.push({
        href: resolve('/chat/[serverId]/manage/server/roles/[name]/members', params),
        label: m('admin.permissions.role_tabs.members'),
        icon: 'icon-[uil--users-alt]',
        current: routeId === `${base}/members`
      });
    }
    return items;
  });
</script>

{#snippet roleSubtitle()}
  {#if role}
    <span class="block truncate" dir="auto">{role.displayName}</span>
  {:else}
    <LoadingFog class="h-4 w-28" label={m('admin.permissions.loading_role')} />
  {/if}
{/snippet}

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: role?.displayName ?? m('admin.permissions.edit_role_title')
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.permissions.edit_role_title')}
    subtitle={role?.displayName ?? m('admin.permissions.loading_role')}
    subtitleContent={roleSubtitle}
    backHref={rolesHref}
    backLabel={m('admin.permissions.back_to_roles')}
  >
    {#snippet tabs()}
      <TabNav label={m('admin.permissions.role_tabs.label')} items={sectionTabs} />
    {/snippet}
  </PaneHeader>

  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      {#if loading}
        <LoadingFog class="h-40 w-full" label={m('admin.permissions.loading_role')} />
      {:else if roleQuery.error}
        <Hint tone="danger">{errorMessage(roleQuery.error)}</Hint>
      {:else if !role}
        <Hint tone="danger">{m('admin.permissions.role_not_found')}</Hint>
      {:else if !canManageRoles}
        <Hint tone="danger">{m('admin.permissions.need_manage_edit')}</Hint>
      {:else}
        {#if !canChangeRole}
          <Hint>{m('rbac.role_order.role_locked')}</Hint>
        {/if}
        {@render children()}
      {/if}
    </div>
  </PaneContent>
</div>
