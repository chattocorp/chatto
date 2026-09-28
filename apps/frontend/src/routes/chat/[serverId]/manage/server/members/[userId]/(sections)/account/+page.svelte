<!--
@component

Account section of a member: identity and password settings for account
managers, and the danger zone for viewers who may delete the account.
-->
<script lang="ts">
  import type { AdminMember } from '$lib/api-client/adminUsers';
  import {
    createUserAPI,
    type UpdateUserProfileInput,
    type UserSummary
  } from '$lib/api-client/users';
  import { m } from '$lib/i18n/messages';
  import { createMutation } from '$lib/query/client';
  import { Hint } from '$lib/ui';
  import MemberDangerZone from '../../MemberDangerZone.svelte';
  import MemberIdentitySettings from '../../MemberIdentitySettings.svelte';
  import { useMemberDetail, type MemberMutationScope } from '../../memberDetailContext';

  const detail = useMemberDetail();
  const canManageIdentity = $derived(!detail.isBot && detail.canAdminManageAccounts);

  type IdentityMutationVariables = MemberMutationScope & {
    input: UpdateUserProfileInput;
    roleNames: string[];
  };
  type PasswordMutationVariables = MemberMutationScope & { password: string };

  const identityMutation = createMutation(() => ({
    mutationFn: ({ connection, userId: targetUserId, input }: IdentityMutationVariables) =>
      connection.getAPI(createUserAPI).updateUserProfile(targetUserId, input),
    onSuccess: (updated, target) => {
      if (!detail.isCurrentTarget(target)) return;
      detail.updateCachedMember(target, (current) => ({
        ...current,
        login: updated.login,
        displayName: updated.displayName,
        bio: updated.bio ?? null
      }));
      detail.invalidateMemberLists(target);
      for (const roleName of target.roleNames) detail.invalidateRole(target, roleName);
    }
  }));

  const cooldownMutation = createMutation(() => ({
    mutationFn: ({ api, userId: targetUserId }: MemberMutationScope) =>
      api.clearUsernameCooldown(targetUserId),
    onSuccess: (cleared, target) => {
      if (!cleared || !detail.isCurrentTarget(target)) return;
      detail.updateCachedMember(target, (current) => ({ ...current, lastLoginChange: null }));
      detail.invalidateMemberLists(target);
    }
  }));

  const passwordMutation = createMutation(() => ({
    mutationFn: ({ api, userId: targetUserId, password }: PasswordMutationVariables) =>
      api.changeUserPassword(targetUserId, password),
    onSuccess: (updated, target) => {
      if (!detail.isCurrentTarget(target)) return;
      detail.updateCachedMember(target, () => updated);
      detail.invalidateMemberLists(target);
    }
  }));

  async function updateIdentity(input: UpdateUserProfileInput): Promise<UserSummary | null> {
    const target = detail.mutationScope();
    if (!target) return null;
    const updated = await identityMutation.mutateAsync({
      ...target,
      input,
      roleNames: [...detail.member.roles]
    });
    return detail.isCurrentTarget(target) ? updated : null;
  }

  async function clearUsernameCooldown(): Promise<boolean> {
    const target = detail.mutationScope();
    if (!target) return false;
    const cleared = await cooldownMutation.mutateAsync(target);
    return cleared && detail.isCurrentTarget(target);
  }

  async function changePassword(password: string): Promise<AdminMember | null> {
    const target = detail.mutationScope();
    if (!target) return null;
    const updated = await passwordMutation.mutateAsync({ ...target, password });
    return detail.isCurrentTarget(target) ? updated : null;
  }
</script>

{#if canManageIdentity || detail.canDeleteHere}
  {#if canManageIdentity}
    {#key detail.userId}
      <MemberIdentitySettings
        member={detail.member}
        isSelf={detail.isSelf}
        {updateIdentity}
        {clearUsernameCooldown}
        {changePassword}
      />
    {/key}
  {/if}

  {#if detail.canDeleteHere}
    <MemberDangerZone member={detail.member} />
  {/if}
{:else}
  <Hint tone="danger">{m('ui.access_denied.message')}</Hint>
{/if}
