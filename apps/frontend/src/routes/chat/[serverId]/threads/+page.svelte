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
  import DaySeparator from '$lib/components/DaySeparator.svelte';
  import UserAvatar from '$lib/components/UserAvatar.svelte';
  import { queryClient } from '$lib/query/client';
  import {
    flattenFollowedThreads,
    reconcileFollowedThreadViewerStates,
    threadQueryKeys,
    updateFollowedThreadSummary,
    type FollowedThreadsData
  } from '$lib/query/threads';
  import { EmptyState, Hint, PaneHeader, SegmentedControl, UnreadDot } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import {
    fileDateGroup,
    formatDate,
    formatMonthYear,
    timeFormatSettingsFor
  } from '$lib/utils/formatTime';
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
  const dateSections = $derived.by(() => groupThreadsByDate(filteredThreads));

  type ThreadDateSection = {
    key: string;
    label: string;
    threads: FollowedThread[];
  };

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

  function rowActors(thread: FollowedThread): FollowedThread['participants'] {
    const participants = thread.participants.slice(0, 2);
    if (participants.length > 0) return participants;
    const actor = thread.latestReply?.actor ?? thread.rootMessage?.actor;
    return actor ? [actor] : [];
  }

  function threadActivityAt(thread: FollowedThread): string | null {
    return (
      thread.lastReplyAt ?? thread.latestReply?.createdAt ?? thread.rootMessage?.createdAt ?? null
    );
  }

  function groupThreadsByDate(items: FollowedThread[]): ThreadDateSection[] {
    const sections: ThreadDateSection[] = [];
    const now = new Date();
    for (const thread of items) {
      const activityAt = threadActivityAt(thread);
      if (!activityAt) continue;
      const dateGroup = fileDateGroup(activityAt, userSettings, now, activeLocale);
      const label =
        dateGroup.key === 'this-month'
          ? formatMonthYear(activityAt, userSettings, activeLocale)
          : dateGroup.label;
      const key = dateGroup.key === 'this-month' ? `this-month:${label}` : dateGroup.key;
      let section = sections.find((candidate) => candidate.key === key);
      if (!section) {
        section = { key, label, threads: [] };
        sections.push(section);
      }
      section.threads.push(thread);
    }
    return sections;
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
      <div class="selectable-list pb-3" aria-busy={loadingMore}>
        {#each dateSections as section (section.key)}
          <section aria-labelledby={`thread-date-${section.key}`}>
            <DaySeparator id={`thread-date-${section.key}`} label={section.label} />
            {#each section.threads as thread (thread.threadRootEventId)}
              {@const actors = rowActors(thread)}
              <article
                class={[
                  'group flex w-full items-center gap-3 selectable-list-item px-3 py-2.5',
                  thread.attention === 'important' && 'bg-attention/5'
                ]}
                data-testid="my-thread-item"
                data-thread-state={thread.hasUnreadReplies ? 'unread' : 'read'}
                data-thread-attention={thread.attention}
              >
                <button
                  type="button"
                  class={[
                    'flex min-w-0 flex-1 cursor-pointer items-center gap-3 rounded-md text-start focus-visible:outline-2 focus-visible:outline-action',
                    !thread.hasUnreadReplies && thread.attention === 'none' && 'opacity-60'
                  ]}
                  disabled={actionThreadId === thread.threadRootEventId}
                  onclick={() => navigateToThread(thread)}
                >
                  <span class="relative flex shrink-0" aria-hidden="true">
                    {#if actors.length > 1}
                      <span class="flex -space-x-2 rtl:space-x-reverse">
                        {#each actors as actor (actor.id)}
                          <UserAvatar
                            user={actor}
                            size="md"
                            class="ring-2 ring-background"
                            useLiveProfile={false}
                          />
                        {/each}
                      </span>
                    {:else if actors[0]}
                      <UserAvatar user={actors[0]} size="md" useLiveProfile={false} />
                    {/if}
                    {#if thread.attention !== 'none'}
                      <UnreadDot
                        color={thread.attention === 'important' ? 'warning' : 'ambient'}
                        overlay
                        class="absolute -end-1 -top-1"
                        testid="thread-attention-dot"
                      />
                    {/if}
                  </span>

                  {#if thread.hasUnreadReplies}<span class="sr-only"
                      >{m('chat.threads.filter_unread')}</span
                    >{/if}
                  <span class="min-w-0 flex-1" data-testid="thread-content">
                    <span class="flex min-w-0 items-baseline gap-2">
                      <bdi class="min-w-0 flex-1 truncate" dir="auto">
                        {#if actorName(thread.latestReply)}
                          <span class="font-medium"
                            >{actorName(thread.latestReply)}:
                            <span class="font-normal">{messageExcerpt(thread.latestReply)}</span
                            ></span
                          >
                        {:else}
                          <span>{messageExcerpt(thread.latestReply)}</span>
                        {/if}
                      </bdi>
                      <span class="shrink-0 text-sm text-muted">
                        {formatRelativeTime(threadActivityAt(thread))}
                      </span>
                    </span>
                    <span class="flex min-w-0 items-baseline gap-2 text-sm text-muted">
                      <bdi class="min-w-0 flex-1 truncate" dir="auto">
                        <span class="font-medium"
                          >#{thread.roomName}
                          <span class="font-normal"
                            >· {actorName(thread.rootMessage)}: {messageExcerpt(
                              thread.rootMessage
                            )}</span
                          ></span
                        >
                      </bdi>
                      <span class="shrink-0">{replyCountLabel(thread.replyCount)}</span>
                    </span>
                  </span>
                </button>

                <div class="hover-reveal-action flex shrink-0 items-center">
                  {#if thread.hasUnreadReplies && thread.latestReply}
                    <button
                      type="button"
                      class="icon-action"
                      disabled={actionThreadId === thread.threadRootEventId}
                      onclick={() => void markThreadRead(thread)}
                      title={m('room_list.mark_as_read')}
                      aria-label={m('room_list.mark_as_read')}
                    >
                      <span class="iconify icon-[uil--check] text-base" aria-hidden="true"></span>
                    </button>
                  {/if}
                  <button
                    type="button"
                    class="icon-action"
                    disabled={actionThreadId === thread.threadRootEventId}
                    onclick={() => void unfollowThread(thread)}
                    title={m('room.message.meta.unfollow_thread')}
                    aria-label={m('room.message.meta.unfollow_thread')}
                  >
                    <span class="iconify icon-[uil--bell-slash] text-base" aria-hidden="true"
                    ></span>
                  </button>
                </div>
              </article>
            {/each}
          </section>
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
