<script lang="ts">
  import { Panel, DataTable } from '$lib/components/admin';
  import { HelpTooltip, Pill, ToggleChip } from '$lib/ui';
  import { getPermissionDescription } from '$lib/permissions';

  type PermissionState = 'allow' | 'deny' | 'neutral';

  // Default category order - can be overridden via prop
  const DEFAULT_CATEGORY_ORDER = [
    'space',
    'room',
    'message',
    'member',
    'role',
    'admin',
    'dm',
    'user'
  ];

  let {
    permissions,
    grantedPermissions,
    deniedPermissions = [],
    inheritedPermissions = [],
    inheritedDenials = [],
    inheritedFromLabel,
    disabled = false,
    updatingPermission = null,
    categoryOrder = DEFAULT_CATEGORY_ORDER,
    onSetState
  }: {
    permissions: string[];
    /** Permissions explicitly granted at this scope. */
    grantedPermissions: string[];
    /** Permissions explicitly denied at this scope. */
    deniedPermissions?: string[];
    /**
     * Permissions inherited as granted from the parent scope. Shown as a
     * faint hint when no override exists at this scope.
     */
    inheritedPermissions?: string[];
    /** Permissions inherited as denied from the parent scope. */
    inheritedDenials?: string[];
    /**
     * Human-readable label for the parent scope (e.g. "space", "instance").
     * Required for inheritance hints to display; otherwise inheritance is
     * silently ignored.
     */
    inheritedFromLabel?: string;
    disabled?: boolean;
    updatingPermission?: string | null;
    categoryOrder?: string[];
    onSetState: (permission: string, state: PermissionState) => void;
  } = $props();

  // Category metadata with display info
  const categoryMeta: Record<string, { title: string; description: string }> = {
    space: {
      title: 'Space Operations',
      description: 'Control who can browse, create, join, and manage spaces'
    },
    room: {
      title: 'Room Operations',
      description: 'Control who can create, join, and manage rooms'
    },
    message: {
      title: 'Messages',
      description: 'Control what users can do with messages'
    },
    member: {
      title: 'Member Management',
      description: 'Control who can invite and remove space members'
    },
    role: {
      title: 'Role Management',
      description: 'Control who can create roles and assign them to users'
    },
    admin: {
      title: 'Instance Administration',
      description: 'Access to instance-wide admin functions'
    },
    dm: {
      title: 'Direct Messages',
      description: 'Control access to direct messaging'
    },
    user: {
      title: 'User Management',
      description: 'Control user account operations'
    }
  };

  function getCategory(permission: string): string {
    const dotIndex = permission.indexOf('.');
    return dotIndex > 0 ? permission.slice(0, dotIndex) : permission;
  }

  const groupedPermissions = $derived.by(() => {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- Map is ephemeral within derived computation
    const groups = new Map<string, string[]>();

    for (const perm of permissions) {
      const category = getCategory(perm);
      if (!groups.has(category)) {
        groups.set(category, []);
      }
      groups.get(category)!.push(perm);
    }

    for (const perms of groups.values()) {
      perms.sort((a, b) => a.localeCompare(b));
    }

    const result: Array<{ category: string; permissions: string[] }> = [];
    for (const category of categoryOrder) {
      const perms = groups.get(category);
      if (perms && perms.length > 0) {
        result.push({ category, permissions: perms });
      }
    }
    for (const [category, perms] of groups) {
      if (!categoryOrder.includes(category) && perms.length > 0) {
        result.push({ category, permissions: perms });
      }
    }

    return result;
  });

  function getPermissionState(id: string): PermissionState {
    if (grantedPermissions.includes(id)) return 'allow';
    if (deniedPermissions.includes(id)) return 'deny';
    return 'neutral';
  }

  function getInheritedState(id: string): PermissionState {
    if (inheritedPermissions.includes(id)) return 'allow';
    if (inheritedDenials.includes(id)) return 'deny';
    return 'neutral';
  }

  function toggleAllow(permission: string, current: PermissionState) {
    onSetState(permission, current === 'allow' ? 'neutral' : 'allow');
  }

  function toggleDeny(permission: string, current: PermissionState) {
    onSetState(permission, current === 'deny' ? 'neutral' : 'deny');
  }
</script>

<div class="flex flex-col gap-6">
  {#each groupedPermissions as group (group.category)}
    {@const meta = categoryMeta[group.category]}

    <Panel title={meta?.title ?? group.category} subtitle={meta?.description} noPadding>
      <DataTable
        items={group.permissions}
        columns={3}
        getKey={(p) => p}
        emptyMessage="No permissions"
      >
        {#snippet header()}
          <!-- Fixed widths on Inherited + Override keep the columns lined up
               across category tables; Permission takes the remainder. -->
          <th class="px-4 py-3 text-left font-medium">Permission</th>
          <th class="w-48 px-4 py-3 text-left font-medium">Inherited</th>
          <th class="w-44 px-4 py-3 text-left font-medium">Override</th>
        {/snippet}
        {#snippet row(permission)}
          {@const state = getPermissionState(permission)}
          {@const inherited = getInheritedState(permission)}
          {@const isUpdating = updatingPermission === permission}
          {@const isDisabled = disabled || isUpdating}
          {@const hasInherited = inherited !== 'neutral' && !!inheritedFromLabel}
          {@const overridden = state !== 'neutral' && hasInherited}
          <!-- Effective state combines override + inheritance: the permission
               identifier reflects what the role actually does at this scope,
               not just the override toggle. -->
          {@const effective = state !== 'neutral' ? state : inherited}

          <td class={['px-4 py-3', isUpdating ? 'animate-pulse' : '']}>
            <div class="flex items-center gap-1.5">
              <span
                data-testid="permission-name"
                class={[
                  effective === 'allow'
                    ? 'text-success'
                    : effective === 'deny'
                      ? 'text-danger'
                      : ''
                ]}
              >
                {permission}
              </span>
              <HelpTooltip label={`What does ${permission} do?`}>
                {getPermissionDescription(permission)}
              </HelpTooltip>
            </div>
          </td>

          <td class={['w-48 px-4 py-3', isUpdating ? 'animate-pulse' : '']}>
            {#if hasInherited}
              <Pill
                tone={inherited === 'allow' ? 'success' : 'danger'}
                dimmed={overridden}
                title={overridden
                  ? `Inherited ${inherited === 'allow' ? 'Allow' : 'Deny'} from ${inheritedFromLabel}, currently overridden at this scope`
                  : `Inherited from ${inheritedFromLabel} when no override is set at this scope`}
              >
                {inherited === 'allow' ? 'Allow' : 'Deny'} from {inheritedFromLabel}
              </Pill>
            {:else}
              <span class="text-xs text-muted/50">—</span>
            {/if}
          </td>

          <td class={['w-44 px-4 py-3', isUpdating ? 'animate-pulse' : '']}>
            <div class="flex items-center gap-2">
              <ToggleChip
                pressed={state === 'allow'}
                tone="success"
                disabled={isDisabled}
                onclick={() => toggleAllow(permission, state)}
              >
                Allow
              </ToggleChip>
              <ToggleChip
                pressed={state === 'deny'}
                tone="danger"
                disabled={isDisabled}
                onclick={() => toggleDeny(permission, state)}
              >
                Deny
              </ToggleChip>
            </div>
          </td>
        {/snippet}
      </DataTable>
    </Panel>
  {/each}
</div>
