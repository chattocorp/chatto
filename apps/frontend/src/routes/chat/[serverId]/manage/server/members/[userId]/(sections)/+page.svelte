<!--
@component

Profile section of a member: the member summary and, for the member and
account managers, the avatar.
-->
<script lang="ts">
  import { createUserAPI } from '@chatto/client/api/users';
  import AvatarEditor from '$lib/components/users/AvatarEditor.svelte';
  import MemberOverviewPanel from '../MemberOverviewPanel.svelte';
  import { useMemberDetail } from '../memberDetailContext';

  const detail = useMemberDetail();

  async function uploadAvatar(file: File): Promise<boolean> {
    const target = detail.mutationScope();
    if (!target) return false;
    const updated = await target.connection.getAPI(createUserAPI).uploadAvatar(target.userId, file);
    if (!detail.isCurrentTarget(target)) return false;
    detail.updateCachedMember(target, (current) => ({ ...current, avatarUrl: updated.avatarUrl }));
    detail.invalidateMemberLists(target);
    return true;
  }

  async function deleteAvatar(): Promise<boolean> {
    const target = detail.mutationScope();
    if (!target) return false;
    const updated = await target.connection.getAPI(createUserAPI).deleteAvatar(target.userId);
    if (!detail.isCurrentTarget(target)) return false;
    detail.updateCachedMember(target, (current) => ({ ...current, avatarUrl: updated.avatarUrl }));
    detail.invalidateMemberLists(target);
    return true;
  }
</script>

<MemberOverviewPanel
  member={detail.member}
  roles={detail.details.roles}
  canViewMemberEmails={detail.canViewMemberEmails}
/>

{#if (detail.isSelf || detail.canAdminManageAccounts) && !detail.member.deleted}
  {#key detail.userId}
    <AvatarEditor user={detail.member} onupload={uploadAvatar} ondelete={deleteAvatar} />
  {/key}
{/if}
