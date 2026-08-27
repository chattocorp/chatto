<script lang="ts">
  import { onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import { createAdminUserManagementAPI } from '$lib/api-client/adminUsers';
  import { Panel } from '$lib/components/admin';
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
  import { Button, FormError, TextInput } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);
  const currentUser = $derived(serverScope.store.currentUser);
  const userId = $derived(page.params.userId!);
  const isSelf = $derived(currentUser.user?.id === userId);

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
  const membersHref = $derived(
    resolve('/chat/[serverId]/manage/server/members', {
      serverId: serverIdToSegment(activeServerId)
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

  let confirmText = $state('');
  let password = $state('');
  let error = $state('');
  let deleting = $state(false);

  const canConfirm = $derived(
    !!member && !deleting && confirmText.length > 0 && confirmText === member.login
  );

  async function handleDelete(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!canConfirm || !member) return;
    // Bind the request to the current privacy generation so a purge (for
    // example from an authentication change mid-flight) discards its result.
    const generation = privacyGeneration;
    deleting = true;
    error = '';

    try {
      await serverScope.connection.getAPI(createAdminUserManagementAPI).deleteUser({
        userId,
        ...(password ? { currentPassword: password } : {})
      });
      if (generation !== privacyGeneration) return;
      toast.success(m('admin.member_delete.success'));
      // The realtime ServerMemberDeletedEvent purge
      // (removeRegisteredAdminUserQueries) fences other admin caches that embed
      // this user; here we only refresh the list and drop this page's entry.
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.membersRoot(activeServerId, serverScope.connection)
      });
      queryClient.removeQueries({
        queryKey: adminQueryKeys.member(activeServerId, serverScope.connection, userId),
        exact: true
      });
      await goto(membersHref);
    } catch (err) {
      error = err instanceof Error ? err.message : m('admin.member_delete.failed');
      // Keep the typed confirmation and password so a retry (for example after
      // supplying the missing fresh-credential password) needs no retyping.
      deleting = false;
    }
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
        <Panel
          title={m('admin.members.danger_zone')}
          icon="iconify icon-[uil--exclamation-triangle]"
        >
          <form class="flex max-w-md flex-col gap-4" onsubmit={handleDelete}>
            <Hint tone="danger">
              <strong>{m('admin.member_delete.warning', { name: member.displayName })}</strong>
            </Hint>

            <p class="text-sm text-muted">{m('admin.member_delete.consequences_intro')}</p>
            <ul class="list-inside list-disc text-sm text-muted">
              <li>{m('admin.member_delete.consequence_rooms')}</li>
              <li>{m('admin.member_delete.consequence_messages')}</li>
              <li>{m('admin.member_delete.consequence_profile')}</li>
              <li>{m('admin.member_delete.consequence_sessions')}</li>
            </ul>

            <TextInput
              id="member-delete-confirm"
              label={m('admin.member_delete.confirm_label', { login: member.login })}
              bind:value={confirmText}
              placeholder={member.login}
              disabled={deleting}
              autocomplete="off"
            />

            <TextInput
              id="member-delete-password"
              label={m('admin.member_delete.password_label')}
              description={m('admin.member_delete.password_hint')}
              type="password"
              bind:value={password}
              disabled={deleting}
              autocomplete="current-password"
            />

            {#if error}
              <FormError {error} />
            {/if}

            <div class="flex flex-wrap justify-end gap-2">
              <Button variant="secondary" href={backHref} disabled={deleting}>
                {m('common.cancel')}
              </Button>
              <Button
                type="submit"
                variant="danger"
                defaultAction
                disabled={!canConfirm}
                loading={deleting}
                loadingText={m('admin.member_delete.deleting')}
              >
                {m('admin.member_delete.submit')}
              </Button>
            </div>
          </form>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>
