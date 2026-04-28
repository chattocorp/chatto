<script lang="ts" module>
  import {
    PermAdminAccess,
    PermAdminUsersView,
    PermAdminSpacesView,
    PermAdminRolesView,
    PermAdminRolesManage,
    PermAdminUsersManage,
    PermAdminSystemView,
    type Permission
  } from '$lib/types/core';

  // Permission required for each admin sub-route (relative paths).
  // Used by getRoutePermission() to check against the current URL.
  export const routePermissions: Record<string, Permission> = {
    '': PermAdminAccess,
    'users': PermAdminUsersView,
    'spaces': PermAdminSpacesView,
    'roles': PermAdminRolesView,
    'inspector': PermAdminRolesView,
    'system': PermAdminSystemView,
    'settings/instance': PermAdminAccess
  };
</script>

<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { createAdminPermissions, getInstancePermissions, viewerHasPermission } from '$lib/state/instance/permissions.svelte';

  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import LoadingPage from '$lib/ui/LoadingPage.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';

  let { children } = $props();

  // Read permissions from centralized instance permissions context
  const instancePerms = getInstancePermissions();
  // Admin requires explicit permission — keep the loading gate here.
  // This layout is inside [instanceId] which has an auth guard, so the viewer
  // query is already in flight by the time this renders. Brief loading is expected.
  let loaded = $derived(instancePerms.current.loaded);
  let canViewAdmin = $derived(instancePerms.current.canViewAdmin);

  // Provide admin permissions to child components (reads from instance permissions context)
  createAdminPermissions({
    hasPermission(perm: string) {
      return viewerHasPermission(instancePerms.current, perm);
    }
  });

  // Nav items with required permissions
  const allNavItems = $derived([
    {
      href: resolve('/chat/[instanceId]/admin', { instanceId: instanceSegment }),
      label: 'Dashboard',
      icon: 'iconify uil--dashboard',
      perm: PermAdminAccess
    },
    {
      href: resolve('/chat/[instanceId]/admin/settings/instance', { instanceId: instanceSegment }),
      label: 'Instance',
      icon: 'iconify uil--setting',
      perm: PermAdminAccess
    },
    {
      href: resolve('/chat/[instanceId]/admin/users', { instanceId: instanceSegment }),
      label: 'Users',
      icon: 'iconify uil--users-alt',
      perm: PermAdminUsersView
    },
    {
      href: resolve('/chat/[instanceId]/admin/spaces', { instanceId: instanceSegment }),
      label: 'Spaces',
      icon: 'iconify uil--comments',
      perm: PermAdminSpacesView
    },
    {
      href: resolve('/chat/[instanceId]/admin/roles', { instanceId: instanceSegment }),
      label: 'Roles',
      icon: 'iconify uil--shield-check',
      perm: PermAdminRolesView
    },
    {
      href: resolve('/chat/[instanceId]/admin/inspector', { instanceId: instanceSegment }),
      label: 'Inspector',
      icon: 'iconify uil--search',
      perm: PermAdminRolesView
    },
    {
      href: resolve('/chat/[instanceId]/admin/system', { instanceId: instanceSegment }),
      label: 'System',
      icon: 'iconify uil--server',
      perm: PermAdminSystemView
    }
  ]);

  // Filter nav items based on permissions
  const navItems = $derived(
    allNavItems.filter((item) => viewerHasPermission(instancePerms.current, item.perm))
  );

  // Check if current route requires a permission the user doesn't have.
  // Extract the sub-path relative to the admin base to match against routePermissions.
  function getRoutePermission(pathname: string): Permission {
    const base = resolve('/chat/[instanceId]/admin', { instanceId: instanceSegment });
    const sub = pathname.startsWith(base) ? pathname.slice(base.length).replace(/^\//, '') : '';

    // Exact match first
    if (routePermissions[sub] !== undefined) {
      return routePermissions[sub];
    }
    // Dynamic role routes (edit role requires admin.manage-roles)
    if (sub.startsWith('roles/') && sub !== 'roles') {
      return PermAdminRolesManage;
    }
    // Dynamic user routes (user management requires admin.manage-users)
    if (sub.startsWith('users/') && sub !== 'users') {
      return PermAdminUsersManage;
    }
    // Settings sub-routes default to admin permission
    if (sub.startsWith('settings/')) {
      return PermAdminAccess;
    }
    // Default to admin
    return PermAdminAccess;
  }

  const currentRoutePermission = $derived(getRoutePermission(page.url.pathname));
  const hasCurrentRoutePermission = $derived(
    viewerHasPermission(instancePerms.current, currentRoutePermission)
  );
</script>

<!-- Instance permissions loaded by chat layout, component handles access denied UI -->
{#if !loaded}
  <LoadingPage />
{:else if !canViewAdmin}
  <AccessDenied message="You do not have permission to view the admin panel." />
{:else if !hasCurrentRoutePermission}
  <AccessDenied
    message="You do not have the {currentRoutePermission} permission."
    backHref={resolve('/chat/[instanceId]/admin', { instanceId: instanceSegment })}
    backLabel="Return to Dashboard"
  />
{:else}
  <SecondarySidebar width="md:w-56">
    <SidebarNav title="Instance" items={navItems} backHref={resolve('/chat/[instanceId]', { instanceId: instanceSegment })} />
  </SecondarySidebar>

  <!-- Main content -->
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    {@render children?.()}
  </div>
{/if}
