<script lang="ts">
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import * as m from '$lib/i18n/messages';

  import { useRenderData } from '$lib/render/data';
  import { RoomEventViewDocument, type RoomEventView } from '$lib/render/types';
  import { createThreadAPI, type FollowedThread as APIFollowedThread } from '$lib/api-client/threads';
  import { EmptyState, Hint, PaneHeader } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import RoomEvent from '../[roomId]/RoomEvent.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate } from '$lib/utils/formatTime';
  import { getLocale } from '$lib/i18n/runtime';
  import { onThreadFollowChanged } from '$lib/eventBus.svelte';
  import { useEvent } from '$lib/hooks';
  import { isMessagePostedEvent } from '$lib/render/eventKinds';
  import {
    createRoomPermissions,
    DEFAULT_ROOM_PERMISSIONS,
    createRoomMembers,
    createComposerContext,
    createMentionRoles
  } from '$lib/state/room';

  // Provide stub room contexts so MessageEvent can render in read-only mode.
  // All permissions are false (no editing, deleting, reacting from this view),
  // members list is empty (no mention highlighting), composer context is a no-op.
  createRoomPermissions(() => DEFAULT_ROOM_PERMISSIONS);
  createRoomMembers();
  createComposerContext();
  createMentionRoles();

  const connection = useConnection();
  const userSettings = getUserSettings();
  const activeLocale = $derived(getLocale());
  const PAGE_SIZE = 20;

  type FollowedThreadItem = {
    roomId: string;
    roomName: string;
    threadRootEventId: string;
    rootMessage: RoomEventView | null;
    replyCount: number;
    lastReplyAt: string | null;
    hasUnread: boolean;
  };

  function mapThread(t: APIFollowedThread): FollowedThreadItem {
    const rootMessage = t.rootMessage ? useRenderData(RoomEventViewDocument, t.rootMessage) : null;

    return {
      roomId: t.roomId,
      roomName: t.roomName,
      threadRootEventId: t.threadRootEventId,
      rootMessage,
      replyCount: t.replyCount,
      lastReplyAt: t.lastReplyAt,
      hasUnread: t.hasUnread
    };
  }

  let threads = $state<FollowedThreadItem[]>([]);
  let loading = $state(true);
  let loadingMore = $state(false);
  let error = $state<string | null>(null);
  let hasMore = $state(false);
  let totalCount = $state(0);
  let loadId = 0;

  const filter = $derived(page.state.threadFilter ?? 'all');

  function setFilter(value: 'all' | 'unread') {
    replaceState('', { ...page.state, threadFilter: value });
  }

  const filteredThreads = $derived(
    filter === 'unread' ? threads.filter((t) => t.hasUnread) : threads
  );

  async function loadThreads({ append = false }: { append?: boolean } = {}) {
    const thisId = ++loadId;
    if (append) {
      loadingMore = true;
    } else {
      loading = true;
    }
    error = null;

    try {
      const conn = connection();
      const result = await createThreadAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }).listFollowedThreads({
        limit: PAGE_SIZE,
        offset: append ? threads.length : 0
      });

      if (thisId !== loadId) return;

      const nextThreads = result.threads.map(mapThread);
      threads = append ? mergeThreads(threads, nextThreads) : nextThreads;
      hasMore = result.hasMore;
      totalCount = result.totalCount;
    } catch (e) {
      if (thisId !== loadId) return;
      error = e instanceof Error ? e.message : 'Failed to load threads';
    } finally {
      if (thisId === loadId) {
        loading = false;
        loadingMore = false;
      }
    }
  }

  function mergeThreads(
    existing: FollowedThreadItem[],
    next: FollowedThreadItem[]
  ): FollowedThreadItem[] {
    const seen = new Set(existing.map((thread) => thread.threadRootEventId));
    return [...existing, ...next.filter((thread) => !seen.has(thread.threadRootEventId))];
  }

  $effect(() => {
    loadThreads();
  });

  // Real-time: Refresh when thread follow state changes
  $effect(() => onThreadFollowChanged(() => loadThreads()));

  // Real-time: Refresh when a new thread reply arrives
  useEvent((spaceEvent) => {
    const event = spaceEvent.event;
    if (!event) return;
    if (isMessagePostedEvent(event) && event.threadRootEventId) {
      // Only refresh if it's a reply in a thread we're displaying
      if (threads.some((t) => t.threadRootEventId === event.threadRootEventId)) {
        loadThreads();
      }
    }
  });

  function navigateToThread(thread: FollowedThreadItem) {
    goto(
      resolve('/chat/[serverId]/[roomId]/[threadId]', {
        serverId: serverIdToSegment(getActiveServer()),
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
      <div
        class="flex rounded-md border border-border text-sm"
        role="radiogroup"
        aria-label={m['chat.threads.filter_label']()}
      >
        <button
          class={[
            'cursor-pointer rounded-l-md px-3 py-1',
            filter === 'all' ? 'bg-surface-200 font-medium' : 'text-muted hover:bg-surface-100'
          ]}
          onclick={() => setFilter('all')}
          role="radio"
          aria-checked={filter === 'all'}>{m['chat.threads.filter_all']()}</button
        >
        <button
          class={[
            'cursor-pointer rounded-r-md border-l border-border px-3 py-1',
            filter === 'unread' ? 'bg-surface-200 font-medium' : 'text-muted hover:bg-surface-100'
          ]}
          onclick={() => setFilter('unread')}
          role="radio"
          aria-checked={filter === 'unread'}>{m['chat.threads.filter_unread']()}</button
        >
      </div>
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
            <Button
              variant="secondary"
              size="sm"
              loading={loadingMore}
              onclick={() => loadThreads({ append: true })}
            >
              {m['chat.threads.load_more']()}
            </Button>
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
          <div class="flex justify-center p-4">
            <Button
              variant="secondary"
              loading={loadingMore}
              onclick={() => loadThreads({ append: true })}
            >
              {m['chat.threads.load_more']()}
            </Button>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
