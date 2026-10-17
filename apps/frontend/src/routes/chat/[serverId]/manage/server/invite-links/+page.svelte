<script lang="ts">
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import { createInviteLinkAPI, type InviteLink } from '@chatto/client/api/invitations';
  import { createUserAPI } from '@chatto/client/api/users';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import FeatureCompatibilityNotice from '$lib/components/chat/FeatureCompatibilityNotice.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { SvelteSet } from 'svelte/reactivity';
  import { onMount } from 'svelte';
  import { registerQueryCacheRemovalListener } from '$lib/query/cacheRegistry';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import {
    Panel,
    DataTable,
    ConfirmDialog,
    Hint,
    PaneContent,
    Pill,
    PaneHeader,
    PageTitle
  } from '$lib/ui';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createInfiniteQuery, createMutation, createQuery, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { getLocale } from '$lib/i18n/runtime';
  import { Button } from '$lib/ui/form';
  import { toast } from '$lib/ui/toast';
  import { formatDateTime, timeFormatSettingsFor } from '$lib/utils/formatTime';

  const PAGE_SIZE = 20;
  const serverScope = useServerScope();
  const limitedFeatures = $derived(
    serverScope.store.serverInfo.limitedFeatures.filter((feature) => feature === 'inviteLinks')
  );
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );
  const activeLocale = $derived(getLocale());
  let scrollContainer = $state<HTMLDivElement>();
  let revokeTarget = $state<InviteLink | null>(null);
  let authorityGeneration = $state(0);
  const guard = createSessionGuard(serverScope);
  const manageAll = $derived(serverScope.store.permissions.canManageInviteLinks);
  const title = $derived(manageAll ? m('invite_links.all') : m('invite_links.own'));

  onMount(() =>
    registerQueryCacheRemovalListener((serverId) => {
      if (serverId !== serverScope.serverId) return;
      // A late list or open confirmation must not retain another creator's link.
      authorityGeneration++;
      revokeTarget = null;
    })
  );

  const invitationsQuery = createInfiniteQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    return {
      queryKey: [
        ...adminQueryKeys.invitations(serverId, connection),
        manageAll,
        authorityGeneration
      ],
      queryFn: ({ pageParam, signal }) =>
        connection.getAPI(createInviteLinkAPI).list(pageParam, PAGE_SIZE, { signal }),
      enabled: limitedFeatures.length === 0,
      initialPageParam: 0,
      getNextPageParam: (lastPage, _pages, lastPageParam) =>
        lastPage.hasMore && lastPage.inviteLinks.length > 0
          ? lastPageParam + lastPage.inviteLinks.length
          : undefined
    };
  });

  const invitations = $derived(
    (invitationsQuery.data?.pages ?? []).flatMap((page) => page.inviteLinks)
  );
  const creatorIds = $derived([...new SvelteSet(invitations.map((link) => link.createdBy))]);
  const creatorsQuery = createQuery(() => {
    const ids = creatorIds;
    return {
      queryKey: [
        ...adminQueryKeys.invitations(serverScope.serverId, serverScope.connection),
        'creators',
        manageAll,
        authorityGeneration,
        ids
      ],
      enabled: limitedFeatures.length === 0 && manageAll && ids.length > 0,
      queryFn: async ({ signal }) => {
        const api = serverScope.connection.getAPI(createUserAPI);
        const batches = [];
        for (let offset = 0; offset < ids.length; offset += 100) {
          batches.push(api.batchGetUsers(ids.slice(offset, offset + 100)));
        }
        const users = (await Promise.all(batches)).flat();
        signal.throwIfAborted();
        return users;
      }
    };
  });
  const creators = $derived(new Map((creatorsQuery.data ?? []).map((user) => [user.id, user])));
  const loading = $derived(invitationsQuery.isPending);
  const loadingMore = $derived(invitationsQuery.isFetchingNextPage);
  const hasMore = $derived(invitationsQuery.hasNextPage);

  const revokeMutation = createMutation(() => ({
    mutationFn: async (invitation: InviteLink) => {
      const snapshot = guard.snapshot();
      await serverScope.connection.getAPI(createInviteLinkAPI).revoke(invitation.id);
      return snapshot;
    },
    onSuccess: async (snapshot) => {
      if (!guard.isCurrent(snapshot)) return;
      revokeTarget = null;
      await queryClient.invalidateQueries({
        queryKey: adminQueryKeys.invitations(serverScope.serverId, serverScope.connection)
      });
      toast.success(m('admin.invitations.revoked'));
    },
    onError: (error) => toastError(error)
  }));

  async function copyInvitation(invitation: InviteLink) {
    try {
      await navigator.clipboard.writeText(invitation.link);
      toast.success(m('invite_links.copied'));
    } catch {
      toastError(undefined, m('room.message.actions.copy_link_failed'));
    }
  }

  function formatTimestamp(value: string): string {
    return formatDateTime(value, userSettings, activeLocale);
  }

  async function loadMore() {
    if (!loading && !loadingMore && hasMore) await invitationsQuery.fetchNextPage();
  }

  function statusTone(status: InviteLink['status']): 'success' | 'danger' | 'muted' {
    return status === 'active' ? 'success' : status === 'revoked' ? 'danger' : 'muted';
  }
