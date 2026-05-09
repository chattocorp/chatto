<script lang="ts">

  import { resolve } from '$app/paths';
  import { graphql } from '$lib/gql';
  import { instanceIdToSegment } from '$lib/navigation';
  import { useQuery, useMutation } from '$lib/hooks';
  import { getAdminPermissions } from '$lib/state/instance/permissions.svelte';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { Panel } from '$lib/components/admin';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, FormError, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { CopyId } from '$lib/components/admin';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDateTime } from '$lib/utils/formatTime';
  import {
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin,
    getLoginChangeCooldownRemaining,
    formatCooldownRemaining
  } from '$lib/validation';

  const userSettings = getUserSettings();

  let { data } = $props();

  type User = {
    id: string;
    login: string;
    displayName: string;
    avatarUrl?: string | null;
    verifiedEmails: string[];
    createdAt?: string | null;
  };
  type Role = {
    name: string;
    displayName: string;
    description: string;
    isSystem: boolean;
    position: number;
    permissions: string[];
    permissionDenials: string[];
  };
  const getInstanceId = getActiveInstance();
  const adminPerms = getAdminPermissions();
  const canManage = $derived(adminPerms.hasPermission('admin.manage-users'));
  const canEditRoles = $derived(adminPerms.hasPermission('admin.manage-roles'));
  const currentUser = getCurrentUser();

  const userId = $derived(data.userId ?? '');

  let user = $state<User | null>(null);
  let allRoles = $state<Role[]>([]);
  let userRoles = $state<string[]>([]);
  let viewerRoles = $state<string[]>([]);
  let allPermissions = $state<string[]>([]);
  let updating = $state<string | null>(null);
  let error = $state<string | null>(null);

  // Identity edit state — kept in sync with the loaded user.
  let editLogin = $state('');
  let editDisplayName = $state('');
  let savingIdentity = $state(false);
  let identityError = $state<string | null>(null);
  let lastLoginChange = $state<Date | null>(null);
  let clearingCooldown = $state(false);

  // Load user details query
  const userQuery = useQuery(
    graphql(`
      query AdminUserDetails($userId: ID!) {
        me {
          roles
        }
        user(id: $userId) {
          id
          login
          displayName
          avatarUrl
          verifiedEmails
          createdAt
          lastLoginChange
        }
        admin {
          roles {
            name
            displayName
            description
            isSystem
            position
            permissions
            permissionDenials
          }
          instancePermissions
          userInstanceRoles(userId: $userId)
          userRoleBasedPermissions(userId: $userId)
          userRoleBasedDenials(userId: $userId)
        }
      }
    `),
    () => ({ userId }),
    {
      skip: () => !userId,
      onCompleted: (data) => {
        user = data.user ?? null;
        allRoles = data.admin?.roles ?? [];
        allPermissions = data.admin?.instancePermissions ?? [];
        viewerRoles = data.me?.roles ?? [];
        userRoles = data.admin?.userInstanceRoles ?? [];
        editLogin = data.user?.login ?? '';
        editDisplayName = data.user?.displayName ?? '';
        lastLoginChange = data.user?.lastLoginChange
          ? new Date(data.user.lastLoginChange)
          : null;
      }
    }
  );

  // Role assignment mutations
  const assignRoleMutation = useMutation(
    graphql(`
      mutation AssignInstanceRole($input: AssignInstanceRoleInput!) {
        assignInstanceRole(input: $input)
      }
    `)
  );

  const revokeRoleMutation = useMutation(
    graphql(`
      mutation RevokeInstanceRole($input: RevokeInstanceRoleInput!) {
        revokeInstanceRole(input: $input)
      }
    `)
  );

  const adminUpdateUserMutation = useMutation(
    graphql(`
      mutation AdminUpdateUser($input: AdminUpdateUserInput!) {
        admin {
          updateUser(input: $input) {
            id
            login
            displayName
            lastLoginChange
          }
        }
      }
    `)
  );

  const adminClearCooldownMutation = useMutation(
    graphql(`
      mutation AdminClearUsernameCooldown($userId: ID!) {
        admin {
          clearUsernameCooldown(userId: $userId)
        }
      }
    `)
  );

  function hasRole(roleName: string): boolean {
    return userRoles.includes(roleName);
  }

  async function toggleRole(roleName: string, currentlyHas: boolean) {
    if (!user) return;

    updating = roleName;
    error = null;

    const result = currentlyHas
      ? await revokeRoleMutation.execute({ input: { userId: user.id, roleName } })
      : await assignRoleMutation.execute({ input: { userId: user.id, roleName } });

    if (result.error) {
      error = result.error;
    } else {
      const role = allRoles.find((r) => r.name === roleName);
      const displayName = role?.displayName ?? roleName;

      if (currentlyHas) {
        userRoles = userRoles.filter((r) => r !== roleName);
        toast.success(`Removed ${displayName} role`);
      } else {
        userRoles = [...userRoles, roleName];
        toast.success(`Assigned ${displayName} role`);
      }
      // Reload to get updated role-based permissions
      await userQuery.refetch();
    }

    updating = null;
  }

  // Filter out member role (it's implicit) - keep everyone for display
  const assignableRoles = $derived(allRoles.filter((r) => r.name !== 'member'));

  // Implicit roles that can't be toggled (universal role: everyone)
  const isImplicitRole = (roleName: string) => roleName === 'everyone';

  // Check if user has this implicit role
  // - everyone: always true (all authenticated users)
  const hasImplicitRole = (roleName: string) => {
    if (roleName === 'everyone') return true;
    return false;
  };

  // All roles the user effectively has (explicit + implicit)
  const effectiveUserRoles = $derived.by(() => {
    const roles = [...userRoles];
    for (const role of allRoles) {
      if (isImplicitRole(role.name) && hasImplicitRole(role.name) && !roles.includes(role.name)) {
        roles.push(role.name);
      }
    }
    return roles;
  });

  // Role hierarchy helpers - lower position = higher rank
  function getRolePosition(roleName: string): number {
    return allRoles.find((r) => r.name === roleName)?.position ?? Infinity;
  }

  function getViewerBestPosition(): number {
    return Math.min(...viewerRoles.map(getRolePosition), Infinity);
  }

  // Check if viewer can manage target user based on role hierarchy
  // Viewer can manage if their best role has a lower position (higher rank) than target's best role
  function computeCanManageUser(targetRoleNames: string[]): boolean {
    const viewerBest = getViewerBestPosition();
    const targetBest = Math.min(...targetRoleNames.map(getRolePosition), Infinity);
    return viewerBest < targetBest;
  }

  // Check if viewer can assign/revoke a specific role
  // Viewer can assign roles at or below their rank (position >= viewer's best position)
  function canAssignRole(roleName: string): boolean {
    const viewerBest = getViewerBestPosition();
    const rolePosition = getRolePosition(roleName);
    return viewerBest <= rolePosition;
  }

  // Check if this is the current user (for self-lockout warning)
  const isSelf = $derived(currentUser.user?.id === userId);

  // Derived: whether viewer can manage this user based on role hierarchy
  // Self-management is always allowed (isSelfOwnerOrAdmin handles the admin role protection separately)
  const viewerCanManageUser = $derived(isSelf || computeCanManageUser(userRoles));

  function formatDate(dateStr: string | null | undefined): string {
    if (!dateStr) return '—';
    return formatDateTime(dateStr, userSettings);
  }

  // Identity edit helpers
  const loginModified = $derived(!!user && editLogin !== user.login);
  const displayNameModified = $derived(!!user && editDisplayName !== user.displayName);
  const identityModified = $derived(loginModified || displayNameModified);

  // Cooldown is informational here — the admin path bypasses it. We still show
  // remaining time so the admin knows whether the user is currently blocked
  // from renaming themselves.
  const cooldownRemaining = $derived(getLoginChangeCooldownRemaining(lastLoginChange));
  const cooldownActive = $derived(cooldownRemaining > 0);

  async function saveIdentity(e?: Event) {
    e?.preventDefault();
    if (!user || !identityModified || savingIdentity) return;

    identityError = null;

    const input: { userId: string; login?: string; displayName?: string } = { userId: user.id };

    if (displayNameModified) {
      const v = validateAndNormalizeDisplayName(editDisplayName);
      if (!v.valid || v.normalized === undefined) {
        identityError = v.error ?? 'Invalid display name';
        return;
      }
      input.displayName = v.normalized;
    }

    if (loginModified) {
      const v = validateAndNormalizeLogin(editLogin);
      if (!v.valid || v.normalized === undefined) {
        identityError = v.error ?? 'Invalid username';
        return;
      }
      input.login = v.normalized;
    }

    savingIdentity = true;
    const result = await adminUpdateUserMutation.execute({ input });
    savingIdentity = false;

    if (result.error) {
      identityError = result.error;
      return;
    }

    const updated = result.data?.admin?.updateUser;
    if (updated) {
      user = { ...user, login: updated.login, displayName: updated.displayName };
      editLogin = updated.login;
      editDisplayName = updated.displayName;
      // Admin edits do not advance the cooldown timestamp; keep current value.
      toast.success('User updated');
      // Re-fetch to refresh related fields (avatar URL stays, but role-based
      // permissions are unaffected).
      await userQuery.refetch();
    }
  }

  function resetIdentity() {
    if (!user) return;
    editLogin = user.login;
    editDisplayName = user.displayName;
    identityError = null;
  }

  async function clearCooldown() {
    if (!user || clearingCooldown) return;
    clearingCooldown = true;
    const result = await adminClearCooldownMutation.execute({ userId: user.id });
    clearingCooldown = false;

    if (result.error) {
      identityError = result.error;
      return;
    }
    if (result.data?.admin?.clearUsernameCooldown) {
      lastLoginChange = null;
      toast.success('Username change cooldown cleared');
    }
  }
