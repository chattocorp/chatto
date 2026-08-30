<script lang="ts">
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';

  import { createThreadAPI, type FollowedThread } from '$lib/api-client/threads';
  import { createReadStateAPI } from '$lib/api-client/readState';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { queryClient } from '$lib/query/client';
  import {
    flattenFollowedThreads,
    reconcileFollowedThreadViewerStates,
    threadQueryKeys,
    updateFollowedThreadSummary,
    type FollowedThreadsData
  } from '$lib/query/threads';
  import { EmptyState, Hint, PaneHeader, SegmentedControl } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { formatDate, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';

  const serverScope = useServerScope();
  const serverStore = $derived(serverScope.store);

  const userSettings = $derived(timeFormatSettingsFor(serverStore.currentUser.user?.settings));
  const activeLocale = $derived(getLocale());
  const PAGE_SIZE = 20;

  let reconciledQueryScope: string | null = null;
  let reconciledMountedSnapshot = false;
  let actionThreadId = $state<string | null>(null);

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
    { value: 'all' as const, label: m('chat.threads.filter_all') },
    { value: 'unread' as const, label: m('chat.threads.filter_unread') }
  ]);

  function setFilter(value: 'all' | 'unread') {
    replaceState('', { ...page.state, threadFilter: value });
  }

  const filteredThreads = $derived(
    filter === 'unread' ? threads.filter((t) => t.hasUnreadReplies) : threads
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
      hasUnreadReplies: summary.viewerState?.hasUnreadReplies,
      attentionLevel: summary.viewerState?.attentionLevel
    });
  }

  function reconcileCachedProjection(
    states: ReadonlyMap<string, { hasUnreadReplies?: boolean; attentionLevel?: number }>,
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

  async function markThreadRead(thread: FollowedThread) {
    const upToEventId = thread.latestReply?.id;
    if (!upToEventId || actionThreadId) return;
    actionThreadId = thread.threadRootEventId;
    try {
      await serverScope.connection.getAPI(createReadStateAPI).markThreadAsRead({
        roomId: thread.roomId,
        threadRootEventId: thread.threadRootEventId,
        upToEventId
      });
      const queryKey = threadQueryKeys.followed(serverScope.serverId, serverScope.connection);
      queryClient.setQueryData<FollowedThreadsData>(queryKey, (current) =>
        updateFollowedThreadSummary(current, {
          roomId: thread.roomId,
          threadRootEventId: thread.threadRootEventId,
          replyCount: thread.replyCount,
          lastReplyAt: thread.lastReplyAt,
          hasUnreadReplies: false,
          attentionLevel: 0
        })
      );
    } catch {
      toast.error(m('common.error.generic'));
    } finally {
      actionThreadId = null;
    }
  }

  async function unfollowThread(thread: FollowedThread) {
    if (actionThreadId) return;
    actionThreadId = thread.threadRootEventId;
    try {
      await serverScope.connection.getAPI(createThreadAPI).unfollowThread({
        roomId: thread.roomId,
        threadRootEventId: thread.threadRootEventId
      });
      await queryClient.invalidateQueries({
        queryKey: threadQueryKeys.followed(serverScope.serverId, serverScope.connection),
        exact: true
      });
    } catch {
      toast.error(m('common.error.generic'));
    } finally {
      actionThreadId = null;
    }
  }

  function messageExcerpt(event: FollowedThread['rootMessage']): string {
    if (!event || event.event.kind !== 'messagePosted') return m('chat.threads.message_missing');
    return event.event.body?.trim() || m('room.message.deleted');
  }

  function actorName(event: FollowedThread['rootMessage']): string {
    return event?.actor?.displayName || event?.actor?.login || '';
  }

  function replyCountLabel(count: number): string {
    return count === 1
      ? m('room.message.meta.reply_count_one')
      : m('room.message.meta.reply_count_many', { count });
  }

  function formatRelativeTime(timestamp: string | null): string {
    if (!timestamp) return '';
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / (1000 * 60));
    const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

    if (diffMins < 1) return m('chat.notifications.time_now');
    if (diffMins < 60) return m('chat.notifications.time_minutes', { count: diffMins });
    if (diffHours < 24) return m('chat.notifications.time_hours', { count: diffHours });
    if (diffDays < 7) return m('chat.notifications.time_days', { count: diffDays });

    return formatDate(date, userSettings, activeLocale);
  }
</script>

<PageTitle title={m('chat.threads.title')} />

