<script lang="ts">
  import { errorMessage } from '$lib/utils/errorMessage';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { formatAccountName } from '$lib/render/accountName';
  import { onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import {
    createAdminUserManagementAPI,
    type AdminMember,
    type AdminMemberDetails,
    type AdminRoleMutationResult,
    type AdminUserManagementAPI
  } from '$lib/api-client/adminUsers';
  import {
    createUserAPI,
    type UpdateUserProfileInput,
    type UserSummary
  } from '$lib/api-client/users';
  import AvatarEditor from '$lib/components/users/AvatarEditor.svelte';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { registerAdminUserRemovalListener } from '$lib/query/cacheRegistry';
  import { createMutation, createQuery, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { Hint, PaneContent, LoadingFog, PaneHeader, PageTitle } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import MemberDangerZone from './MemberDangerZone.svelte';
  import MemberIdentitySettings from './MemberIdentitySettings.svelte';
  import MemberOverviewPanel from './MemberOverviewPanel.svelte';
  import MemberRoleAssignments from './MemberRoleAssignments.svelte';

  const serverScope = useServerScope();
  const activeServerId = serverScope.serverId;
  const store = serverScope.store;
  const userId = $derived(page.params.userId!);
  const isSelf = $derived(store.viewerId === userId);
  const canViewMemberEmails = $derived(isSelf || store.permissions.canAdminViewUsers);
  const canAdminManageAccounts = $derived(store.permissions.canAdminManageAccounts);
  const backHref = $derived(
    resolve('/chat/[serverId]/manage/server/members', {
      serverId: serverIdToSegment(activeServerId)
    })
  );

  const session = createSessionGuard(serverScope);
  let removedMember = $state<{ serverId: string; userId: string } | null>(null);
  let roleError = $state<{ targetKey: string; message: string } | null>(null);

  const removeUserRemovalListener = registerAdminUserRemovalListener((serverId, removedUserId) => {
    if (serverId !== activeServerId || removedUserId !== userId) return;
    session.invalidate();
    removedMember = { serverId, userId: removedUserId };
  });

  onDestroy(removeUserRemovalListener);

  const memberQuery = createQuery(() => {
    const serverId = activeServerId;
    const connection = serverScope.connection;
    const targetUserId = userId;
    const removed = removedMember?.serverId === serverId && removedMember.userId === targetUserId;
    return {
      queryKey: adminQueryKeys.member(serverId, connection, targetUserId),
      queryFn: ({ signal }) =>
        connection.getAPI(createAdminUserManagementAPI).getMember(targetUserId, { signal }),
      enabled: !!serverId && !!targetUserId && !removed
    };
  });

  const details = $derived(memberQuery.data ?? null);
  const member = $derived(details?.member ?? null);
  const isBot = $derived(member?.isBot === true);
  // Self-deletion stays in the account settings danger zone; bots cascade with
  // their owner and cannot be deleted directly.
  const canDeleteHere = $derived(
    !isSelf && !isBot && !!member && !member.deleted && member.viewerCanDeleteAccount
  );
  const memberTargetKey = $derived(userId);
  const loading = $derived(memberQuery.isPending && memberQuery.isEnabled);
  const visibleRoleError = $derived(
    roleError?.targetKey === memberTargetKey ? roleError.message : null
  );

  type MemberMutationScope = SessionSnapshot & {
    userId: string;
    queryKey: ReturnType<typeof adminQueryKeys.member>;
    api: AdminUserManagementAPI;
  };
  type IdentityMutationVariables = MemberMutationScope & {
    input: UpdateUserProfileInput;
    roleNames: string[];
  };
  type PasswordMutationVariables = MemberMutationScope & { password: string };
  type RoleMutationVariables = MemberMutationScope & {
    roleName: string;
    currentlyHasRole: boolean;
  };

  function mutationScope(): MemberMutationScope | null {
    if (!member) return null;
    const snapshot = session.snapshot();
    return {
      ...snapshot,
      userId,
      queryKey: adminQueryKeys.member(snapshot.serverId, snapshot.connection, userId),
      api: snapshot.connection.getAPI(createAdminUserManagementAPI)
    };
  }

  function isCurrentTarget(target: MemberMutationScope | undefined): target is MemberMutationScope {
    return session.isCurrent(target) && target.userId === userId;
  }

  function updateCachedMember(
    target: MemberMutationScope,
    update: (current: AdminMember) => AdminMember
  ): void {
    queryClient.setQueryData<AdminMemberDetails>(target.queryKey, (current) => {
      if (!current?.member) return current;
      return { ...current, member: update(current.member) };
    });
  }

  function invalidateMemberLists(target: MemberMutationScope): void {
    const filters = {
      queryKey: adminQueryKeys.membersRoot(target.serverId, target.connection)
    };
    void queryClient.cancelQueries(filters).then(() => queryClient.invalidateQueries(filters));
  }

  function invalidateRole(target: MemberMutationScope, roleName: string): void {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.roleMembers(target.serverId, target.connection, roleName),
      exact: true
    });
  }

  const identityMutation = createMutation(() => ({
    mutationFn: ({ connection, userId: targetUserId, input }: IdentityMutationVariables) =>
      connection.getAPI(createUserAPI).updateUserProfile(targetUserId, input),
    onSuccess: (updated, target) => {
      if (!isCurrentTarget(target)) return;
      updateCachedMember(target, (current) => ({
        ...current,
        login: updated.login,
        displayName: updated.displayName,
        bio: updated.bio ?? null
      }));
      invalidateMemberLists(target);
      for (const roleName of target.roleNames) invalidateRole(target, roleName);
    }
  }));

  const cooldownMutation = createMutation(() => ({
    mutationFn: ({ api, userId: targetUserId }: MemberMutationScope) =>
      api.clearUsernameCooldown(targetUserId),
    onSuccess: (cleared, target) => {
      if (!cleared || !isCurrentTarget(target)) return;
      updateCachedMember(target, (current) => ({ ...current, lastLoginChange: null }));
      invalidateMemberLists(target);
    }
  }));

  const passwordMutation = createMutation(() => ({
    mutationFn: ({ api, userId: targetUserId, password }: PasswordMutationVariables) =>
      api.changeUserPassword(targetUserId, password),
    onSuccess: (updated, target) => {
      if (!isCurrentTarget(target)) return;
      updateCachedMember(target, () => updated);
      invalidateMemberLists(target);
    }
  }));

  const roleMutation = createMutation(() => ({
    mutationFn: ({
      api,
      userId: targetUserId,
      roleName,
      currentlyHasRole
    }: RoleMutationVariables) =>
      currentlyHasRole
        ? api.revokeRole(targetUserId, roleName)
        : api.assignRole(targetUserId, roleName)
  }));

  async function updateIdentity(input: UpdateUserProfileInput): Promise<UserSummary | null> {
    const target = mutationScope();
    if (!target || !member) return null;
    const updated = await identityMutation.mutateAsync({
      ...target,
      input,
      roleNames: [...member.roles]
    });
    return isCurrentTarget(target) ? updated : null;
  }

  async function clearUsernameCooldown(): Promise<boolean> {
    const target = mutationScope();
    if (!target) return false;
    const cleared = await cooldownMutation.mutateAsync(target);
    return cleared && isCurrentTarget(target);
  }

  async function changePassword(password: string): Promise<AdminMember | null> {
    const target = mutationScope();
    if (!target) return null;
    const updated = await passwordMutation.mutateAsync({ ...target, password });
    return isCurrentTarget(target) ? updated : null;
  }

  async function uploadAvatar(file: File): Promise<boolean> {
    const target = mutationScope();
    if (!target) return false;
    const updated = await target.connection.getAPI(createUserAPI).uploadAvatar(target.userId, file);
    if (!isCurrentTarget(target)) return false;
    updateCachedMember(target, (current) => ({ ...current, avatarUrl: updated.avatarUrl }));
    invalidateMemberLists(target);
    return true;
  }

  async function deleteAvatar(): Promise<boolean> {
    const target = mutationScope();
    if (!target) return false;
    const updated = await target.connection.getAPI(createUserAPI).deleteAvatar(target.userId);
    if (!isCurrentTarget(target)) return false;
    updateCachedMember(target, (current) => ({ ...current, avatarUrl: updated.avatarUrl }));
    invalidateMemberLists(target);
    return true;
  }

  async function toggleMemberRole(roleName: string, currentlyHasRole: boolean): Promise<boolean> {
    const target = mutationScope();
    if (!target || (roleMutation.isPending && isCurrentTarget(roleMutation.variables)))
      return false;
    const targetKey = memberTargetKey;
    roleError = null;

    let result: AdminRoleMutationResult;
    try {
      result = await roleMutation.mutateAsync({ ...target, roleName, currentlyHasRole });
    } catch (error) {
      if (isCurrentTarget(target)) {
        roleError = {
          targetKey,
          message: errorMessage(error, m('admin.members.role_update_failed'))
        };
      }
      return false;
    }
    if (!isCurrentTarget(target) || !result.changed) return false;

    if (result.member) {
      updateCachedMember(target, () => result.member!);
    } else {
      await queryClient.invalidateQueries({ queryKey: target.queryKey, exact: true });
      if (isCurrentTarget(target) && memberQuery.isError) {
        roleError = {
          targetKey,
          message: errorMessage(memberQuery.error, m('admin.members.load_failed'))
        };
      }
    }

    invalidateMemberLists(target);
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.userPermissions(target.serverId, target.connection, target.userId),
      exact: true
    });
    invalidateRole(target, roleName);
    return isCurrentTarget(target);
  }

  const updatingRole = $derived(
    roleMutation.isPending && isCurrentTarget(roleMutation.variables)
      ? (roleMutation.variables?.roleName ?? null)
      : null
  );
