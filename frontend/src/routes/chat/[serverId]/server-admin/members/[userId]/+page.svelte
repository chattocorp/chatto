<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { graphql } from '$lib/gql';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { CopyId, Panel } from '$lib/components/admin';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, Form, FormError, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { formatDate, formatDateTime } from '$lib/utils/formatTime';
  import { getLiveLogin } from '$lib/state/userProfiles.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import {
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin,
    getLoginChangeCooldownRemaining,
    formatCooldownRemaining
  } from '$lib/validation';

  type User = {
    id: string;
    login: string;
    displayName: string;
    avatarUrl?: string | null;
    roles: string[];
    createdAt?: string | null;
    deleted: boolean;
    hasVerifiedEmail: boolean;
    verifiedEmails: string[];
    viewerCanDeleteAccount: boolean;
    lastLoginChange?: string | null;
  };
  type Role = {
    name: string;
    displayName: string;
    position: number;
    permissions: string[];
    permissionDenials: string[];
  };
  // Everyone role is implicit for all server members and shouldn't be assignable
  const IMPLICIT_ROLES = ['everyone'];

  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const connection = useConnection();
  const userSettings = getUserSettings();
  const userId = $derived(page.params.userId!);

  const serverPerms = getServerPermissions();
  const canAdminManageUsers = $derived(serverPerms.current.canAdminManageUsers);

  let member = $state<User | null>(null);
  let allRoles = $state<Role[]>([]);
  let memberServerRoles = $state<string[]>([]); // Member's server roles (separate from member object)
  let canAssignRoles = $state(false);
  let canManageRoles = $state(false);
  let canManageUserPermissions = $state(false);
  let loading = $state(true);
  let updating = $state<string | null>(null);
  let error = $state<string | null>(null);

  // Identity edit state (gated on canAdminManageUsers)
  let editLogin = $state('');
  let editDisplayName = $state('');
  let savingIdentity = $state(false);
  let identityError = $state<string | null>(null);
  let lastLoginChange = $state<Date | null>(null);
  let clearingCooldown = $state(false);

  async function loadData() {
    error = null;

    const resp = await connection().client.query(
      graphql(`
        query ServerAdminMemberDetails($userId: ID!) {
          server {
            viewerCanAssignRoles
            viewerCanManageRoles
            viewerCanManageUserPermissions
            availablePermissions
            roles {
              name
              displayName
              position
              permissions
              permissionDenials
            }
            member(userId: $userId) {
              id
              login
              displayName
              avatarUrl
              roles
              createdAt
              deleted
              hasVerifiedEmail
              verifiedEmails
              viewerCanDeleteAccount
              lastLoginChange
            }
          }
        }
      `),
      { userId }
    );

    if (resp.error) {
      error = resp.error.message;
      loading = false;
      return;
    }

    if (!resp.data?.server) {
      error = 'Server not found';
      loading = false;
      return;
    }

    member = resp.data.server.member ?? null;
    allRoles = resp.data.server.roles ?? [];
    memberServerRoles = resp.data.server.member?.roles ?? [];
    canAssignRoles = resp.data.server.viewerCanAssignRoles;
    canManageRoles = resp.data.server.viewerCanManageRoles;
    canManageUserPermissions = resp.data.server.viewerCanManageUserPermissions;
    editLogin = resp.data.server.member?.login ?? '';
    editDisplayName = resp.data.server.member?.displayName ?? '';
    lastLoginChange = resp.data.server.member?.lastLoginChange
      ? new Date(resp.data.server.member.lastLoginChange)
      : null;
    loading = false;
  }

  // Identity edit derivations
  const loginModified = $derived(!!member && editLogin !== member.login);
  const displayNameModified = $derived(!!member && editDisplayName !== member.displayName);
  const identityModified = $derived(loginModified || displayNameModified);
  const cooldownRemaining = $derived(getLoginChangeCooldownRemaining(lastLoginChange));
  const cooldownActive = $derived(cooldownRemaining > 0);

  async function saveIdentity(e?: Event) {
    e?.preventDefault();
    if (!member || !identityModified || savingIdentity) return;

    identityError = null;

    const input: { userId: string; login?: string; displayName?: string } = { userId: member.id };

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
    const resp = await connection().client.mutation(
      graphql(`
        mutation AdminUpdateUser($input: AdminUpdateUserInput!) {
          admin {
            updateUser(input: $input) {
              id
              login
              displayName
            }
          }
        }
      `),
      { input }
    );
    savingIdentity = false;

    if (resp.error) {
      identityError = resp.error.message;
      return;
    }

    const updated = resp.data?.admin?.updateUser;
    if (updated && member) {
      member = { ...member, login: updated.login, displayName: updated.displayName };
      editLogin = updated.login;
      editDisplayName = updated.displayName;
      toast.success('User updated');
      // Refetch so the rest of the page (live-login lookups, role assignments)
      // sees the new identity without a manual reload.
      await loadData();
    }
  }

  function resetIdentity() {
    if (!member) return;
    editLogin = member.login;
    editDisplayName = member.displayName;
    identityError = null;
  }

  async function clearCooldown() {
    if (!member || clearingCooldown) return;
    clearingCooldown = true;
    const resp = await connection().client.mutation(
      graphql(`
        mutation AdminClearUsernameCooldown($input: ClearUsernameCooldownInput!) {
          admin {
            clearUsernameCooldown(input: $input)
          }
        }
      `),
      { input: { userId: member.id } }
    );
    clearingCooldown = false;

    if (resp.error) {
      identityError = resp.error.message;
      return;
    }
    if (resp.data?.admin?.clearUsernameCooldown) {
      lastLoginChange = null;
      toast.success('Username change cooldown cleared');
    }
  }

  // Check if user has a specific role (explicit assignment)
  function hasRole(roleName: string): boolean {
    return memberServerRoles.includes(roleName);
  }

  // Check if a role is implicit (always assigned to all members)
  function isImplicitRole(roleName: string): boolean {
    return IMPLICIT_ROLES.includes(roleName);
  }

  function getRoleDisplayName(roleName: string): string {
    const role = allRoles.find((r) => r.name === roleName);
    return role?.displayName || roleName;
  }

  function getRolePosition(roleName: string): number {
    const role = allRoles.find((r) => r.name === roleName);
    return role?.position ?? Number.MAX_SAFE_INTEGER;
  }

  // Check if this is the current user
  const isSelf = $derived(currentUser.user?.id === userId);
  const canViewMemberEmails = $derived(isSelf || serverPerms.current.canAdminViewUsers);

  // Sorted server roles (excluding everyone, sorted by position)
  const sortedServerRoles = $derived(
    memberServerRoles
      .filter((r) => r !== 'everyone')
      .sort((a, b) => getRolePosition(a) - getRolePosition(b))
  );
  const serverRoleCount = $derived(sortedServerRoles.length);
  const cooldownSummary = $derived.by(() => {
    if (cooldownActive) {
      return `Self-rename cooldown active for ${formatCooldownRemaining(cooldownRemaining)}.`;
    }
    if (lastLoginChange) {
      return `Last self-rename: ${formatDateTime(lastLoginChange, userSettings)}.`;
    }
    return 'No self-rename recorded.';
  });

  function formatOptionalDate(date: string | null | undefined): string {
    return date ? formatDate(date, userSettings) : 'Unknown';
  }

  function emailSummary(user: User): string {
    if (!canViewMemberEmails) return 'Email visibility unavailable';
    if (user.verifiedEmails.length > 0) return user.verifiedEmails.join(', ');
    if (user.hasVerifiedEmail) return 'Verified email on file';
    return 'No verified email';
  }

  async function toggleRole(roleName: string, currentlyHas: boolean) {
    if (!member) return;

    updating = roleName;
    error = null;

    const mutation = currentlyHas
      ? graphql(`
          mutation RevokeRoleFromMember($input: RevokeRoleInput!) {
            revokeRole(input: $input)
          }
        `)
      : graphql(`
          mutation AssignRoleToMember($input: AssignRoleInput!) {
            assignRole(input: $input)
          }
        `);

    const resp = await connection().client.mutation(mutation, {
      input: { userId: member.id, roleName }
    });

    if (resp.error) {
      error = resp.error.message;
    } else {
      const displayName = getRoleDisplayName(roleName);
      if (currentlyHas) {
        toast.success(`Removed ${displayName} role`);
      } else {
        toast.success(`Assigned ${displayName} role`);
      }
      // Reload to get updated state
      await loadData();
    }

    updating = null;
  }

  // Load data when params change
  $effect(() => {
    if (userId) {
      loadData();
    }
  });
