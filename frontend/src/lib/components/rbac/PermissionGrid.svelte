<script lang="ts">
  import { DataTable } from '$lib/components/admin';
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
     * faint hint in the row when no override exists at this scope.
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

  // Extract category from permission ID (e.g., "message.delete-any" -> "message")
  function getCategory(permission: string): string {
    const dotIndex = permission.indexOf('.');
    return dotIndex > 0 ? permission.slice(0, dotIndex) : permission;
  }

  // Group permissions by category
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
</script>

<div class="flex flex-col gap-8">
  {#each groupedPermissions as group (group.category)}
    {@const meta = categoryMeta[group.category]}
    <div class="flex flex-col gap-3">
      <div>
        <h3 class="font-semibold">{meta?.title ?? group.category}</h3>
        {#if meta?.description}
          <p class="text-sm text-muted">{meta.description}</p>
        {/if}
      </div>

      <DataTable
        items={group.permissions}
        columns={2}
        getKey={(p) => p}
        emptyMessage="No permissions"
      >
        {#snippet header()}
          <th class="px-4 py-3 font-medium">Permission</th>
          <th class="px-4 py-3 text-right font-medium">Override</th>
        {/snippet}
        {#snippet row(permission)}
          {@const state = getPermissionState(permission)}
          {@const inherited = getInheritedState(permission)}
          {@const isUpdating = updatingPermission === permission}
          {@const isDisabled = disabled || isUpdating}
          {@const showInherited = state === 'neutral' && inherited !== 'neutral' && !!inheritedFromLabel}

          <td class={['px-4 py-3', isUpdating ? 'animate-pulse' : '']}>
            <code
              class={[
                'text-sm',
                state === 'allow' ? 'text-success' : state === 'deny' ? 'text-danger' : ''
              ]}
            >
              {permission}
            </code>
            <div class="text-sm text-muted">{getPermissionDescription(permission)}</div>
            {#if showInherited}
              <div class="mt-1 text-xs">
                <span
                  class={[
                    'rounded px-1.5 py-0.5 font-medium',
                    inherited === 'allow'
                      ? 'bg-success/10 text-success'
                      : 'bg-danger/10 text-danger'
                  ]}
                >
                  Inherits {inherited === 'allow' ? 'Allow' : 'Deny'} from {inheritedFromLabel}
                </span>
              </div>
            {:else if state === 'deny'}
              <div class="mt-1 text-xs text-muted">Denied (overrides grants from other roles)</div>
            {/if}
          </td>
          <td class={['px-4 py-3', isUpdating ? 'animate-pulse' : '']}>
            <div class="flex items-center justify-end gap-4 text-sm">
              <label
                class={[
                  'flex items-center gap-1.5',
                  isDisabled ? 'cursor-not-allowed' : 'cursor-pointer'
                ]}
              >
                <input
                  type="checkbox"
                  checked={state === 'allow'}
                  disabled={isDisabled}
                  class="accent-success"
                  onchange={() => onSetState(permission, state === 'allow' ? 'neutral' : 'allow')}
                />
                <span class="text-success">Allow</span>
              </label>
              <label
                class={[
                  'flex items-center gap-1.5',
                  isDisabled ? 'cursor-not-allowed' : 'cursor-pointer'
                ]}
              >
                <input
                  type="checkbox"
                  checked={state === 'deny'}
                  disabled={isDisabled}
                  class="accent-danger"
                  onchange={() => onSetState(permission, state === 'deny' ? 'neutral' : 'deny')}
                />
                <span class="text-danger">Deny</span>
              </label>
            </div>
          </td>
        {/snippet}
      </DataTable>
    </div>
  {/each}
</div>
