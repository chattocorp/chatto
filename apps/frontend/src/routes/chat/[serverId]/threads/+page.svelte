<script lang="ts">
  import DirectMessageName from '$lib/components/users/DirectMessageName.svelte';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import ChatSearchInput from '$lib/components/chat/ChatSearchInput.svelte';
  import SearchAvailability from '$lib/components/search/SearchAvailability.svelte';
  import { MessageSearchState } from '$lib/api-client/messageSearch';
  import { useDebounce } from '$lib/hooks/useDebounce.svelte';
  import { createInfiniteQuery } from '@tanstack/svelte-query';
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import LoadingFog from '$lib/ui/LoadingFog.svelte';

  import { createThreadAPI, type FollowedThread } from '$lib/api-client/threads';
  import { createReadStateAPI } from '$lib/api-client/readState';
  import DaySeparator from '$lib/components/DaySeparator.svelte';
  import UserAvatarStack from '$lib/components/UserAvatarStack.svelte';
  import { queryClient } from '$lib/query/client';
  import {
    flattenFollowedThreads,
    threadQueryKeys,
    updateFollowedThreadSummary,
    type FollowedThreadsData
  } from '$lib/query/threads';
  import {
    ActivityListRow,
    EmptyState,
    Hint,
    PaneHeader,
    SegmentedControl,
    UnreadDot
  } from '$lib/ui';
  import { toast } from '$lib/ui/toast';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import {
    formatRelativeTime,
    groupByActivityDate,
    timeFormatSettingsFor
  } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { useLoadMoreWhenVisible } from '$lib/hooks/useLoadMoreWhenVisible.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import { buildDirectMessagePresentation } from '$lib/render/users';
  import { NotificationAttentionLevel } from '$lib/api-client/notifications';
  import { notificationAttentionForThread } from '$lib/state/server/notifications.svelte';

  const serverScope = useServerScope();
  const serverStore = $derived(serverScope.store);

  const userSettings = $derived(timeFormatSettingsFor(serverStore.currentUser.user?.settings));
  const activeLocale = $derived(getLocale());
  const PAGE_SIZE = 20;

  let actionThreadId = $state<string | null>(null);
  const debounce = useDebounce();
  // Tag transient input with the connection so switching servers or sessions
  // cannot submit a query from the previous viewer.
  let searchInput = $state({ scope: '', raw: '', submitted: '' });
  const inputScope = $derived(`${serverScope.serverId}:${serverScope.connection.queryScope}`);
  const rawQuery = $derived(searchInput.scope === inputScope ? searchInput.raw : '');
  const searchStatus = $derived(serverStore.messageSearch);
  const supportsSearch = $derived(serverStore.serverInfo.supportsFeature('followedThreadSearch'));
  const searchEnabled = $derived(
    supportsSearch && (searchStatus.statusError ||
      (searchStatus.statusLoaded && searchStatus.status.state !== MessageSearchState.DISABLED))
  );
  const searchQuery = $derived(
    searchEnabled && searchInput.scope === inputScope ? searchInput.submitted : ''
  );
  const waitingForSearch = $derived(searchEnabled && rawQuery.trim() !== searchQuery);

  $effect(() => {
    if (supportsSearch) void searchStatus.ensureStatus();
  });

  function scheduleSearch(raw: string): void {
    const scope = inputScope;
    searchInput = { scope, raw, submitted: searchQuery };
    debounce.cancel();
    if (!raw.trim()) {
      searchInput.submitted = '';
      return;
    }
    debounce.run(() => {
      if (inputScope === scope) searchInput.submitted = raw.trim();
    }, 300);
  }

  function submitSearch(): void {
    debounce.cancel();
    const unchanged = searchQuery === rawQuery.trim();
    searchInput = { scope: inputScope, raw: rawQuery, submitted: rawQuery.trim() };
    if (unchanged && searchStatus.available) void threadsQuery.refetch();
  }

  const threadsQuery = createInfiniteQuery(
    () => {
      const serverId = serverScope.serverId;
      const connection = serverScope.connection;
      const query = searchQuery;
      return {
        queryKey: threadQueryKeys.followed(serverId, connection, query),
        enabled: !query || searchStatus.available,
        queryFn: async ({ pageParam, signal }) => {
          const result = await connection
            .getAPI(createThreadAPI)
            .listFollowedThreads({
              limit: PAGE_SIZE,
              offset: typeof pageParam === 'number' ? pageParam : 0,
              cursor: typeof pageParam === 'string' ? pageParam : undefined,
              query
            }, { signal });
          const pageData = {
            ...result,
            nextOffset: (typeof pageParam === 'number' ? pageParam : 0) + result.threads.length
          };
          if (!serverScope.isCurrent() || connection !== serverScope.connection) return pageData;
          return pageData;
        },
        initialPageParam: query ? '' : 0,
        getNextPageParam: (lastPage, _pages, lastPageParam) =>
          query
            ? lastPage.nextCursor || undefined
            : lastPage.hasMore && typeof lastPageParam === 'number' && lastPage.nextOffset > lastPageParam
              ? lastPage.nextOffset : undefined
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
  const dateSections = $derived.by(() =>
    groupByActivityDate(
      filteredThreads,
      threadActivityAt,
      () => userSettings,
      new Date(),
      activeLocale
    )
  );

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

  function actorName(event: FollowedThread['rootMessage']): string {
    const actor = event?.actor;
    return actor
      ? getLiveDisplayName(actor.id, actor.displayName || actor.login)
      : '';
  }

  function rowActors(thread: FollowedThread): FollowedThread['participants'] {
    if (thread.isDirectMessage) {
      return buildDirectMessagePresentation(
        thread.directMessageParticipants,
        serverStore.currentUser.user?.id,
        m('common.you'),
        getLiveDisplayName
      ).visibleParticipants;
    }
    const participants = thread.participants.slice(0, 2);
    if (participants.length > 0) return participants;
    const actor = thread.latestReply?.actor ?? thread.rootMessage?.actor;
    return actor ? [actor] : [];
  }

  function roomLabel(thread: FollowedThread): string {
    if (!thread.isDirectMessage) return `#${thread.roomName}`;
    return buildDirectMessagePresentation(
      thread.directMessageParticipants,
      serverStore.currentUser.user?.id,
      m('common.you'),
      getLiveDisplayName
    ).label;
  }

  function primaryEvent(thread: FollowedThread): FollowedThread['rootMessage'] {
    return thread.latestReply ?? thread.rootMessage;
  }

  function threadActivityAt(thread: FollowedThread): string | null {
    return (
      thread.lastReplyAt ?? thread.latestReply?.createdAt ?? thread.rootMessage?.createdAt ?? null
    );
  }

  function replyCountLabel(count: number): string {
    return count === 1
      ? m('room.message.meta.reply_count_one')
      : m('room.message.meta.reply_count_many', { count });
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

  {#if searchEnabled}
    <div class="shrink-0 p-3">
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
      {#if searchStatus.status.state === MessageSearchState.DEGRADED}
        <Hint tone="warning">{m('search.degraded')}</Hint>
      {/if}
    </div>
  {/if}

  <div class="flex flex-1 flex-col overflow-y-auto">
    {#if searchQuery && !searchStatus.available}
      <SearchAvailability
        state={searchStatus.status.state}
        checking={searchStatus.statusLoading && !searchStatus.statusLoaded}
        error={searchStatus.statusError}
        onRetry={() => void searchStatus.refreshStatus()}
        checkingClass="p-6 text-muted"
      >
        <LoadingFog class="m-6 h-32" label={m('search.checking')} />
      </SearchAvailability>
    {:else if waitingForSearch || (loading && threads.length === 0)}
      <LoadingFog class="m-6 h-48" />
    {:else if error}
      <div class="m-6">
        <Hint tone="danger">{error}</Hint>
      </div>
    {:else if searchQuery && threads.length === 0}
      <EmptyState icon="icon-[uil--search]" title={m('search.no_threads')}>
        {m('search.no_results.description')}
      </EmptyState>
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
            {#each section.items as thread (thread.threadRootEventId)}
              {@const actors = rowActors(thread)}
              {@const primary = primaryEvent(thread)}
              {@const hasUnreadAttention = thread.hasUnreadReplies &&
                !serverStore.readViews.covers(thread.roomId, thread.threadRootEventId)}
              {@const attention = notificationAttentionForThread(
                serverStore.notifications.attentionOccurrences,
                thread.roomId,
                thread.threadRootEventId
              )}
              <ActivityListRow
                pending={actionThreadId === thread.threadRootEventId}
                disabled={actionThreadId === thread.threadRootEventId}
                dimmed={!hasUnreadAttention &&
                  attention === NotificationAttentionLevel.UNSPECIFIED}
                important={attention === NotificationAttentionLevel.IMPORTANT}
                onclick={() => navigateToThread(thread)}
                rowAttributes={{
                  'data-testid': 'my-thread-item',
                  'data-thread-state': hasUnreadAttention ? 'unread' : 'read',
                  'data-thread-attention':
                    attention === NotificationAttentionLevel.IMPORTANT
                      ? 'important'
                      : attention === NotificationAttentionLevel.AMBIENT
                        ? 'ambient'
                        : 'none'
                }}
              >
                {#snippet leading()}
                  <span class="relative flex shrink-0" aria-hidden="true">
                    <UserAvatarStack users={actors} />
                    {#if attention !== NotificationAttentionLevel.UNSPECIFIED}
                      <UnreadDot
                        color={attention === NotificationAttentionLevel.IMPORTANT
                          ? 'warning'
                          : 'ambient'}
                        overlay
                        class="absolute -end-1 -top-1"
                        testid="thread-attention-dot"
                      />
                    {/if}
                  </span>
                {/snippet}

                {#if hasUnreadAttention}<span class="sr-only"
                    >{m('chat.threads.filter_unread')}</span
                  >{/if}
                {#if attention !== NotificationAttentionLevel.UNSPECIFIED}<span class="sr-only"
                    >{m('room_list.notifications', { count: 1 })}</span
                  >{/if}
                <span class="min-w-0 flex-1" data-testid="thread-content">
                  <span class="flex min-w-0 items-baseline gap-2">
                    <bdi class="min-w-0 flex-1 truncate" dir="auto">
                      {#if actorName(primary)}
                        <span class="font-medium"
                          ><AccountName name={actorName(primary)} identity={primary?.actor} />:
                          <span class="font-normal">{messageExcerpt(primary)}</span></span
                        >
                      {:else}
                        <span>{messageExcerpt(primary)}</span>
                      {/if}
                    </bdi>
                    <span class="shrink-0 text-sm text-muted">
                      {formatRelativeTime(threadActivityAt(thread), userSettings, activeLocale)}
                    </span>
                  </span>
                  <span class="flex min-w-0 items-baseline gap-2 text-sm text-muted">
                    <bdi class="min-w-0 flex-1 truncate" dir="auto">
                      <span class="font-medium"
                        >{#if thread.isDirectMessage}
                          <DirectMessageName
                            participants={thread.directMessageParticipants}
                            currentUserId={serverStore.currentUser.user?.id}
                            getDisplayName={getLiveDisplayName}
                          />
                        {:else}{roomLabel(thread)}{/if}
                        {#if thread.latestReply}<span class="font-normal"
                            >· <AccountName name={actorName(thread.rootMessage)} identity={thread.rootMessage?.actor} />: {messageExcerpt(
                              thread.rootMessage
                            )}</span
                          >{/if}</span
                      >
                    </bdi>
                    <span class="shrink-0">{replyCountLabel(thread.replyCount)}</span>
                  </span>
                </span>

                {#snippet actions()}
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
                {/snippet}
              </ActivityListRow>
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
