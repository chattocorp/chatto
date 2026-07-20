<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import {
    createAdminUserManagementAPI,
    type AdminMember,
    type AdminRoleDetails
  } from '$lib/api-client/adminUsers';
  import { getServerPermissions } from '$lib/state/server/permissions.svelte';
  import { CopyId, Panel } from '$lib/components/admin';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { Hint, Pill } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button, Checkbox, Form, FormError, TextInput, z, validate } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { getAvatarInitials } from '$lib/utils/initials';
  import { formatDate, formatDateTime } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { getLiveLogin } from '$lib/state/userProfiles.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import * as m from '$lib/i18n/messages';
  import {
    validateAndNormalizeDisplayName,
    validateAndNormalizeLogin,
    getLoginChangeCooldownRemaining,
    formatCooldownRemaining
  } from '$lib/validation';

  // Everyone role is implicit for all server members and shouldn't be assignable
  const IMPLICIT_ROLES = ['everyone'];

  const currentUser = $derived(serverRegistry.getStore(getActiveServer()).currentUser);
  const connection = useConnection();
  const userSettings = getUserSettings();
  const activeLocale = $derived(getLocale());
  const userId = $derived(page.params.userId!);

  const serverPerms = getServerPermissions();
  const canAdminManageAccounts = $derived(serverPerms.current.canAdminManageAccounts);

  let member = $state<AdminMember | null>(null);
  let allRoles = $state<AdminRoleDetails[]>([]);
  let memberServerRoles = $state<string[]>([]); // Member's roles (separate from member object)
  let canAssignRoles = $state(false);
  let canManageRoles = $state(false);
  let canManageUserPermissions = $state(false);
  let assignableRoleNames = $state<string[] | null>(null);
  let revocableRoleNames = $state<string[] | null>(null);
  let loading = $state(true);
  let updating = $state<string | null>(null);
  let error = $state<string | null>(null);

  // Identity edit state (gated on canAdminManageAccounts)
  let editLogin = $state('');
  let editDisplayName = $state('');
  let savingIdentity = $state(false);
  let identityError = $state<string | null>(null);
  let lastLoginChange = $state<Date | null>(null);
  let clearingCooldown = $state(false);
  let adminPassword = $state('');
  let adminConfirmPassword = $state('');
  let passwordError = $state<string | null>(null);
  let settingPassword = $state(false);

  async function loadData() {
    error = null;

    try {
      const resp = await adminUsersAPI().getMember(userId);

      member = resp.member;
      allRoles = resp.roles;
      memberServerRoles = resp.member?.roles ?? [];
      canAssignRoles = resp.viewerCanAssignRoles;
      canManageRoles = resp.viewerCanManageRoles;
      canManageUserPermissions = resp.viewerCanManageUserPermissions;
      assignableRoleNames = resp.assignableRoleNames;
      revocableRoleNames = resp.revocableRoleNames;
      editLogin = resp.member?.login ?? '';
      editDisplayName = resp.member?.displayName ?? '';
      lastLoginChange = resp.member?.lastLoginChange ? new Date(resp.member.lastLoginChange) : null;
    } catch (err) {
      error = err instanceof Error ? err.message : m['admin.members.load_failed']();
    } finally {
      loading = false;
    }
  }

  // Identity edit derivations
  const loginModified = $derived(!!member && editLogin !== member.login);
  const displayNameModified = $derived(!!member && editDisplayName !== member.displayName);
  const identityModified = $derived(loginModified || displayNameModified);
  const cooldownRemaining = $derived(getLoginChangeCooldownRemaining(lastLoginChange));
  const cooldownActive = $derived(cooldownRemaining > 0);
  const passwordSchema = z.string().min(8, m['common.validation.password_min']());
  const adminPasswordValidationError = $derived(
    adminPassword ? validate(passwordSchema, adminPassword) : undefined
  );
  const adminConfirmPasswordError = $derived(
    adminConfirmPassword && adminPassword !== adminConfirmPassword
      ? m['common.validation.passwords_match']()
      : undefined
  );
  const canSetMemberPassword = $derived(
    !!member &&
      adminPassword !== '' &&
      adminConfirmPassword !== '' &&
      !adminPasswordValidationError &&
      !adminConfirmPasswordError &&
      !settingPassword
  );

  function adminUsersAPI() {
    const conn = connection();
    return createAdminUserManagementAPI({
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
  }

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
    let updated: { login: string; displayName: string } | null = null;
    try {
      updated = await adminUsersAPI().updateUser(input);
    } catch (err) {
      identityError = err instanceof Error ? err.message : 'Failed to update user';
    }
    savingIdentity = false;

    if (!updated) {
      return;
    }

    if (member) {
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
    identityError = null;
    let cleared = false;
    try {
      cleared = await adminUsersAPI().clearUsernameCooldown(member.id);
    } catch (err) {
      identityError = err instanceof Error ? err.message : 'Failed to clear username cooldown';
    }
    clearingCooldown = false;

    if (!cleared) {
      return;
    }
    lastLoginChange = null;
    toast.success('Username change cooldown cleared');
  }

  async function setMemberPassword(e?: Event) {
    e?.preventDefault();
    if (!member || !canSetMemberPassword) {
      passwordError =
        adminPasswordValidationError ||
        adminConfirmPasswordError ||
        m['common.validation.fix_errors']();
      return;
    }

    settingPassword = true;
    passwordError = null;
    let updatedMember: AdminMember | null = null;
    try {
      updatedMember = await adminUsersAPI().updateUserPassword(member.id, adminPassword);
    } catch (err) {
      passwordError = err instanceof Error ? err.message : m['admin.members.set_password_failed']();
    }
    settingPassword = false;

    if (!updatedMember) {
      return;
    }
    member = updatedMember;
    adminPassword = '';
    adminConfirmPassword = '';
    toast.success(m['admin.members.password_set']());
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

  // Sorted roles (excluding everyone, sorted by position)
  const sortedServerRoles = $derived(
    memberServerRoles
      .filter((r) => r !== 'everyone')
      .sort((a, b) => getRolePosition(a) - getRolePosition(b))
  );
  const serverRoleCount = $derived(sortedServerRoles.length);
  const cooldownSummary = $derived.by(() => {
    if (cooldownActive) {
      return m['admin.member_detail.self_rename_cooldown']({
        remaining: formatCooldownRemaining(cooldownRemaining)
      });
    }
    if (lastLoginChange) {
      return m['admin.member_detail.last_self_rename']({
        time: formatDateTime(lastLoginChange, userSettings, activeLocale)
      });
    }
    return m['admin.member_detail.no_self_rename']();
  });

  function formatOptionalDate(date: string | null | undefined): string {
    return date ? formatDate(date, userSettings, activeLocale) : m['admin.common.unknown']();
  }

  function emailSummary(user: AdminMember): string {
    if (!canViewMemberEmails) return m['admin.member_detail.email_unavailable']();
    if (user.verifiedEmails.length > 0) return user.verifiedEmails.join(', ');
    if (user.hasVerifiedEmail) return m['admin.member_detail.verified_email_on_file']();
    return m['admin.member_detail.no_verified_email']();
  }

  async function toggleRole(roleName: string, currentlyHas: boolean) {
    if (!member) return;

    updating = roleName;
    error = null;

    try {
      const result = currentlyHas
        ? await adminUsersAPI().revokeRole(member.id, roleName)
        : await adminUsersAPI().assignRole(member.id, roleName);
      if (result.changed) {
        const displayName = getRoleDisplayName(roleName);
        if (currentlyHas) {
          toast.success(m['admin.members.removed_role']({ role: displayName }));
        } else {
          toast.success(m['admin.members.assigned_role']({ role: displayName }));
        }
        if (result.member) {
          member = result.member;
          memberServerRoles = result.member.roles;
        } else {
          await loadData();
        }
      }
    } catch (err) {
      error = err instanceof Error ? err.message : m['admin.members.role_update_failed']();
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

<PageTitle
  title={m['admin.common.server_admin_page_title']({
    title: member?.displayName ?? m['admin.members.member_fallback']()
  })}
/>

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title={m['admin.members.member_details']()}
    subtitle={member?.displayName ?? m['common.loading']()}
    backHref={resolve('/chat/[serverId]/manage/server/members', {
      serverId: serverIdToSegment(getActiveServer())
    })}
    backLabel={m['admin.members.back_to_members']()}
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    {#if loading}
      <div class="text-muted">{m['admin.members.loading_member']()}</div>
    {:else if !member}
      <Hint tone="danger">{m['admin.members.not_found']()}</Hint>
    {:else}
      {#if error}
        <FormError {error} />
      {/if}

      <!-- User Details -->
      <Panel title={m['admin.members.user_details']()} icon="iconify uil--user">
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
                class="flex h-20 w-20 shrink-0 items-center justify-center rounded-full bg-surface-strong text-3xl text-muted"
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
                  <Pill tone="danger">{m['admin.members.deleted_account']()}</Pill>
                {:else}
                  <Pill tone="success">{m['admin.members.member']()}</Pill>
                {/if}
                {#if canViewMemberEmails}
                  <Pill tone={member.hasVerifiedEmail ? 'success' : 'muted'}>
                    {member.hasVerifiedEmail
                      ? m['admin.members.email_verified']()
                      : m['admin.members.email_not_verified']()}
                  </Pill>
                {:else}
                  <Pill tone="muted">{m['admin.members.email_hidden']()}</Pill>
                {/if}
                <Pill tone={serverRoleCount > 0 ? 'neutral' : 'muted'}>
                  {serverRoleCount === 1
                    ? m['admin.members.server_role_one']()
                    : m['admin.members.server_role_many']({ count: serverRoleCount })}
                </Pill>
                <Pill tone={member.viewerCanDeleteAccount ? 'danger' : 'muted'}>
                  {member.viewerCanDeleteAccount
                    ? m['admin.members.deletion_allowed']()
                    : m['admin.members.deletion_protected']()}
                </Pill>
                <Pill tone={cooldownActive ? 'action' : 'muted'}>
                  {cooldownActive
                    ? m['admin.members.rename_cooldown']()
                    : m['admin.members.rename_available']()}
                </Pill>
              </div>
            </div>
          </div>

          <div class="grid gap-4 md:grid-cols-2">
            <div class="min-w-0">
              <div class="text-sm text-muted">{m['admin.members.user_id']()}</div>
              <div class="mt-1 min-w-0">
                <CopyId value={member.id} />
              </div>
            </div>
            <div>
              <div class="text-sm text-muted">{m['admin.common.joined']()}</div>
              <div class="mt-1">{formatOptionalDate(member.createdAt)}</div>
            </div>
            <div class="min-w-0">
              <div class="text-sm text-muted">{m['admin.members.verified_email']()}</div>
              <div class="mt-1 truncate" title={emailSummary(member)}>
                {emailSummary(member)}
              </div>
            </div>
            <div>
              <div class="text-sm text-muted">{m['admin.members.username_changes']()}</div>
              <div class="mt-1">{cooldownSummary}</div>
            </div>
            <div class="min-w-0 md:col-span-2">
              <div class="text-sm text-muted">{m['admin.members.server_roles']()}</div>
              <div class="mt-1 flex flex-wrap gap-1">
                {#each sortedServerRoles as roleName (roleName)}
                  <Pill tone="neutral">{getRoleDisplayName(roleName)}</Pill>
                {/each}
                <Pill>{m['admin.members.member']()}</Pill>
              </div>
            </div>
          </div>
        </div>
      </Panel>

      {#if canAdminManageAccounts}
        <!-- Identity (admin) — bypasses the 30-day rename cooldown -->
        <Panel title={m['admin.members.identity']()} icon="iconify uil--edit">
          <Form onsubmit={saveIdentity} error={identityError}>
            <TextInput
              id="member-login"
              testid="admin-identity-login"
              label={m['common.username']()}
              bind:value={editLogin}
              disabled={savingIdentity}
              description={m['admin.members.admin_rename_description']()}
            />
            <TextInput
              id="member-display-name"
              testid="admin-identity-display-name"
              label={m['settings.profile.display_name.label']()}
              bind:value={editDisplayName}
              disabled={savingIdentity}
            />
            {#snippet footer()}
              <Button
                type="submit"
                disabled={!identityModified || savingIdentity}
                loading={savingIdentity}
                loadingText={m['rbac.role_form.saving']()}
              >
                {m['rbac.role_form.save']()}
              </Button>
              <Button
                type="button"
                variant="ghost"
                onclick={resetIdentity}
                disabled={!identityModified || savingIdentity}
              >
                {m['admin.members.reset']()}
              </Button>
            {/snippet}
            <div class="flex items-center gap-3 surface-box p-3">
              <div class="flex-1 text-sm text-muted">
                {#if cooldownActive}
                  {m['admin.members.cooldown_active']({
                    remaining: formatCooldownRemaining(cooldownRemaining)
                  })}
                {:else if lastLoginChange}
                  {m['admin.members.last_self_rename']({
                    time: formatDateTime(lastLoginChange, userSettings, activeLocale)
                  })}
                {:else}
                  {m['admin.members.never_renamed']()}
                {/if}
              </div>
              <Button
                type="button"
                variant="ghost"
                onclick={clearCooldown}
                disabled={!cooldownActive}
                loading={clearingCooldown}
                loadingText={m['admin.members.clearing']()}
              >
                {m['admin.members.reset_cooldown']()}
              </Button>
            </div>
          </Form>
          {#if !isSelf}
            <form
              class="mt-6 flex flex-col gap-4 border-t border-border pt-6"
              onsubmit={setMemberPassword}
            >
              <div>
                <h4 class="text-sm font-semibold">{m['admin.members.set_password']()}</h4>
                <p class="mt-1 text-sm text-muted">
                  {m['admin.members.set_password_description']()}
                </p>
              </div>
              <TextInput
                id="admin-member-password"
                label={m['common.new_password']()}
                type="password"
                bind:value={adminPassword}
                placeholder={m['common.password_min_placeholder']()}
                disabled={settingPassword}
                autocomplete="new-password"
                error={adminPasswordValidationError}
              />
              <TextInput
                id="admin-member-password-confirm"
                label={m['common.confirm_password']()}
                type="password"
                bind:value={adminConfirmPassword}
                placeholder={m['common.password_confirm_placeholder']()}
                disabled={settingPassword}
                autocomplete="new-password"
                error={adminConfirmPasswordError}
              />
              {#if passwordError}
                <FormError error={passwordError} />
              {/if}
              <div>
                <Button
                  type="submit"
                  loading={settingPassword}
                  loadingText={m['admin.members.setting_password']()}
                  disabled={!canSetMemberPassword}
                >
                  <span class="iconify mdi--key-change"></span>
                  {m['admin.members.set_password']()}
                </Button>
              </div>
            </form>
          {/if}
        </Panel>
      {/if}

      <!-- Role Assignments -->
      <Panel title={m['admin.members.role_assignments']()} icon="iconify uil--shield-check">
        <p class="mb-4 text-sm text-muted">
          {#if canAssignRoles}
            {m['admin.members.assign_roles_description']()}
          {:else}
            {m['admin.members.view_roles_description']()}
          {/if}
        </p>

        <div class="flex flex-col gap-2">
          {#each allRoles as role (role.name)}
            {@const isImplicit = isImplicitRole(role.name)}
            {@const has = isImplicit || hasRole(role.name)}
            {@const isUpdating = updating === role.name}
            {@const isSelfProtectedRole =
              isSelf && (role.name === 'admin' || role.name === 'owner') && has}
            {@const isWithinAssignmentAuthority = has
              ? revocableRoleNames === null || revocableRoleNames.includes(role.name)
              : assignableRoleNames === null || assignableRoleNames.includes(role.name)}
            {@const isDisabled =
              !canAssignRoles ||
              isImplicit ||
              isUpdating ||
              isSelfProtectedRole ||
              !isWithinAssignmentAuthority}
            {@const tooltip = isImplicit
              ? m['admin.members.implicit_role_tooltip']()
              : isSelfProtectedRole
                ? m['admin.members.cannot_revoke_own_role']({ role: role.displayName })
                : !isWithinAssignmentAuthority
                  ? m['ui.access_denied.message']()
                  : ''}

            <div class="flex items-center gap-3">
              <div class="min-w-0 flex-1" title={tooltip}>
                <Checkbox
                  id={`role-assignment-${role.name}`}
                  checked={has}
                  disabled={isDisabled}
                  loading={isUpdating}
                  onchange={() => toggleRole(role.name, has)}
                >
                  <span class="block">{role.displayName}</span>
                  {#if isImplicit}
                    <span class="block text-xs font-normal text-muted">
                      {m['admin.members.implicit_all_members']()}
                    </span>
                  {/if}
                </Checkbox>
              </div>
              {#if canManageRoles}
                <a
                  href={resolve('/chat/[serverId]/manage/server/permissions/[name]', {
                    serverId: serverIdToSegment(getActiveServer()),
                    name: role.name
                  })}
                  class="shrink-0 text-sm link"
                >
                  {m['admin.members.edit']()}
                </a>
              {/if}
            </div>
          {/each}
        </div>
      </Panel>

      {#if canManageUserPermissions}
        <!-- Per-user permission overrides. -->
        <Hint>{m['admin.permissions.resolution_hint']()}</Hint>
        <UserPermissionsMatrix {userId} />
      {/if}
    {/if}
  </div>
</div>