</script>

<PageTitle title={`${member?.displayName ?? 'Member'} | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Member Details"
    subtitle={member?.displayName ?? 'Loading...'}
    backHref={resolve('/chat/[serverId]/server-admin/members', {
      serverId: serverIdToSegment(getActiveServer())
    })}
    backLabel="Back to Members"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading member...</div>
    {:else if !member}
      <Hint tone="danger">Member not found. They may have left the server.</Hint>
    {:else}
      {#if error}
        <FormError {error} />
      {/if}

      <!-- User Details -->
      <Panel title="User Details" icon="iconify uil--user">
        <div class="flex flex-col gap-6">
          <div class="flex flex-col gap-4 sm:flex-row sm:items-start">
            {#if member.avatarUrl}
              <img
                src={member.avatarUrl}
                alt={member.displayName}
                class="h-20 w-20 rounded-full border border-border object-cover"
              />
            {:else}
              <div
                class="flex h-20 w-20 shrink-0 items-center justify-center rounded-full bg-surface-300 text-3xl text-muted"
              >
                {getAvatarInitials(member.displayName, member.login)}
              </div>
            {/if}

            <div class="min-w-0 flex-1">
              <div class="flex flex-col gap-1">
                <h3 class="truncate text-2xl font-semibold">{member.displayName}</h3>
                <div class="truncate text-muted">@{getLiveLogin(member.id, member.login)}</div>
              </div>

              <div class="mt-4 flex flex-wrap gap-2">
                {#if member.deleted}
                  <Pill tone="danger">Deleted account</Pill>
                {:else}
                  <Pill tone="success">Member</Pill>
                {/if}
                {#if canViewMemberEmails}
                  <Pill tone={member.hasVerifiedEmail ? 'success' : 'muted'}>
                    {member.hasVerifiedEmail ? 'Email verified' : 'Email not verified'}
                  </Pill>
                {:else}
                  <Pill tone="muted">Email hidden</Pill>
                {/if}
                <Pill tone={serverRoleCount > 0 ? 'primary' : 'muted'}>
                  {serverRoleCount}
                  {serverRoleCount === 1 ? 'server role' : 'server roles'}
                </Pill>
                <Pill tone={member.viewerCanDeleteAccount ? 'danger' : 'muted'}>
                  {member.viewerCanDeleteAccount ? 'Deletion allowed' : 'Deletion protected'}
                </Pill>
                <Pill tone={cooldownActive ? 'accent' : 'muted'}>
                  {cooldownActive ? 'Rename cooldown' : 'Rename available'}
                </Pill>
              </div>
            </div>
          </div>

          <div class="grid gap-4 md:grid-cols-2">
            <div class="min-w-0">
              <div class="text-sm text-muted">User ID</div>
              <div class="mt-1 min-w-0">
                <CopyId value={member.id} />
              </div>
            </div>
            <div>
              <div class="text-sm text-muted">Joined</div>
              <div class="mt-1">{formatOptionalDate(member.createdAt)}</div>
            </div>
            <div class="min-w-0">
              <div class="text-sm text-muted">Verified email</div>
              <div class="mt-1 truncate" title={emailSummary(member)}>
                {emailSummary(member)}
              </div>
            </div>
            <div>
              <div class="text-sm text-muted">Username changes</div>
              <div class="mt-1">{cooldownSummary}</div>
            </div>
            <div class="min-w-0 md:col-span-2">
              <div class="text-sm text-muted">Server roles</div>
              <div class="mt-1 flex flex-wrap gap-1">
                {#each sortedServerRoles as roleName (roleName)}
                  <Pill tone="primary">{getRoleDisplayName(roleName)}</Pill>
                {/each}
                <Pill>Member</Pill>
              </div>
            </div>
          </div>
        </div>
      </Panel>

      {#if canAdminManageUsers}
        <!-- Identity (admin) — bypasses the 30-day rename cooldown -->
        <Panel title="Identity" icon="iconify uil--edit">
          <Form onsubmit={saveIdentity} error={identityError}>
            <TextInput
              id="member-login"
              testid="admin-identity-login"
              label="Username"
              bind:value={editLogin}
              disabled={savingIdentity}
              description="Admin renames bypass the 30-day cooldown."
            />
            <TextInput
              id="member-display-name"
              testid="admin-identity-display-name"
              label="Display Name"
              bind:value={editDisplayName}
              disabled={savingIdentity}
            />
            {#snippet footer()}
              <Button
                type="submit"
                disabled={!identityModified || savingIdentity}
                loading={savingIdentity}
                loadingText="Saving..."
              >
                Save
              </Button>
              <Button
                type="button"
                variant="ghost"
                onclick={resetIdentity}
                disabled={!identityModified || savingIdentity}
              >
                Reset
              </Button>
            {/snippet}
            <div class="flex items-center gap-3 surface-box p-3">
              <div class="flex-1 text-sm text-muted">
                {#if cooldownActive}
                  Self-rename cooldown active for this user — {formatCooldownRemaining(
                    cooldownRemaining
                  )} remaining.
                {:else if lastLoginChange}
                  Last self-rename: {lastLoginChange.toLocaleString()}.
                {:else}
                  User has never changed their username.
                {/if}
              </div>
              <Button
                type="button"
                variant="ghost"
                onclick={clearCooldown}
                disabled={!cooldownActive}
                loading={clearingCooldown}
                loadingText="Clearing..."
              >
                Reset cooldown
              </Button>
            </div>
          </Form>
        </Panel>
      {/if}

      <!-- Role Assignments -->
      <Panel title="Role Assignments" icon="iconify uil--shield-check">
        <p class="mb-4 text-sm text-muted">
          {#if canAssignRoles}
            Assign server roles to this member. Changes are saved immediately.
          {:else}
            View the server roles assigned to this member.
          {/if}
        </p>

        <div class="flex flex-col gap-2">
          {#each allRoles as role (role.name)}
            {@const isImplicit = isImplicitRole(role.name)}
            {@const has = isImplicit || hasRole(role.name)}
            {@const isUpdating = updating === role.name}
            {@const isSelfProtectedRole =
              isSelf && (role.name === 'admin' || role.name === 'owner') && has}
            {@const isDisabled = !canAssignRoles || isImplicit || isUpdating || isSelfProtectedRole}
            {@const tooltip = isImplicit
              ? 'All server members have this role implicitly'
              : isSelfProtectedRole
                ? `You cannot revoke your own ${role.displayName} role`
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
                  <div class="font-medium">{role.displayName}</div>
                  {#if isImplicit}
                    <div class="text-xs text-muted">Implicit for all members</div>
                  {/if}
                </div>
              </label>
              {#if canManageRoles}
                <a
                  href={resolve('/chat/[serverId]/server-admin/permissions/[name]', {
                    serverId: serverIdToSegment(getActiveServer()),
                    name: role.name
                  })}
                  class="shrink-0 text-sm link"
                >
                  Edit
                </a>
              {/if}
            </div>
          {/each}
        </div>
      </Panel>

      {#if canManageUserPermissions}
        <!-- Per-user permission overrides. -->
        <Hint>
          User-level overrides for this account. Any applicable deny wins over grants; use sparingly
          for per-user exceptions like suspensions or one-off elevations.
        </Hint>
        <UserPermissionsMatrix {userId} />
      {/if}
    {/if}
  </div>
</div>
