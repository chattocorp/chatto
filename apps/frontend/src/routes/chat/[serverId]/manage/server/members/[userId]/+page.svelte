<script lang="ts">
  import { onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createMutation, createQuery } from '@tanstack/svelte-query';
  import {
    createAdminUserManagementAPI,
    type AdminManagedUser,
    type AdminMember,
    type AdminMemberDetails,
    type AdminRoleMutationResult,
    type AdminUpdateUserInput,
    type AdminUserManagementAPI
  } from '$lib/api-client/adminUsers';
  import { createUserAPI } from '$lib/api-client/users';
  import AvatarEditor from '$lib/components/users/AvatarEditor.svelte';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import {
    registerAdminUserRemovalListener,
    registerQueryCacheRemovalListener
  } from '$lib/query/cacheRegistry';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
  import { Hint, PaneContent } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import MemberDangerZone from './MemberDangerZone.svelte';
  import MemberIdentitySettings from './MemberIdentitySettings.svelte';
  import MemberOverviewPanel from './MemberOverviewPanel.svelte';
  import MemberRoleAssignments from './MemberRoleAssignments.svelte';

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);
  const store = $derived(serverScope.store);
  const userId = $derived(page.params.userId!);
  const currentUser = $derived(store.currentUser);
  const isSelf = $derived(currentUser.user?.id === userId);
  const canViewMemberEmails = $derived(isSelf || store.permissions.canAdminViewUsers);
  const canAdminManageAccounts = $derived(store.permissions.canAdminManageAccounts);
  const supportsUserAvatars = $derived(store.serverInfo.supportsFeature('userAvatars'));
  const backHref = $derived(
    resolve('/chat/[serverId]/manage/server/members', {
      serverId: serverIdToSegment(activeServerId)
    })
  );

  let componentActive = true;
  let privacyGeneration = 0;
  let removedMember = $state<{ serverId: string; userId: string } | null>(null);
  let roleError = $state<{ targetKey: string; message: string } | null>(null);

  const removeUserRemovalListener = registerAdminUserRemovalListener((serverId, removedUserId) => {
    if (serverId !== activeServerId || removedUserId !== userId) return;
    privacyGeneration += 1;
    removedMember = { serverId, userId: removedUserId };
  });
  const removeCacheRemovalListener = registerQueryCacheRemovalListener((serverId) => {
    if (serverId === activeServerId) privacyGeneration += 1;
  });

  onDestroy(() => {
    componentActive = false;
    privacyGeneration += 1;
    removeUserRemovalListener();
    removeCacheRemovalListener();
  });

  const memberQuery = createQuery(
    () => {
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
    },
    () => queryClient
  );

  const details = $derived(memberQuery.data ?? null);
  const member = $derived(details?.member ?? null);
  const isBot = $derived(member?.isBot === true);
  // Self-deletion stays in the account settings danger zone; bots cascade with
  // their owner and cannot be deleted directly.
  const canDeleteHere = $derived(
    !isSelf && !isBot && !!member && !member.deleted && member.viewerCanDeleteAccount
  );
  const memberTargetKey = $derived(
    `${activeServerId}:${serverScope.connection.queryScope}:${userId}`
  );
  const loading = $derived(memberQuery.isPending && memberQuery.isEnabled);
  const visibleRoleError = $derived(
    roleError?.targetKey === memberTargetKey ? roleError.message : null
  );

  type MemberMutationScope = {
    serverId: string;
    connection: ServerConnection;
    userId: string;
    queryKey: ReturnType<typeof adminQueryKeys.member>;
    api: AdminUserManagementAPI;
    privacyGeneration: number;
  };
  type IdentityMutationVariables = MemberMutationScope & {
    input: Omit<AdminUpdateUserInput, 'userId'>;
    roleNames: string[];
  };
  type PasswordMutationVariables = MemberMutationScope & { password: string };
  type RoleMutationVariables = MemberMutationScope & {
    roleName: string;
    currentlyHasRole: boolean;
  };

  function mutationScope(): MemberMutationScope | null {
    if (!member) return null;
    const connection = serverScope.connection;
    return {
      serverId: activeServerId,
      connection,
      userId,
      queryKey: adminQueryKeys.member(activeServerId, connection, userId),
      api: connection.getAPI(createAdminUserManagementAPI),
      privacyGeneration
    };
  }

  function isCurrentTarget(target: MemberMutationScope | undefined): target is MemberMutationScope {
    return (
      target !== undefined &&
      componentActive &&
      serverScope.isCurrent() &&
      target.serverId === activeServerId &&
      target.connection.queryScope === serverScope.connection.queryScope &&
      target.userId === userId &&
      target.privacyGeneration === privacyGeneration
    );
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
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.membersRoot(target.serverId, target.connection)
    });
  }

  function invalidateRole(target: MemberMutationScope, roleName: string): void {
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.role(target.serverId, target.connection, roleName),
      exact: true
    });
  }

  const identityMutation = createMutation(
    () => ({
      mutationFn: ({ api, userId: targetUserId, input }: IdentityMutationVariables) =>
        api.updateUser({ userId: targetUserId, ...input }),
      onSuccess: (updated, target) => {
        if (!isCurrentTarget(target)) return;
        updateCachedMember(target, (current) => ({
          ...current,
          login: updated.login,
          displayName: updated.displayName
        }));
        invalidateMemberLists(target);
        for (const roleName of target.roleNames) invalidateRole(target, roleName);
      }
    }),
    () => queryClient
  );

  const cooldownMutation = createMutation(
    () => ({
      mutationFn: ({ api, userId: targetUserId }: MemberMutationScope) =>
        api.clearUsernameCooldown(targetUserId),
      onSuccess: (cleared, target) => {
        if (!cleared || !isCurrentTarget(target)) return;
        updateCachedMember(target, (current) => ({ ...current, lastLoginChange: null }));
        invalidateMemberLists(target);
      }
    }),
    () => queryClient
  );

  const passwordMutation = createMutation(
    () => ({
      mutationFn: ({ api, userId: targetUserId, password }: PasswordMutationVariables) =>
        api.updateUserPassword(targetUserId, password),
      onSuccess: (updated, target) => {
        if (!isCurrentTarget(target)) return;
        updateCachedMember(target, () => updated);
        invalidateMemberLists(target);
      }
    }),
    () => queryClient
  );

  const roleMutation = createMutation(
    () => ({
      mutationFn: ({
        api,
        userId: targetUserId,
        roleName,
        currentlyHasRole
      }: RoleMutationVariables) =>
        currentlyHasRole
          ? api.revokeRole(targetUserId, roleName)
          : api.assignRole(targetUserId, roleName)
    }),
    () => queryClient
  );

  async function updateIdentity(input: {
    login?: string;
    displayName?: string;
  }): Promise<AdminManagedUser | null> {
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

  async function updatePassword(password: string): Promise<AdminMember | null> {
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
          message: error instanceof Error ? error.message : m('admin.members.role_update_failed')
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
          message:
            memberQuery.error instanceof Error
              ? memberQuery.error.message
              : m('admin.members.load_failed')
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

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: member?.displayName ?? m('admin.members.member_fallback')
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.members.member_details')}
    subtitle={member?.displayName ?? m('common.loading')}
    {backHref}
    backLabel={m('admin.members.back_to_members')}
    showMobileNav
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if loading}
        <div class="text-muted">{m('admin.members.loading_member')}</div>
      {:else if !details || !member}
        <Hint tone="danger">{m('admin.members.not_found')}</Hint>
      {:else}
        {#if visibleRoleError}
          <FormError error={visibleRoleError} />
        {/if}

        <MemberOverviewPanel {member} roles={details.roles} {canViewMemberEmails} />

        {#if supportsUserAvatars && (isSelf || canAdminManageAccounts) && !member.deleted}
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
                {updatePassword}
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

          {#if details.viewerCanManageUserPermissions}
            <Hint>{m('admin.permissions.resolution_hint')}</Hint>
            <UserPermissionsMatrix {userId} />
          {/if}
        {/if}
      {/if}
    </div>
  </PaneContent>
</div>