<div class="flex h-full w-full flex-col">
  <PaneHeader title={m('chat.threads.title')} subtitle={m('chat.threads.subtitle')} showMobileNav>
    {#snippet actions()}
      <SegmentedControl
        label={m('chat.threads.filter_label')}
        options={filterOptions}
        value={filter}
        onchange={setFilter}
      />
    {/snippet}
  </PaneHeader>

  <div class="flex flex-1 flex-col overflow-y-auto">
    {#if loading && threads.length === 0}
      <div class="p-6 text-muted">{m('common.loading')}</div>
    {:else if error}
      <div class="m-6">
        <Hint tone="danger">{error}</Hint>
      </div>
    {:else if threads.length === 0}
      <EmptyState icon="icon-[uil--comment-lines]" title={m('chat.threads.empty_title')}>
        {m('chat.threads.empty_body')}
      </EmptyState>
    {:else if filteredThreads.length === 0}
      <EmptyState
        icon="icon-[uil--comment-check]"
        title={hasMore ? m('chat.threads.no_unread_loaded') : m('chat.threads.all_caught_up')}
      >
        {#if hasMore}
          <div class="flex flex-col items-center gap-3">
            <span>
              {m('chat.threads.loaded_summary', { loaded: threads.length, total: totalCount })}
            </span>
            <div class="min-h-8 text-muted" {@attach loadMoreWhenVisible}>
              {#if loadingMore}{m('common.loading')}{/if}
            </div>
          </div>
        {:else}
          {m('chat.threads.no_unread')}
        {/if}
      </EmptyState>
    {:else}
      <div class="mx-auto flex w-full max-w-4xl flex-col gap-2 p-2 sm:p-4">
        {#each filteredThreads as thread (thread.threadRootEventId)}
          <article
            class={[
              'group relative overflow-hidden rounded-xl border bg-surface transition-colors',
              'focus-within:border-action/50 hover:border-surface-strong hover:bg-surface-emphasized/40',
              thread.hasUnreadReplies && 'border-s-2 border-s-text',
              thread.attention === 'important' && 'border-s-2 border-s-warning',
              thread.attention === 'ambient' && !thread.hasUnreadReplies && 'border-s-2 border-s-muted'
            ]}
            data-testid="my-thread-item"
          >
            <button
              class="flex w-full cursor-pointer flex-col gap-3 p-4 text-start focus:outline-none sm:p-5"
              onclick={() => navigateToThread(thread)}
            >
              <span class="flex w-full min-w-0 items-center gap-2 text-xs text-muted">
                <span class="font-semibold text-text">#{thread.roomName}</span>
                <span aria-hidden="true">·</span>
                <span>{formatRelativeTime(thread.lastReplyAt)}</span>
                <span class="ms-auto shrink-0">{replyCountLabel(thread.replyCount)}</span>
              </span>

              <span class="flex w-full min-w-0 items-start gap-3">
                {#if thread.rootMessage?.actor}
                  <UserAvatar user={thread.rootMessage.actor} size="sm" />
                {/if}
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-xs font-semibold text-muted">
                    {actorName(thread.rootMessage)}
                  </span>
                  <span class="line-clamp-2 block text-sm text-muted">
                    {messageExcerpt(thread.rootMessage)}
                  </span>
                </span>
              </span>

              {#if thread.latestReply}
                <span class="ms-4 flex w-[calc(100%-1rem)] min-w-0 items-start gap-3 rounded-lg bg-surface-emphasized/60 p-3 sm:ms-10 sm:w-[calc(100%-2.5rem)]">
                  {#if thread.latestReply.actor}
                    <UserAvatar user={thread.latestReply.actor} size="xs" />
                  {/if}
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-xs font-semibold">
                      {actorName(thread.latestReply)}
                    </span>
                    <span
                      class={[
                        'line-clamp-2 block text-sm',
                        thread.hasUnreadReplies ? 'text-text' : 'text-muted'
                      ]}
                    >
                      {messageExcerpt(thread.latestReply)}
                    </span>
                  </span>
                  {#if thread.hasUnreadReplies}
                    <span class="mt-1 h-2 w-2 shrink-0 rounded-full bg-foreground" aria-hidden="true"></span>
                  {/if}
                </span>
              {/if}

              <span class="flex w-full items-center gap-2">
                <span class="flex -space-x-1.5" aria-hidden="true">
                  {#each thread.participants.slice(0, 4) as participant (participant.id)}
                    <UserAvatar
                      user={participant}
                      size="xs"
                      class="ring-2 ring-surface"
                      useLiveProfile={false}
                    />
                  {/each}
                </span>
                {#if thread.participantCount > thread.participants.length}
                  <span class="text-xs text-muted">
                    +{thread.participantCount - thread.participants.length}
                  </span>
                {/if}
              </span>
            </button>

            <div
              class="absolute end-3 bottom-3 flex gap-1 rounded-lg bg-surface/95 p-1 opacity-100 shadow-sm sm:opacity-0 sm:transition-opacity sm:group-hover:opacity-100 sm:group-focus-within:opacity-100"
            >
              {#if thread.hasUnreadReplies && thread.latestReply}
                <button
                  class="btn-ghost min-h-8 min-w-8 px-2 py-1"
                  disabled={actionThreadId === thread.threadRootEventId}
                  onclick={(event) => {
                    event.stopPropagation();
                    void markThreadRead(thread);
                  }}
                  title={m('room_list.mark_as_read')}
                  aria-label={m('room_list.mark_as_read')}
                >
                  <span class="iconify icon-[uil--check]" aria-hidden="true"></span>
                </button>
              {/if}
              <button
                class="btn-ghost min-h-8 min-w-8 px-2 py-1"
                disabled={actionThreadId === thread.threadRootEventId}
                onclick={(event) => {
                  event.stopPropagation();
                  void unfollowThread(thread);
                }}
                title={m('room.message.meta.unfollow_thread')}
                aria-label={m('room.message.meta.unfollow_thread')}
              >
                <span class="iconify icon-[uil--bell-slash]" aria-hidden="true"></span>
              </button>
            </div>
          </article>
        {/each}
        {#if hasMore}
          <div class="flex min-h-14 justify-center p-4 text-muted" {@attach loadMoreWhenVisible}>
            {#if loadingMore}{m('common.loading')}{/if}
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
