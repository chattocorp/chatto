<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import { onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { createAdminUserManagementAPI } from '$lib/api/adminUsers';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { registerAdminUserRemovalListener } from '$lib/query/cacheRegistry';
  import { createQuery, queryClient, removeAdminUserQueries } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { Hint, PaneContent, PageTitle, LoadingFog, PaneHeader } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import MemberDeleteForm from './MemberDeleteForm.svelte';

  const serverScope = useServerScope();
  const activeServerId = serverScope.serverId;
  const userId = $derived(page.params.userId!);
  const isSelf = $derived(serverScope.store.viewerId === userId);
  // The detail page keys its interactive sections on this value; keying the
  // confirmation form here resets input state when the route target changes.
  const memberTargetKey = $derived(userId);

  // Privacy fence: once a removal of this member is observed (for example by
  // another admin), stop rendering and refetching the deletion flow. The
  // realtime purge listener also owns cache invalidation for any other admin
  // queries embedding this user; success below only extends it.
  let removedMember = $state<{ serverId: string; userId: string } | null>(null);
  const removeRemovalListener = registerAdminUserRemovalListener((serverId, removedUserId) => {
    if (serverId !== activeServerId || removedUserId !== userId) return;
    queryClient.removeQueries({
      queryKey: adminQueryKeys.member(serverId, serverScope.connection, userId),
      exact: true
    });
    removedMember = { serverId, userId: removedUserId };
  });
  onDestroy(removeRemovalListener);
  // Authentication or visibility changes purge all admin queries for a
  // session. The guard also discards results after this component is destroyed.
  const session = createSessionGuard(serverScope);

  const backHref = $derived(
    resolve('/chat/[serverId]/manage/server/members/[userId]', {
      serverId: serverIdToSegment(activeServerId),
      userId
    })
  );
  // Shares the member-detail page's cache entry, so data is fresh if the
  // viewer came straight from there and stays consistent while they type.
  const memberQuery = createQuery(() => {
    const serverId = activeServerId;
    const connection = serverScope.connection;
    return {
      queryKey: adminQueryKeys.member(serverId, connection, userId),
      queryFn: ({ signal }) =>
        connection.getAPI(createAdminUserManagementAPI).getMember(userId, { signal }),
      enabled:
        !!serverId &&
        !!userId &&
        !(removedMember?.serverId === serverId && removedMember.userId === userId)
    };
  });

  const details = $derived(memberQuery.data ?? null);
  const member = $derived(details?.member ?? null);
  const loading = $derived(memberQuery.isPending && memberQuery.isEnabled);
  // Mirrors the backend's CanDeleteUser gate surface plus local rules: admins
  // delete others here, never themselves or bots.
  const deletable = $derived(
    !!member && !member.deleted && !member.isBot && !isSelf && member.viewerCanDeleteAccount
  );

  type DeletionTarget = SessionSnapshot & { userId: string };

  function isCurrentTarget(target: DeletionTarget): boolean {
    return session.isCurrent(target) && target.userId === userId;
  }

  async function handleDelete(): Promise<void> {
    // Bind the request and all completion effects to the route target. SvelteKit
    // can reuse this component when the user or server parameter changes.
    const target: DeletionTarget = { ...session.snapshot(), userId };

    await target.connection
      .getAPI(createAdminUserManagementAPI)
      .deleteUser({ userId: target.userId });
    if (!isCurrentTarget(target)) return;

    toast.success(m('admin.member_delete.success'));
    // Scrub private row caches immediately, even if realtime delivery is delayed.
    removeAdminUserQueries(target.serverId, target.userId);
    void queryClient.invalidateQueries({
      queryKey: adminQueryKeys.membersRoot(target.serverId, target.connection)
    });
    queryClient.removeQueries({
      queryKey: adminQueryKeys.member(target.serverId, target.connection, target.userId),
      exact: true
    });
    await goto(
      resolve('/chat/[serverId]/manage/server/members', {
        serverId: serverIdToSegment(target.serverId)
      })
    );
  }
</script>

{#snippet memberName()}
  {#if member}
    <AccountName name={member.displayName} identity={member} />
  {:else}
    <LoadingFog class="h-4 w-28" label={m('admin.members.loading_member')} />
  {/if}
{/snippet}

<!-- @component Full-page confirmation for permanently deleting another member's account. Lives outside a modal so consequences and future blockers can be described before confirming. -->
<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.member_delete.title') })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.member_delete.title')}
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
      {:else if !deletable}
        <Hint tone="danger">{m('admin.member_delete.not_allowed')}</Hint>
      {:else}
        {#key memberTargetKey}
          <MemberDeleteForm {member} cancelHref={backHref} deleteMember={handleDelete} />
        {/key}
      {/if}
    </div>
  </PaneContent>
</div>
