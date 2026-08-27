<script lang="ts">
  import { onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import { createAdminUserManagementAPI } from '$lib/api-client/adminUsers';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import {
    registerAdminUserRemovalListener,
    registerQueryCacheRemovalListener
  } from '$lib/query/cacheRegistry';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, PaneContent, PageTitle } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import { toast } from '$lib/ui/toast';
  import MemberDeleteForm from './MemberDeleteForm.svelte';

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);
  const currentUser = $derived(serverScope.store.currentUser);
  const userId = $derived(page.params.userId!);
  const isSelf = $derived(currentUser.user?.id === userId);
  // The detail page keys its interactive sections on this value; keying the
  // confirmation form here resets input state when the route target changes.
  const memberTargetKey = $derived(
    `${activeServerId}:${serverScope.connection.queryScope}:${userId}`
  );

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
  // Authentication or visibility changes purge all admin queries for a
  // session; fence in-flight reads against that generation the same way the
  // member detail page does.
  let privacyGeneration = $state(0);
  const removeCacheRemovalListener = registerQueryCacheRemovalListener((serverId) => {
    if (serverId === activeServerId) privacyGeneration += 1;
  });
  onDestroy(() => {
    // Discard in-flight mutation results bound to this component instance.
    privacyGeneration += 1;
    removeRemovalListener();
    removeCacheRemovalListener();
  });

  const backHref = $derived(
    resolve('/chat/[serverId]/manage/server/members/[userId]', {
      serverId: serverIdToSegment(activeServerId),
      userId
    })
  );
  // Shares the member-detail page's cache entry, so data is fresh if the
  // viewer came straight from there and stays consistent while they type.
  const memberQuery = createQuery(
    () => {
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
    },
    () => queryClient
  );

  const details = $derived(memberQuery.data ?? null);
  const member = $derived(details?.member ?? null);
  const loading = $derived(memberQuery.isPending && memberQuery.isEnabled);
  // Mirrors the backend's CanDeleteUser gate surface plus local rules: admins
  // delete others here, never themselves or bots.
  const deletable = $derived(
    !!member && !member.deleted && !member.isBot && !isSelf && member.viewerCanDeleteAccount
  );

  type DeletionTarget = {
    serverId: string;
    connection: typeof serverScope.connection;
    userId: string;
    privacyGeneration: number;
  };

  function isCurrentTarget(target: DeletionTarget): boolean {
    return (
      serverScope.isCurrent() &&
      target.serverId === activeServerId &&
      target.connection.queryScope === serverScope.connection.queryScope &&
      target.userId === userId &&
      target.privacyGeneration === privacyGeneration
    );
  }

  async function handleDelete(): Promise<void> {
    // Bind the request and all completion effects to the route target. SvelteKit
    // can reuse this component when the user or server parameter changes.
    const target: DeletionTarget = {
      serverId: activeServerId,
      connection: serverScope.connection,
      userId,
      privacyGeneration
    };

    await target.connection
      .getAPI(createAdminUserManagementAPI)
      .deleteUser({ userId: target.userId });
    if (!isCurrentTarget(target)) return;

    toast.success(m('admin.member_delete.success'));
    // The realtime ServerMemberDeletedEvent purge
    // (removeRegisteredAdminUserQueries) fences other admin caches that embed
    // this user; here we only refresh the list and drop this page's entry.
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

<!-- @component Full-page confirmation for permanently deleting another member's account. Lives outside a modal so consequences and future blockers can be described before confirming. -->
<PageTitle
  title={m('admin.common.server_admin_page_title', { title: m('admin.member_delete.title') })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.member_delete.title')}
    subtitle={member?.displayName ?? m('common.loading')}
    {backHref}
    backLabel={m('admin.members.back_to_members')}
    showMobileNav
  />

  <PaneContent>
    <div class="flex max-w-xl flex-col gap-6">
      {#if loading}
        <div class="text-muted">{m('admin.members.loading_member')}</div>
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
