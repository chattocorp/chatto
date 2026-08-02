<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import * as m from '$lib/i18n/messages';

  import { createThreadAPI, type FollowedThread } from '$lib/api-client/threads';
  import { queryClient } from '$lib/query/client';
  import {
    flattenFollowedThreads,
    reconcileFollowedThreadViewerStates,
    threadQueryKeys,
    updateFollowedThreadSummary,
    type FollowedThreadsData
  } from '$lib/query/threads';
  import { EmptyState, Hint, PaneHeader, SegmentedControl } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import RoomEvent from '../[roomId]/RoomEvent.svelte';
  import { formatDate, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import {
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS,
    createRoomMembers,
    createComposerContext,
    createMentionRoles
  } from '$lib/state/room';

  const serverScope = useServerScope();
  const serverStore = $derived(serverScope.store);

  // Provide room contexts so MessageEvent can render in read-only mode.
  // All permissions are false (no editing, deleting, reacting from this view),
  // and the members list is empty; role highlighting uses server reference data.
  createRoomPermissions(() => DEFAULT_ROOM_PERMISSIONS);
  createRoomMembers();
  createComposerContext();
  createMentionRoles(() => serverStore.mentionRoles.roles);

  const userSettings = $derived(timeFormatSettingsFor(serverStore.currentUser.user?.settings));
  const activeLocale = $derived(getLocale());
  const PAGE_SIZE = 20;

  $effect(() => {
    void serverStore.mentionRoles.refresh();
  });

  let reconciledQueryScope: string | null = null;
  let reconciledMountedSnapshot = false;

  const threadsQuery = createInfiniteQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      return {
        queryKey: threadQueryKeys.followed(serverId, connection),
        queryFn: async ({ pageParam, signal }) => {
          const result = await connection
            .getAPI(createThreadAPI)
            .listFollowedThreads({ limit: PAGE_SIZE, offset: pageParam }, { signal });
          const pageData = {
            ...result,
            nextOffset: pageParam + result.threads.length
          };
          if (!serverScope.isCurrent() || connection !== serverScope.connection) return pageData;
          return reconcilePageWithCurrentProjection(pageData, pageParam);
        },
        initialPageParam: 0,
        getNextPageParam: (lastPage, _pages, lastPageParam) =>
          lastPage.hasMore && lastPage.nextOffset > lastPageParam ? lastPage.nextOffset : undefined
      };
    },
    () => queryClient
  );

  const threads = $derived(flattenFollowedThreads(threadsQuery.data));
  const loading = $derived(threadsQuery.isPending);
  const loadingMore = $derived(threadsQuery.isFetchingNextPage);
  const error = $derived(
    threadsQuery.isError
      ? threadsQuery.error instanceof Error
        ? threadsQuery.error.message
        : 'Failed to load threads'
      : null
  );
  const hasMore = $derived(threadsQuery.hasNextPage);
  const totalCount = $derived(threadsQuery.data?.pages[0]?.totalCount ?? 0);

  const filter = $derived(page.state.threadFilter ?? 'all');
  const filterOptions = $derived([
    { value: 'all' as const, label: m['chat.threads.filter_all']() },
    { value: 'unread' as const, label: m['chat.threads.filter_unread']() }
  ]);

  function setFilter(value: 'all' | 'unread') {
    replaceState('', { ...page.state, threadFilter: value });
  }

  const filteredThreads = $derived(
    filter === 'unread' ? threads.filter((t) => t.hasUnread) : threads
  );

  function reconcilePageWithCurrentProjection(
    pageData: FollowedThreadsData['pages'][number],
    pageParam: number
  ): FollowedThreadsData['pages'][number] {
    if (!serverStore.realtimeSync.hasUsableProjection) return pageData;
    let data: FollowedThreadsData | undefined = { pages: [pageData], pageParams: [pageParam] };
    data = reconcileFollowedThreadViewerStates(
      data,
      serverStore.projection.threadViewerStates
    ).data;
    for (const thread of data?.pages[0]?.threads ?? []) {
      data = applyProjectedTimelineSummary(data, thread);
    }
    return data?.pages[0] ?? pageData;
  }

  function applyProjectedTimelineSummary(
    data: FollowedThreadsData | undefined,
    thread: FollowedThread
  ): FollowedThreadsData | undefined {
    const event = serverStore.projection.timelines
      .get(thread.roomId)
      ?.events.find((candidate) => candidate.id === thread.threadRootEventId);
    const message = event?.event.case === 'messagePosted' ? event.event.value.message : null;
    const summary = message?.thread;
    if (!summary) return data;
    return updateFollowedThreadSummary(data, {
      roomId: thread.roomId,
      threadRootEventId: thread.threadRootEventId,
      replyCount: summary.replyCount,
      lastReplyAt: summary.lastReplyAt?.toDate().toISOString() ?? null,
      hasUnread: summary.viewerState?.hasUnread
    });
  }

  function reconcileCachedProjection(
    states: ReadonlyMap<string, { hasUnread?: boolean }>,
    refetchUnknown: boolean
  ) {
    const queryKey = threadQueryKeys.followed(serverScope.serverId, serverScope.connection);
    const current = queryClient.getQueryData<FollowedThreadsData>(queryKey);
    if (!current) return;

    const reconciled = reconcileFollowedThreadViewerStates(current, states);
    let next = reconciled.data;
    for (const thread of flattenFollowedThreads(next)) {
      next = applyProjectedTimelineSummary(next, thread);
    }
    if (next !== current) queryClient.setQueryData(queryKey, next);
    if (refetchUnknown && reconciled.hasUnknownThreads) {
      void queryClient.invalidateQueries({ queryKey, exact: true });
    }
  }

  // Reconcile after every query commit so an append cannot restore an older
  // page snapshot over a projection update that arrived while it was in flight.
  $effect(() => {
    const queryScope = serverScope.connection.queryScope;
    const queryData = threadsQuery.data;
    if (reconciledQueryScope !== queryScope) {
      reconciledQueryScope = queryScope;
      reconciledMountedSnapshot = false;
    }

    if (!serverStore.realtimeSync.hasUsableProjection || !queryData) return;
    const refetchUnknown = !reconciledMountedSnapshot;
    reconciledMountedSnapshot = true;
    reconcileCachedProjection(serverStore.projection.threadViewerStates, refetchUnknown);
  });

  async function loadMore() {
    if (loading || loadingMore || !hasMore) return;
    await threadsQuery.fetchNextPage();
  }

  const loadMoreWhenVisible = useLoadMoreWhenVisible({
    getCursor: () =>
      hasMore ? `${threadsQuery.data?.pageParams.at(-1) ?? 0}:${threads.length}` : null,
    loadMore,
    hasError: () => error !== null
  });

  function navigateToThread(thread: FollowedThread) {
    goto(
      resolve('/chat/[serverId]/[roomId]/[threadId]', {
        serverId: serverIdToSegment(serverScope.serverId),
        roomId: thread.roomId,
        threadId: thread.threadRootEventId
      })
    );
  }

  function formatRelativeTime(timestamp: string | null): string {
    if (!timestamp) return '';
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / (1000 * 60));
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffMins < 1) return m['chat.notifications.time_now']();
    if (diffMins < 60) return m['chat.notifications.time_minutes']({ count: diffMins });
    if (diffHours < 24) return m['chat.notifications.time_hours']({ count: diffHours });
    if (diffDays < 7) return m['chat.notifications.time_days']({ count: diffDays });

    return formatDate(date, userSettings, activeLocale);
  }