</script>

{#snippet memberName()}
  {#if member}
    <AccountName name={member.displayName} identity={member} />
  {:else}
    <LoadingFog class="h-4 w-28" label={m('admin.members.loading_member')} />
  {/if}
{/snippet}

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: formatAccountName(member?.displayName ?? m('admin.members.member_fallback'), member)
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.members.member_details')}
    subtitle={formatAccountName(member?.displayName ?? m('common.loading'), member)}
    subtitleContent={memberName}
    {backHref}
    backLabel={m('admin.members.back_to_members')}
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if loading}
        <LoadingFog class="h-40 w-full" label={m('admin.members.loading_member')} />
      {:else if !details || !member}
        <Hint tone="danger">{m('admin.members.not_found')}</Hint>
      {:else}
        {#if visibleRoleError}
          <FormError error={visibleRoleError} />
        {/if}

        <MemberOverviewPanel {member} roles={details.roles} {canViewMemberEmails} />

        {#if (isSelf || canAdminManageAccounts) && !member.deleted}
          {#key memberTargetKey}
            <AvatarEditor user={member} onupload={uploadAvatar} ondelete={deleteAvatar} />
          {/key}
        {/if}

        {#if canDeleteHere}
          <MemberDangerZone {member} />
        {/if}

        {#if !isBot}
          {#key memberTargetKey}
            {#if canAdminManageAccounts}
              <MemberIdentitySettings
                {member}
                {isSelf}
                {updateIdentity}
                {clearUsernameCooldown}
                {changePassword}
              />
            {/if}

            <MemberRoleAssignments
              {details}
              {isSelf}
              serverId={activeServerId}
              {updatingRole}
              {toggleMemberRole}
            />
          {/key}
        {/if}
      {/if}
      {#if loading || (details?.viewerCanManageUserPermissions && !isBot)}
        <Hint>{m('admin.permissions.resolution_hint')}</Hint>
        <UserPermissionsMatrix {userId} />
      {/if}
    </div>
  </PaneContent>
</div>
