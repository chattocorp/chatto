<script lang="ts">
  import { conversationPath } from '$lib/navigation/conversationPath';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { serverUi } from '$lib/state/server/serverUi';
  import DirectMessageName from '$lib/components/users/DirectMessageName.svelte';
  import ChatSearchInput from '$lib/components/chat/ChatSearchInput.svelte';
  import FeatureCompatibilityNotice from '$lib/components/chat/FeatureCompatibilityNotice.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import SearchAvailability from '$lib/components/search/SearchAvailability.svelte';
  import { MessageSearchState } from '@chatto/client/api/messageSearch';
  import { useDebounce } from '$lib/hooks/useDebounce.svelte';
  import { goto, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import {
    LoadingDots,
    EmptyState,
    Hint,
    PaneHeader,
    ScrollFader,
    SegmentedControl,
    PageTitle
  } from '$lib/ui';

  import {
    createThreadAPI,
    ThreadOrder,
    ThreadRootAuthorFilter,
    type FollowedThread
  } from '@chatto/client/api/threads';
  import { createReadStateAPI } from '@chatto/client/api/readState';
  import ActivityDaySection from '$lib/components/ActivityDaySection.svelte';
  import ThreadRow from './ThreadRow.svelte';
  import { threadRowActors } from './threadRowActors';
  import { createInfiniteQuery, queryClient } from '$lib/query/client';
  import {
    flattenFollowedThreads,
    nextUnreadFollowedThreadOffset,
    threadQueryKeys,
    updateFollowedThreadSummary,
    type FollowedThreadsData
  } from '$lib/query/threads';
  import { toast } from '$lib/ui/toast';
  import { groupByActivityDay, timeFormatSettingsFor } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { buildDirectMessagePresentation } from '@chatto/client/timeline/users';
  import { directMessageLabels } from '$lib/render/directMessageLabels';
  import { notificationAttentionForThread } from '@chatto/client/server/notifications';

  const serverScope = useServerScope();
  const serverStore = serverScope.store;

  const userSettings = $derived(timeFormatSettingsFor(serverStore.currentUser.user?.settings));
  const activeLocale = $derived(getLocale());
  const PAGE_SIZE = 20;

  let actionThreadId = $state<string | null>(null);
  const debounce = useDebounce();
  let searchInput = $state({ raw: '', submitted: '' });
  const rawQuery = $derived(searchInput.raw);
  const searchStatus = $derived(serverUi(serverStore).messageSearch.availability);
  const searchEnabled = $derived(
    searchStatus.statusError ||
      (searchStatus.statusLoaded && searchStatus.status.state !== MessageSearchState.DISABLED)
  );
  const searchQuery = $derived(searchEnabled ? searchInput.submitted : '');
  const waitingForSearch = $derived(searchEnabled && rawQuery.trim() !== searchQuery);

  function scheduleSearch(raw: string): void {
    searchInput = { raw, submitted: searchQuery };
    debounce.cancel();
    if (!raw.trim()) {
      searchInput.submitted = '';
      return;
    }
    debounce.run(() => {
      searchInput.submitted = raw.trim();
    }, 300);
  }

  function submitSearch(): void {
    debounce.cancel();
    const unchanged = searchQuery === rawQuery.trim();
    searchInput = { raw: rawQuery, submitted: rawQuery.trim() };
    if (unchanged && searchStatus.available) void threadsQuery.refetch();
  }

  const filter = $derived(page.state.threadFilter ?? 'all');
  const tab = $derived(page.state.threadTab ?? 'followed');
  const order = $derived(page.state.threadOrder ?? 'activity');
  const orderOptions = $derived([
    { value: 'activity' as const, label: m('chat.threads.sort_activity') },
    { value: 'created' as const, label: m('chat.threads.sort_created') }
  ]);

  function setOrder(value: 'activity' | 'created') {
    replaceState('', { ...page.state, threadOrder: value });
  }
  const rootAuthorFilter = $derived(
    tab === 'mine' ? ThreadRootAuthorFilter.VIEWER : ThreadRootAuthorFilter.UNSPECIFIED
  );
  const tabOptions = $derived([
    { value: 'followed' as const, label: m('chat.threads.tab_followed') },
    { value: 'mine' as const, label: m('chat.threads.tab_started') }
  ]);

  function setTab(value: 'mine' | 'followed') {
    replaceState('', { ...page.state, threadTab: value });
  }
  const filterOptions = $derived([
    { value: 'all' as const, label: m('chat.threads.filter_all') },
    { value: 'unread' as const, label: m('chat.threads.filter_unread') }
  ]);

  function setFilter(value: 'all' | 'unread') {
    replaceState('', { ...page.state, threadFilter: value });
  }

  const threadsQuery = createInfiniteQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    const query = searchQuery;
    const authorFilter = rootAuthorFilter;
    const selectedOrder = order;
    // Search results are filtered locally below.
    const unreadOnly = !query && filter === 'unread';
    return {
      queryKey: threadQueryKeys.followed(serverId, connection, {
        query,
        unreadOnly,
        rootAuthorFilter: authorFilter,
        order: selectedOrder
      }),
      enabled: !query || searchStatus.available,
      // Inbox state can change while this page or tab is inactive. Keep cached
      // rows for display, but refresh them whenever this query becomes active.
      staleTime: 0,
      queryFn: async ({ pageParam, signal }) => {
        const result = await connection.getAPI(createThreadAPI).listFollowedThreads(
          {
            limit: PAGE_SIZE,
            offset: typeof pageParam === 'number' ? pageParam : 0,
            cursor: typeof pageParam === 'string' ? pageParam : undefined,
            query,
            rootAuthorFilter: authorFilter,
            order: selectedOrder === 'created' ? ThreadOrder.CREATED : ThreadOrder.ACTIVITY,
            unreadOnly
          },
          { signal }
        );
        const pageData = {
          ...result,
          nextOffset: (typeof pageParam === 'number' ? pageParam : 0) + result.threads.length
        };
        if (!serverScope.isCurrent() || connection !== serverScope.connection) return pageData;
        return pageData;
      },
      initialPageParam: query ? '' : 0,
      getNextPageParam: (lastPage, pages, lastPageParam) => {
        if (query) return lastPage.nextCursor || undefined;
        if (!lastPage.hasMore || typeof lastPageParam !== 'number') return undefined;
        if (unreadOnly) {
          return lastPage.threads.length > 0 ? nextUnreadFollowedThreadOffset(pages) : undefined;
        }
        return lastPage.nextOffset > lastPageParam ? lastPage.nextOffset : undefined;
      }
    };
  });

  const threads = $derived(flattenFollowedThreads(threadsQuery.data));
  const loading = $derived(threadsQuery.isPending);
  const loadingMore = $derived(threadsQuery.isFetchingNextPage);
  const error = $derived(
    threadsQuery.isError ? errorMessage(threadsQuery.error, m('chat.threads.load_failed')) : null
  );
  const hasMore = $derived(threadsQuery.hasNextPage);

  // Also filter server-filtered pages so that a thread marked as read leaves the
  // unread view at once, before the next refetch.
  const filteredThreads = $derived(
    filter === 'unread' ? threads.filter((t) => t.hasUnreadReplies) : threads
  );
  const dateSections = $derived.by(() => {
    const sections = groupByActivityDay(
      filteredThreads,
      threadDate,
      () => userSettings,
      new Date(),
      activeLocale
    );
    const undated = filteredThreads.filter((thread) => !threadDate(thread));
    if (undated.length)
      sections.push({ key: 'unknown', label: m('common.unknown'), items: undated });
    return sections;
  });

  // Locally filtered pages without a match leave no scroll edge to observe, so
  // load the next page directly until a match appears or the feed ends.
  // An imperative fetch ignores `enabled`, so check search availability here too.
  const needsNextFilteredPage = $derived(
    filteredThreads.length === 0 &&
      hasMore &&
      !threadsQuery.isFetching &&
      error === null &&
      (!searchQuery || searchStatus.available)
  );
  $effect(() => {
    if (needsNextFilteredPage) void threadsQuery.fetchNextPage();
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
      conversationPath(serverScope.serverId, thread.roomId, {
        threadId: thread.threadRootEventId,
        store: serverScope.store
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
      queryClient.setQueriesData<FollowedThreadsData>({ queryKey }, (current) =>
        updateFollowedThreadSummary(current, {
          roomId: thread.roomId,
          threadRootEventId: thread.threadRootEventId,
          replyCount: thread.replyCount,
          lastReplyAt: thread.lastReplyAt,
          hasUnreadReplies: false
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
        exact: false
      });
    } catch {
      toast.error(m('common.error.generic'));
    } finally {
      actionThreadId = null;
    }
  }

  function messageExcerpt(event: FollowedThread['rootMessage']): string {
    if (!event || event.event.kind !== 'messagePosted') return m('chat.threads.message_missing');
    if (event.event.deletedAt) return m('room.message.deleted');
    const body = event.event.body?.trim();
    if (body) return body;
    if (event.event.attachments.length > 1) {
      return m('message_preview.attachments_count', { count: event.event.attachments.length });
    }
    const attachment = event.event.attachments[0];
    if (attachment) {
      if (attachment.filename) return attachment.filename;
      if (attachment.contentType.startsWith('image/')) return m('message_preview.attachment_image');
      if (attachment.contentType.startsWith('video/')) return m('message_preview.attachment_video');
      if (attachment.contentType.startsWith('audio/')) return m('message_preview.attachment_audio');
      return m('message_preview.attachment_file');
    }
    const preview = event.event.linkPreview;
    return preview?.title || preview?.siteName || preview?.url || m('chat.threads.message_missing');
  }

  /** Participant preview for a row; see `threadRowActors`. */
  function rowActors(thread: FollowedThread): FollowedThread['participants'] {
    const others = thread.isDirectMessage
      ? buildDirectMessagePresentation(
          thread.directMessageParticipants,
          serverStore.viewerId,
          directMessageLabels(),
          getLiveDisplayName
        ).visibleParticipants
      : thread.participants;
    return threadRowActors(thread.rootMessage?.actor, others, thread.latestReply?.actor);
  }

  function roomLabel(thread: FollowedThread): string {
    if (!thread.isDirectMessage) return `#${thread.roomName}`;
    return buildDirectMessagePresentation(
      thread.directMessageParticipants,
      serverStore.viewerId,
      directMessageLabels(),
      getLiveDisplayName
    ).label;
  }

  function threadActivityAt(thread: FollowedThread): string | null {
    return (
      thread.lastReplyAt ?? thread.latestReply?.createdAt ?? thread.rootMessage?.createdAt ?? null
    );
  }

  /** Keep day headings and row times aligned with the selected sort order. */
  function threadDate(thread: FollowedThread): string | null {
    return order === 'created' ? (thread.rootMessage?.createdAt ?? null) : threadActivityAt(thread);
  }
</script>

<PageTitle title={m('chat.threads.title')} />

<div class="pane-page">
  <PaneHeader title={m('chat.threads.title')} subtitle={m('chat.threads.subtitle')} />

  <div class="flex min-h-0 max-w-pane flex-1 flex-col">
    <div class="flex shrink-0 flex-col gap-3 px-4 py-3">
      <FeatureCompatibilityNotice
        serverSegment={serverIdToSegment(serverScope.serverId)}
        features={serverStore.serverInfo.limitedFeatures.filter(
          (feature) => feature === 'threadViews'
        )}
      />
      <div class="flex flex-wrap items-center gap-3">
        {#if searchEnabled}
          <div class="min-w-0 flex-1 basis-56">
            <ChatSearchInput
              appearance="bordered"
              label={m('search.in_threads')}
              placeholder={m('search.query.placeholder')}
              testid="my-threads-search"
              focusOnMount
              value={rawQuery}
              clearLabel={m('common.clear')}
              oninput={(event) => scheduleSearch((event.currentTarget as HTMLInputElement).value)}
              onclear={() => scheduleSearch('')}
              onsubmit={submitSearch}
            />
          </div>
        {/if}
        <div class="segmented-control-group flex-auto">
          <SegmentedControl
            label={m('chat.threads.tab_label')}
            options={tabOptions}
            value={tab}
            onchange={setTab}
          />
          <SegmentedControl
            label={m('chat.threads.filter_label')}
            options={filterOptions}
            value={filter}
            onchange={setFilter}
          />
          <SegmentedControl
            label={m('chat.threads.sort_label')}
            options={orderOptions}
            value={order}
            onchange={setOrder}
          />
        </div>
      </div>
      {#if searchEnabled && searchStatus.status.state === MessageSearchState.DEGRADED}
        <Hint tone="warning">{m('search.degraded')}</Hint>
      {/if}
    </div>
    <ScrollFader
      top
      bottom
      keyboardFocusable={false}
      class="min-h-0 flex-1 px-3 [&_section:first-child>h2]:mt-0"
    >
      <div class="flex min-h-full flex-col">
        {#if searchQuery && !searchStatus.available}
          <SearchAvailability
            state={searchStatus.status.state}
            checking={searchStatus.statusLoading && !searchStatus.statusLoaded}
            error={searchStatus.statusError}
            onRetry={() => void searchStatus.refreshStatus()}
            checkingClass="p-6 text-muted"
          >
            <LoadingDots class="py-6" label={m('search.checking')} />
          </SearchAvailability>
        {:else if waitingForSearch || (loading && threads.length === 0)}
          <LoadingDots class="py-6" />
        {:else if error}
          <div class="m-6">
            <Hint tone="danger">{error}</Hint>
          </div>
        {:else if searchQuery && threads.length === 0}
          <EmptyState icon="icon-[uil--search]" title={m('search.no_threads')}>
            {m('search.no_results.description')}
          </EmptyState>
        {:else if threads.length === 0 && filter === 'all'}
          <EmptyState
            icon="icon-[uil--comment-lines]"
            title={tab === 'mine'
              ? m('chat.threads.empty_mine_title')
              : m('chat.threads.empty_title')}
          >
            {tab === 'mine' ? m('chat.threads.empty_mine_body') : m('chat.threads.empty_body')}
          </EmptyState>
        {:else if filteredThreads.length === 0 && hasMore}
          <LoadingDots class="py-6" />
        {:else if filteredThreads.length === 0}
          <EmptyState icon="icon-[uil--comment-check]" title={m('chat.threads.all_caught_up')}>
            {m('chat.threads.no_unread')}
          </EmptyState>
        {:else}
          <div class="selectable-list pb-3" aria-busy={loadingMore}>
            {#each dateSections as section (section.key)}
              <ActivityDaySection
                dayKey={section.key}
                label={section.label}
                id={`thread-date-${section.key}`}
              >
                {#snippet children(rowTime)}
                  {#each section.items as thread (thread.threadRootEventId)}
                    {@const actors = rowActors(thread)}
                    {@const hasUnreadAttention =
                      thread.hasUnreadReplies &&
                      !serverUi(serverStore).readViews.covers(
                        thread.roomId,
                        thread.threadRootEventId
                      )}
                    {@const attention = notificationAttentionForThread(
                      serverUi(serverStore).attention.occurrences,
                      thread.roomId,
                      thread.threadRootEventId
                    )}
                    <ThreadRow
                      participants={actors}
                      message={messageExcerpt(thread.rootMessage)}
                      time={rowTime(threadDate(thread), userSettings, activeLocale)}
                      unread={hasUnreadAttention}
                      {attention}
                      pending={actionThreadId === thread.threadRootEventId}
                      canMarkRead={thread.hasUnreadReplies && thread.latestReply !== null}
                      onopen={() => navigateToThread(thread)}
                      onmarkread={() => void markThreadRead(thread)}
                      onunfollow={() => void unfollowThread(thread)}
                    >
                      {#snippet room()}
                        {#if thread.isDirectMessage}
                          <DirectMessageName
                            participants={thread.directMessageParticipants}
                            currentUserId={serverStore.viewerId}
                            getDisplayName={getLiveDisplayName}
                          />
                        {:else}{roomLabel(thread)}{/if}
                      {/snippet}
                    </ThreadRow>
                  {/each}
                {/snippet}
              </ActivityDaySection>
            {/each}
            {#if hasMore}
              <div class="flex min-h-14 justify-center p-4" {@attach loadMoreWhenVisible}>
                {#if loadingMore}<LoadingDots />{/if}
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </ScrollFader>
  </div>
</div>