</script>

<PageTitle title={m['chat.threads.title']()} />

<div class="flex h-full w-full flex-col">
  <PaneHeader
    title={m['chat.threads.title']()}
    subtitle={m['chat.threads.subtitle']()}
    showMobileNav
  >
    {#snippet actions()}
      <SegmentedControl
        label={m['chat.threads.filter_label']()}
        options={filterOptions}
        value={filter}
        onchange={setFilter}
      />
    {/snippet}
  </PaneHeader>

  <div class="flex flex-1 flex-col overflow-y-auto">
    {#if loading && threads.length === 0}
      <div class="p-6 text-muted">{m['common.loading']()}</div>
    {:else if error}
      <div class="m-6">
        <Hint tone="danger">{error}</Hint>
      </div>
    {:else if threads.length === 0}
      <EmptyState icon="uil--comment-lines" title={m['chat.threads.empty_title']()}>
        {m['chat.threads.empty_body']()}
      </EmptyState>
    {:else if filteredThreads.length === 0}
      <EmptyState
        icon="uil--comment-check"
        title={hasMore ? m['chat.threads.no_unread_loaded']() : m['chat.threads.all_caught_up']()}
      >
        {#if hasMore}
          <div class="flex flex-col items-center gap-3">
            <span>
              {m['chat.threads.loaded_count']({ loaded: threads.length, total: totalCount })}
            </span>
            <div class="min-h-8 text-muted" {@attach loadMoreWhenVisible}>
              {#if loadingMore}{m['common.loading']()}{/if}
            </div>
          </div>
        {:else}
          {m['chat.threads.no_unread']()}
        {/if}
      </EmptyState>
    {:else}
      <div class="flex flex-col divide-y divide-border">
        {#each filteredThreads as thread (thread.threadRootEventId)}
          <div class="group relative" data-testid="my-thread-item">
            <!-- Channel label above the message -->
            <div class="flex gap-4 px-2 pt-4 pb-2 md:mx-2">
              <div class="w-11 shrink-0"></div>
              <div class="text-muted">
                <span
                  >{#if thread.lastReplyAt}{formatRelativeTime(thread.lastReplyAt)}, {m[
                      'chat.threads.in_room'
                    ]()}{:else}{m['chat.threads.in_room_capitalized']()}{/if}
                  #{thread.roomName}:</span
                >
              </div>
            </div>

            <!-- Clickable wrapper for navigation -->
            <div
              class="cursor-pointer pb-4"
              onclick={() => navigateToThread(thread)}
              onkeydown={(e) => e.key === 'Enter' && navigateToThread(thread)}
              role="button"
              tabindex="0"
            >
              {#if thread.rootMessage}
                <RoomEvent
                  event={thread.rootMessage}
                  roomId={thread.roomId}
                  onOpenThread={() => navigateToThread(thread)}
                />
              {:else}
                <div class="px-2 md:mx-2">
                  <p class="text-sm text-muted">{m['chat.threads.message_missing']()}</p>
                </div>
              {/if}
            </div>
          </div>
        {/each}
        {#if hasMore}
          <div class="flex min-h-14 justify-center p-4 text-muted" {@attach loadMoreWhenVisible}>
            {#if loadingMore}{m['common.loading']()}{/if}
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