</script>

<PageTitle title={`${user?.displayName ?? 'Manage User'} | Admin`} />

<PaneHeader
  title="Manage User"
  subtitle={user?.displayName ?? 'Loading...'}
  backHref={resolve('/chat/[instanceId]/admin/users', { instanceId: instanceIdToSegment(getInstanceId()) })}
  backLabel="Back to users"
  showMobileNav
/>

<div class="flex flex-col gap-6 overflow-y-auto p-6">
  {#if userQuery.loading}
    <div class="text-muted">Loading user...</div>
  {:else if !user}
    <div class="text-danger">User not found</div>
  {:else if !canManage}
    <div class="text-danger">
      You need the <code class="rounded bg-surface-200 px-1">admin.manage-users</code> permission to manage
      users.
    </div>
  {:else}
    {#if error}
      <FormError {error} />
    {/if}

    <!-- User Details -->
    <Panel title="User Details" icon="iconify uil--user">
      <div class="flex gap-6">
        {#if user.avatarUrl}
          <img
            src={user.avatarUrl}
            alt={user.displayName}
            class="h-20 w-20 rounded-full border border-border"
          />
        {:else}
          <div
            class="flex h-20 w-20 items-center justify-center rounded-full bg-surface-300 text-3xl text-muted"
          >
            {getAvatarInitials(user.displayName, user.login)}
          </div>
        {/if}
        <div class="flex flex-col gap-2">
          <div>
            <div class="text-sm text-muted">Username</div>
            <div class="font-medium">{user.login}</div>
          </div>
          <div>
            <div class="text-sm text-muted">Display Name</div>
            <div>{user.displayName}</div>
          </div>
          <div>
            <div class="text-sm text-muted">Verified Emails</div>
            {#if user.verifiedEmails.length > 0}
              <div class="flex flex-col gap-1">
                {#each user.verifiedEmails as email (email)}
                  <div class="flex items-center gap-1.5">
                    <span class="iconify text-success uil--check-circle"></span>
                    <span>{email}</span>
                  </div>
                {/each}
              </div>
            {:else}
              <div class="text-muted italic">No verified emails</div>
            {/if}
          </div>
          <div>
            <div class="text-sm text-muted">Account Created</div>
            <div>{formatDate(user.createdAt)}</div>
          </div>
          <div>
            <div class="text-sm text-muted">ID</div>
            <CopyId value={user.id} />
          </div>
        </div>
      </div>
    </Panel>

    <!-- Identity -->
    <Panel title="Identity" icon="iconify uil--edit">
      {#if !viewerCanManageUser}
        <p class="text-sm text-muted">
          You cannot edit this user's identity because their highest role outranks yours.
        </p>
      {:else}
        {#if isSelf}
          <p class="mb-4 text-sm text-muted">You're editing your own account.</p>
        {/if}

        {#if identityError}
          <FormError error={identityError} />
        {/if}

        <form onsubmit={saveIdentity} class="flex max-w-md flex-col gap-4">
          <TextInput
            label="Username"
            testid="admin-identity-login"
            bind:value={editLogin}
            description="Bypasses the 30-day cooldown but still validates against the blocked-username list."
            disabled={savingIdentity}
          />
          <TextInput
            label="Display Name"
            testid="admin-identity-display-name"
            bind:value={editDisplayName}
            disabled={savingIdentity}
          />

          <div class="flex gap-2">
            <Button
              type="submit"
              variant="primary"
              disabled={!identityModified || savingIdentity}
              loading={savingIdentity}
              loadingText="Saving…"
            >
              <span class="iconify uil--check"></span>
              Save
            </Button>
            {#if identityModified}
              <Button variant="secondary" onclick={resetIdentity} disabled={savingIdentity}>
                <span class="iconify uil--times"></span>
                Cancel
              </Button>
            {/if}
          </div>
        </form>

        <hr class="my-6 border-border" />

        <div class="flex items-center justify-between gap-4">
          <div>
            <div class="text-sm text-muted">Username Cooldown</div>
            {#if cooldownActive && lastLoginChange}
              <div>
                User must wait {formatCooldownRemaining(cooldownRemaining)} before they can rename
                themselves again.
              </div>
              <div class="text-sm text-muted">
                Last change: {formatDate(lastLoginChange.toISOString())}
              </div>
            {:else if lastLoginChange}
              <div>No active cooldown — last change: {formatDate(lastLoginChange.toISOString())}.</div>
            {:else}
              <div>User has never changed their username.</div>
            {/if}
          </div>
          <Button
            variant="secondary"
            onclick={clearCooldown}
            disabled={!cooldownActive || clearingCooldown}
          >
            {clearingCooldown ? 'Clearing…' : 'Reset cooldown'}
          </Button>
        </div>
      {/if}
    </Panel>

    <!-- Role Assignments -->
    <Panel title="Role Assignments" icon="iconify uil--shield-check">
      <p class="mb-4 text-sm text-muted">
        {#if viewerCanManageUser}
          Assign instance roles to this user. Changes are saved immediately.
        {:else}
          You cannot modify roles for this user because their highest role outranks yours.
        {/if}
      </p>

      <div class="flex flex-col gap-2">
        {#each assignableRoles as role (role.name)}
          {@const isImplicit = isImplicitRole(role.name)}
          {@const has = isImplicit ? hasImplicitRole(role.name) : hasRole(role.name)}
          {@const isUpdating = updating === role.name}
          {@const isSelfOwnerOrAdmin =
            isSelf && (role.name === 'instance-owner' || role.name === 'instance-admin') && has}
          {@const canModifyRole = canAssignRole(role.name)}
          {@const isDisabled =
            isUpdating ||
            isSelfOwnerOrAdmin ||
            isImplicit ||
            !viewerCanManageUser ||
            !canModifyRole}
          {@const tooltip = isSelfOwnerOrAdmin
            ? `You cannot revoke your own ${role.name === 'instance-owner' ? 'owner' : 'admin'} role`
            : isImplicit
              ? 'This role is assigned automatically and cannot be changed'
              : !viewerCanManageUser
                ? "This user's role outranks yours"
                : !canModifyRole
                  ? 'You cannot modify roles that outrank yours'
                  : ''}

          <div
            class={[
              'flex items-center gap-3 rounded-lg border border-border p-3',
              isDisabled ? 'opacity-50' : ''
            ]}
          >
            <label
              class={[
                'flex flex-1 items-center gap-3',
                isDisabled ? 'cursor-not-allowed' : 'cursor-pointer'
              ]}
              title={tooltip}
            >
              <input
                type="checkbox"
                checked={has}
                disabled={isDisabled}
                class={[
                  'h-5 w-5',
                  isDisabled ? 'cursor-not-allowed' : 'cursor-pointer',
                  isUpdating ? 'animate-pulse' : ''
                ]}
                onchange={() => toggleRole(role.name, has)}
              />
              <div class="flex-1">
                <div class="font-medium">
                  {role.displayName}
                  {#if isImplicit}
                    <span class="ml-1 text-xs text-muted">(automatic)</span>
                  {/if}
                </div>
                <div class="text-sm text-muted">{role.description}</div>
              </div>
            </label>
            {#if canEditRoles}
              <a
                href={resolve('/chat/[instanceId]/admin/roles/[name]', { instanceId: instanceIdToSegment(getInstanceId()), name: role.name })}
                class="shrink-0 text-sm text-primary hover:underline"
              >
                Edit
              </a>
            {/if}
          </div>
        {/each}
      </div>

      <p class="mt-4 text-sm text-muted">
        Note: Everyone and Verified roles are assigned automatically based on authentication and
        email verification status.
      </p>
    </Panel>

    <!-- Effective Permissions: hand off to the inspector for the full trace -->
    <Panel title="Effective Permissions" icon="iconify uil--lock-access">
      <p class="mb-4 text-sm text-muted">
        Open the Permission Inspector to see every permission this user has, with the role and
        level (instance/space/room) that decided each call.
      </p>
      <Button
        variant="primary"
        href={resolve('/chat/[instanceId]/admin/inspector', {
          instanceId: instanceIdToSegment(getInstanceId())
        }) + `?userId=${userId}`}
      >
        Open in Permission Inspector
      </Button>
    </Panel>
  {/if}
</div>