</script>

<PageTitle {title} />

<div class="pane-page">
  <PaneHeader {title} />

  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      <FeatureCompatibilityNotice
        features={limitedFeatures}
        serverSegment={serverIdToSegment(serverScope.serverId)}
      />
      {#if limitedFeatures.length === 0}
        <Panel title={m('admin.invitations.list_title')} noPadding>
          {#if invitationsQuery.error}
            <div class="p-5"><Hint tone="danger">{errorMessage(invitationsQuery.error)}</Hint></div>
          {/if}
          <DataTable
            items={invitations}
            columns={manageAll ? 6 : 5}
            loading={loading && invitations.length === 0}
            emptyMessage={m('admin.invitations.empty')}
            {hasMore}
            {loadingMore}
            onLoadMore={loadMore}
            loadMoreRoot={scrollContainer}
            loadingMoreMessage={m('admin.common.loading')}
          >
            {#snippet header()}
              {#if manageAll}<th class="table-header-cell">{m('invite_links.creator')}</th>{/if}
              <th class="table-header-cell">{m('admin.common.status')}</th>
              <th class="table-header-cell">{m('admin.invitations.uses')}</th>
              <th class="table-header-cell">{m('admin.common.expires')}</th>
              <th class="table-header-cell">{m('admin.invitations.created_at')}</th>
              <th class="table-header-cell text-end">{m('admin.invitations.actions')}</th>
            {/snippet}
            {#snippet row(invitation)}
              {#if manageAll}
                {@const creator = creators.get(invitation.createdBy)}
                <td class="px-4 py-3"
                  ><AccountName
                    name={creator?.displayName || creator?.login || invitation.createdBy}
                    identity={creator}
                  /></td
                >
              {/if}
              <td class="px-4 py-3"
                ><Pill tone={statusTone(invitation.status)}
                  >{m(`admin.invitations.status.${invitation.status}`)}</Pill
                ></td
              >
              <td class="px-4 py-3 whitespace-nowrap"
                >{invitation.useCount} / {invitation.maxUses ?? '∞'}</td
              >
              <td class="px-4 py-3 text-muted"
                >{invitation.expiresAt
                  ? formatTimestamp(invitation.expiresAt)
                  : m('admin.invitations.never')}</td
              >
              <td class="px-4 py-3 text-muted">{formatTimestamp(invitation.createdAt)}</td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-2">
                  <Button size="sm" variant="secondary" onclick={() => copyInvitation(invitation)}>
                    <span aria-hidden="true" class="iconify icon-[uil--copy]"></span>{m(
                      'invite_links.copy'
                    )}
                  </Button>
                  {#if invitation.status === 'active'}
                    <Button size="sm" variant="danger" onclick={() => (revokeTarget = invitation)}>
                      <span aria-hidden="true" class="iconify icon-[uil--ban]"></span>{m(
                        'admin.invitations.revoke'
                      )}
                    </Button>
                  {/if}
                </div>
              </td>
            {/snippet}
          </DataTable>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>

{#if revokeTarget}
  <ConfirmDialog
    title={m('admin.invitations.revoke_title')}
    actionLabel={m('admin.invitations.revoke')}
    loading={revokeMutation.isPending}
    onconfirm={() => revokeTarget && revokeMutation.mutate(revokeTarget)}
    onclose={() => (revokeTarget = null)}
  >
    {m('admin.invitations.revoke_description')}
  </ConfirmDialog>
{/if}
