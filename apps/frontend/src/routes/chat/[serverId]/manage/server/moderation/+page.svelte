<script lang="ts">
  import { onDestroy } from 'svelte';
  import {
    createRoomCommandAPI,
    type RoomBanSummary,
    type RoomCommandAPI
  } from '$lib/api-client/rooms';
  import Panel from '$lib/ui/Panel.svelte';
  import DataTable from '$lib/ui/DataTable.svelte';
  import { Hint, PaneContent } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import UnbanRoomMemberModal from '$lib/components/moderation/UnbanRoomMemberModal.svelte';
  import { formatDate as formatDateUtil, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { toast } from '$lib/ui/toast';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { createInfiniteQuery, createMutation } from '@tanstack/svelte-query';
  import { adminQueryKeys } from '$lib/query/admin';
  import { queryClient } from '$lib/query/client';
  import { m } from '$lib/i18n/messages';

  const activeLocale = $derived(getLocale());
  const serverScope = useServerScope();
  const PAGE_SIZE = 20;
  const userSettings = $derived(
    timeFormatSettingsFor(serverScope.store.currentUser.user?.settings)
  );

  let scrollContainer = $state<HTMLDivElement>();
  let unbanDialogBan = $state<RoomBanSummary | null>(null);
  let unbanError = $state<string | null>(null);
  let unbanRequest = 0;

  onDestroy(() => {
    unbanRequest += 1;
  });

  const bansQuery = createInfiniteQuery(
    () => {
      const serverId = serverScope.serverId;
      const activeConnection = serverScope.connection;
      return {
        queryKey: adminQueryKeys.bans(serverId, activeConnection),
        queryFn: ({ pageParam, signal }) =>
          activeConnection
            .getAPI(createRoomCommandAPI)
            .listBans({ limit: PAGE_SIZE, offset: pageParam }, { signal }),
        initialPageParam: 0,
        getNextPageParam: (lastPage, _pages, lastPageParam) =>
          lastPage.hasMore && lastPage.bans.length > 0
            ? lastPageParam + lastPage.bans.length
            : undefined
      };
    },
    () => queryClient
  );

  const bans = $derived.by(() => {
    const seen = new SvelteSet<string>();
    return (bansQuery.data?.pages ?? []).flatMap((page) =>
      page.bans.filter((ban) => {
        if (seen.has(ban.id)) return false;
        seen.add(ban.id);
        return true;
      })
    );
  });
  const hasMore = $derived(bansQuery.hasNextPage);
  const loading = $derived(bansQuery.isPending);
  const loadingMore = $derived(bansQuery.isFetchingNextPage);
  const error = $derived(bansQuery.isError ? m('admin.moderation.admin_unavailable') : null);

  type UnbanVariables = {
    api: RoomCommandAPI;
    queryKey: ReturnType<typeof adminQueryKeys.bans>;
    ban: RoomBanSummary;
    reason: string;
  };

  const unbanMutation = createMutation(
    () => ({
      mutationFn: ({ api, ban, reason }: UnbanVariables) =>
        api.unbanMember({ roomId: ban.roomId, userId: ban.userId, reason }),
      onSuccess: (_unbanned, variables) =>
        queryClient.invalidateQueries({ queryKey: variables.queryKey })
    }),
    () => queryClient
  );

  const unbanningBanId = $derived(
    unbanMutation.isPending ? (unbanMutation.variables?.ban.id ?? null) : null
  );

  async function loadMore() {
    if (loading || loadingMore || !hasMore) return;
    await bansQuery.fetchNextPage();
  }

  function formatDate(value: string | null | undefined): string {
    if (!value) return m('admin.moderation.no_expiry');
    return formatDateUtil(value, userSettings, activeLocale);
  }

  function roomLabel(ban: RoomBanSummary): string {
    return ban.room ? `#${ban.room.name}` : ban.roomId;
  }

  function openUnbanDialog(ban: RoomBanSummary) {
    unbanRequest += 1;
    unbanDialogBan = ban;
    unbanError = null;
  }

  function isCurrentUnban(request: number): boolean {
    return request === unbanRequest && serverScope.isCurrent();
  }

  async function unban(ban: RoomBanSummary, reason: string) {
    if (unbanMutation.isPending) return;
    const request = ++unbanRequest;
    const serverId = serverScope.serverId;
    const activeConnection = serverScope.connection;
    unbanError = null;
    try {
      await unbanMutation.mutateAsync({
        api: activeConnection.getAPI(createRoomCommandAPI),
        queryKey: adminQueryKeys.bans(serverId, activeConnection),
        ban,
        reason
      });
    } catch {
      if (!isCurrentUnban(request)) return;
      unbanError = m('admin.moderation.unban_failed');
      toast.error(unbanError);
      return;
    }
    if (!isCurrentUnban(request)) return;

    toast.success(m('admin.moderation.unban_success'));
    unbanDialogBan = null;
  }

  function closeUnbanDialog() {
    unbanRequest += 1;
    unbanDialogBan = null;
  }
</script>

<PageTitle title={m('admin.common.page_title', { title: m('admin.moderation.title') })} />

<div class="pane-page">
  <PaneHeader
    title={m('admin.moderation.title')}
    subtitle={m('admin.moderation.subtitle')}
    showMobileNav
  />

  <PaneContent bind:scrollContainer>
    <div class="flex flex-col gap-6">
      {#if loading && bans.length === 0}
        <div class="text-muted">{m('admin.moderation.loading_bans')}</div>
      {:else}
        {#if error}
          <Hint tone="danger">{error}</Hint>
        {/if}

        <Panel noPadding>
          <DataTable
            items={bans}
            columns={5}
            emptyMessage={m('admin.moderation.empty_bans')}
            hasMore={hasMore && !error}
            {loadingMore}
            onLoadMore={loadMore}
            loadMoreRoot={scrollContainer}
            loadingMoreMessage={m('ui.data_table.loading_more')}
          >
            {#snippet header()}
              <th class="table-header-cell">{m('admin.common.user')}</th>
              <th class="table-header-cell">{m('admin.common.room')}</th>
              <th class="table-header-cell">{m('admin.common.reason')}</th>
              <th class="table-header-cell">{m('admin.common.expires')}</th>
              <th class="table-header-cell"></th>
            {/snippet}
            {#snippet row(ban)}
              {@const user = ban.user}
              <td class="min-w-48 px-3 py-2">
                <div class="flex items-center gap-2">
                  {#if user}
                    <UserAvatar {user} size="sm" />
                  {:else}
                    <div
                      class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-surface-emphasized text-muted"
                    >
                      <span class="iconify icon-[uil--user] text-base"></span>
                    </div>
                  {/if}
                  <div class="min-w-0">
                    <div class="truncate font-medium">{user?.displayName || ban.userId}</div>
                    <div class="truncate text-xs text-muted">
                      {#if user}@{user.login}{/if}
                    </div>
                  </div>
                </div>
              </td>
              <td class="max-w-56 px-3 py-2">
                <div class="truncate">{roomLabel(ban)}</div>
              </td>
              <td class="min-w-64 px-3 py-2">
                <div class="line-clamp-2 break-words whitespace-pre-wrap">{ban.reason}</div>
              </td>
              <td class="px-3 py-2 text-muted">
                <div class="whitespace-nowrap">{formatDate(ban.expiresAt)}</div>
              </td>
              <td class="px-3 py-2 text-right">
                <Button
                  variant="secondary"
                  size="sm"
                  loading={unbanningBanId === ban.id}
                  loadingText={m('admin.moderation.unbanning')}
                  onclick={() => openUnbanDialog(ban)}
                >
                  <span class="iconify icon-[uil--unlock]"></span>
                  <span>{m('admin.moderation.unban')}</span>
                </Button>
              </td>
            {/snippet}
          </DataTable>
        </Panel>
      {/if}
    </div>
  </PaneContent>
</div>

{#if unbanDialogBan}
  <UnbanRoomMemberModal
    user={unbanDialogBan.user}
    userId={unbanDialogBan.userId}
    room={unbanDialogBan.room}
    roomId={unbanDialogBan.roomId}
    submitting={unbanningBanId === unbanDialogBan.id}
    error={unbanError}
    onconfirm={(reason) => unban(unbanDialogBan!, reason)}
    onclose={closeUnbanDialog}
  />
{/if}
