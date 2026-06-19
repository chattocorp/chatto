<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { Panel } from '$lib/components/admin';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, FormError, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { getLiveLogin } from '$lib/state/userProfiles.svelte';
  import {
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin,
    getLoginChangeCooldownRemaining,
    formatCooldownRemaining
  } from '$lib/validation';
  import {
    AdminClearUsernameCooldownRequest,
    AdminUpdateUserRequest,
    AssignMemberRoleRequest,
    GetAdminMemberRequest,
    RevokeMemberRoleRequest
  } from '$lib/pb/chatto/api/v1/chat_pb';
  import type { AdminMemberView, AdminRoleView } from '$lib/pb/chatto/api/v1/chat_pb';
  import { withActiveServerWireClient } from '$lib/wire/activeServerClient';

  type User = {
    id: string;
    login: string;
    displayName: string;
    avatarUrl?: string | null;
    roles: string[];
    lastLoginChange?: string | null;
  };
  type Role = {
    name: string;
    displayName: string;
    position: number;
    permissions: string[];
    permissionDenials: string[];
  };
  // Everyone role is implicit for all space members and shouldn't be assignable
  const IMPLICIT_ROLES = ['everyone'];

  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const userId = $derived(page.params.userId!);

  const serverPerms = getServerPermissions();
  const canAdminManageUsers = $derived(serverPerms.current.canAdminManageUsers);

  let member = $state<User | null>(null);
  let allRoles = $state<Role[]>([]);
  let memberSpaceRoles = $state<string[]>([]); // Member's space roles (separate from member object)
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
  let requestId = 0;
  let loadedUserId = '';

  async function loadData() {
    const targetUserId = userId;
    if (!targetUserId) return;

    const currentRequest = ++requestId;
    loading = true;
    error = null;

    try {
      const resp = await withActiveServerWireClient((client) =>
        client.getAdminMember(new GetAdminMemberRequest({ userId: targetUserId }))
      );
      if (currentRequest !== requestId) return;

      const nextMember = memberFromWire(resp.member);
      member = nextMember;
      allRoles = resp.roles.map(roleFromWire);
      memberSpaceRoles = nextMember?.roles ?? [];
      canAssignRoles = resp.viewerCanAssignRoles;
      canManageRoles = resp.viewerCanManageRoles;
      canManageUserPermissions = resp.viewerCanManageUserPermissions;
      editLogin = nextMember?.login ?? '';
      editDisplayName = nextMember?.displayName ?? '';
      lastLoginChange = resp.member?.lastLoginChange?.toDate() ?? null;
      loadedUserId = targetUserId;
    } catch (e) {
      if (currentRequest !== requestId) return;
      error = e instanceof Error ? e.message : 'Failed to load member';
      member = null;
    } finally {
      if (currentRequest === requestId) {
        loading = false;
      }
    }
  }

  afterNavigate(() => {
    if (userId && userId !== loadedUserId) {
      void loadData();
    }
  });

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
    try {
      const resp = await withActiveServerWireClient((client) =>
        client.adminUpdateUser(new AdminUpdateUserRequest(input))
      );
      const updated = memberFromWire(resp.member);
      if (updated && member) {
        member = updated;
        memberSpaceRoles = updated.roles;
        editLogin = updated.login;
        editDisplayName = updated.displayName;
        lastLoginChange = resp.member?.lastLoginChange?.toDate() ?? null;
      }
      toast.success('User updated');
    } catch (e) {
      identityError = e instanceof Error ? e.message : 'Failed to update user';
    } finally {
      savingIdentity = false;
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
    try {
      const resp = await withActiveServerWireClient((client) =>
        client.adminClearUsernameCooldown(
          new AdminClearUsernameCooldownRequest({ userId: member?.id ?? '' })
        )
      );
      const updated = memberFromWire(resp.member);
      if (updated) {
        member = updated;
        memberSpaceRoles = updated.roles;
      }
      lastLoginChange = null;
      toast.success('Username change cooldown cleared');
    } catch (e) {
      identityError = e instanceof Error ? e.message : 'Failed to clear username cooldown';
    } finally {
      clearingCooldown = false;
    }
  }

  // Check if user has a specific role (explicit assignment)
  function hasRole(roleName: string): boolean {
    return memberSpaceRoles.includes(roleName);
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

  // Sorted space roles (excluding everyone, sorted by position)
  const sortedSpaceRoles = $derived(
    memberSpaceRoles
      .filter((r) => r !== 'everyone')
      .sort((a, b) => getRolePosition(a) - getRolePosition(b))
  );

  async function toggleRole(roleName: string, currentlyHas: boolean) {
    if (!member) return;

    updating = roleName;
    error = null;

    try {
      const resp = await withActiveServerWireClient((client) =>
        currentlyHas
          ? client.revokeMemberRole(new RevokeMemberRoleRequest({ userId: member?.id ?? '', roleName }))
          : client.assignMemberRole(new AssignMemberRoleRequest({ userId: member?.id ?? '', roleName }))
      );
      const updated = memberFromWire(resp.member);
      if (updated) {
        member = updated;
        memberSpaceRoles = updated.roles;
      }
      const displayName = getRoleDisplayName(roleName);
      if (currentlyHas) {
        toast.success(`Removed ${displayName} role`);
      } else {
        toast.success(`Assigned ${displayName} role`);
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update role';
    } finally {
      updating = null;
    }
  }

  function memberFromWire(value: AdminMemberView | undefined): User | null {
    if (!value?.user) return null;
    return {
      id: value.user.id,
      login: value.user.login,
      displayName: value.user.displayName,
      avatarUrl: value.avatarUrl || null,
      roles: [...value.roles],
      lastLoginChange: value.lastLoginChange?.toDate().toISOString() ?? null
    };
  }

  function roleFromWire(value: AdminRoleView): Role {
    return {
      name: value.name,
      displayName: value.displayName,
      position: value.position,
      permissions: [...value.permissions],
      permissionDenials: [...value.permissionDenials]
    };
  }
</script>

<PageTitle title={`${member?.displayName ?? 'Member'} | Server Admin`} />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Member Details"
    subtitle={member?.displayName ?? 'Loading...'}
    backHref={resolve('/chat/[serverId]/server-admin/members', { serverId: serverIdToSegment(getActiveServer()) })}
    backLabel="Back to Members"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">Loading member...</div>
    {:else if !member}
      <Hint tone="danger">Member not found. They may have left the space.</Hint>
    {:else}
      {#if error}
        <FormError {error} />
      {/if}

      <!-- User Details -->
      <Panel title="User Details" icon="iconify uil--user">
        <div class="flex gap-6">
          {#if member.avatarUrl}
            <img
              src={member.avatarUrl}
              alt={member.displayName}
              class="h-20 w-20 rounded-full border border-border"
            />
          {:else}
            <div
              class="flex h-20 w-20 items-center justify-center rounded-full bg-surface-300 text-3xl text-muted"
            >
              {getAvatarInitials(member.displayName, member.login)}
            </div>
          {/if}
          <div class="flex flex-col gap-2">
            <div>
              <div class="text-sm text-muted">Login</div>
              <div class="font-medium">@{getLiveLogin(member.id, member.login)}</div>
            </div>
            <div>
              <div class="text-sm text-muted">Display Name</div>
              <div>{member.displayName}</div>
            </div>
            <div>
              <div class="text-sm text-muted">Space Roles</div>
              <div class="flex flex-wrap gap-1">
                {#each sortedSpaceRoles as roleName (roleName)}
                  <Pill>{getRoleDisplayName(roleName)}</Pill>
                {/each}
                <Pill>Member</Pill>
              </div>
            </div>
            <div>
              <div class="text-sm text-muted">ID</div>
              <code class="text-xs">{member.id}</code>
            </div>
          </div>
        </div>
      </Panel>

      {#if canAdminManageUsers}
        <!-- Identity (admin) — bypasses the 30-day rename cooldown -->
        <Panel title="Identity" icon="iconify uil--edit">
          <form class="flex flex-col gap-4" onsubmit={saveIdentity}>
            {#if identityError}
              <FormError error={identityError} />
            {/if}
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
            <div class="flex items-center gap-3">
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
            </div>
            <div class="flex items-center gap-3 rounded-lg border border-border bg-surface-100 p-3">
              <div class="flex-1 text-sm text-muted">
                {#if cooldownActive}
                  Self-rename cooldown active for this user — {formatCooldownRemaining(cooldownRemaining)} remaining.
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
          </form>
        </Panel>
      {/if}

      <!-- Role Assignments -->
      <Panel title="Role Assignments" icon="iconify uil--shield-check">
        <p class="mb-4 text-sm text-muted">
          {#if canAssignRoles}
            Assign roles to this member. Changes are saved immediately.
          {:else}
            View the roles assigned to this member.
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
              ? 'All space members have this role implicitly'
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
                  href={resolve('/chat/[serverId]/server-admin/permissions/[name]', { serverId: serverIdToSegment(getActiveServer()), name: role.name })}
                  class="link shrink-0 text-sm"
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
          User-level overrides for this account. Any applicable deny wins over grants; use
          sparingly for per-user exceptions like suspensions or one-off elevations.
        </Hint>
        <UserPermissionsMatrix {userId} />
      {/if}
    {/if}
  </div>
</div>
