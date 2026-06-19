<script lang="ts">
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { ListMyFollowedThreadsRequest, type FollowedThreadView } from '$lib/pb/chatto/api/v1/chat_pb';
  import type { RoomEventViewFragment } from '$lib/chatTypes';
  import { wireRoomEventViewToFragment } from '$lib/wire';
  import { wireMessagePosted, wireThreadFollowChanged } from '$lib/wire/events';
  import { wireEventBusManager } from '$lib/state/server/wireEventBus.svelte';
  import { EmptyState, Hint, PaneHeader } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import RoomEvent from '../[roomId]/RoomEvent.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate } from '$lib/utils/formatTime';
  import { useWireEvent } from '$lib/hooks';
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

  const userSettings = getUserSettings();
  const PAGE_SIZE = 20;

  type FollowedThreadItem = {
    roomId: string;
    roomName: string;
    threadRootEventId: string;
    rootMessage: RoomEventViewFragment | null;
    replyCount: number;
    lastReplyAt: string | null;
    hasUnread: boolean;
  };

  function mapThread(t: FollowedThreadView): FollowedThreadItem {
    return {
      roomId: t.roomId,
      roomName: t.room?.name ?? t.roomId,
      threadRootEventId: t.threadRootEventId,
      rootMessage: wireRoomEventViewToFragment(t.rootMessage),
      replyCount: t.replyCount,
      lastReplyAt: t.lastReplyAt?.toDate().toISOString() ?? null,
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
      const serverId = getActiveServer();
      const client = wireEventBusManager.getClient(serverId);
      if (!client) throw new Error('Not connected to server');

      const page = await client.listMyFollowedThreads(
        new ListMyFollowedThreadsRequest({
          limit: PAGE_SIZE,
          offset: append ? threads.length : 0
        })
      );

      if (thisId !== loadId) return;

      const nextThreads = page.threads.map(mapThread);
      threads = append ? mergeThreads(threads, nextThreads) : nextThreads;
      hasMore = page.hasMore;
      totalCount = page.totalCount;
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

  useWireEvent((streamEvent) => {
    const followChanged = wireThreadFollowChanged(streamEvent);
    if (followChanged) {
      loadThreads();
      return;
    }

    const messagePosted = wireMessagePosted(streamEvent);
    if (messagePosted?.threadRootEventId) {
      if (threads.some((t) => t.threadRootEventId === messagePosted.threadRootEventId)) {
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

    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    if (diffHours < 24) return `${diffHours}h ago`;
    if (diffDays < 7) return `${diffDays}d ago`;

    return formatDate(date, userSettings);
  }
</script>

<PageTitle title="My Threads" />

<div class="flex h-full w-full flex-col">
  <PaneHeader title="My Threads" subtitle="Threads you're following" showMobileNav>
    {#snippet actions()}
      <div
        class="flex rounded-md border border-border text-sm"
        role="radiogroup"
        aria-label="Filter threads"
      >
        <button
          class={[
            'cursor-pointer rounded-l-md px-3 py-1',
            filter === 'all' ? 'bg-surface-200 font-medium' : 'text-muted hover:bg-surface-100'
          ]}
          onclick={() => setFilter('all')}
          role="radio"
          aria-checked={filter === 'all'}>All</button
        >
        <button
          class={[
            'cursor-pointer rounded-r-md border-l border-border px-3 py-1',
            filter === 'unread' ? 'bg-surface-200 font-medium' : 'text-muted hover:bg-surface-100'
          ]}
          onclick={() => setFilter('unread')}
          role="radio"
          aria-checked={filter === 'unread'}>Unread</button
        >
      </div>
    {/snippet}
  </PaneHeader>

  <div class="flex flex-1 flex-col overflow-y-auto">
    {#if loading && threads.length === 0}
      <div class="p-6 text-muted">Loading...</div>
    {:else if error}
      <div class="m-6">
        <Hint tone="danger">{error}</Hint>
      </div>
    {:else if threads.length === 0}
      <EmptyState icon="uil--comment-lines" title="No followed threads">
        Threads you follow will appear here. You automatically follow threads you participate in.
      </EmptyState>
    {:else if filteredThreads.length === 0}
      <EmptyState
        icon="uil--comment-check"
        title={hasMore ? 'No unread threads loaded' : 'All caught up'}
      >
        {#if hasMore}
          <div class="flex flex-col items-center gap-3">
            <span>{threads.length} of {totalCount} followed threads loaded.</span>
            <Button
              variant="secondary"
              size="sm"
              loading={loadingMore}
              onclick={() => loadThreads({ append: true })}
            >
              Load more
            </Button>
          </div>
        {:else}
          No unread threads right now.
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
                  >{#if thread.lastReplyAt}{formatRelativeTime(thread.lastReplyAt)}, in{:else}In{/if}
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
                  <p class="text-sm text-muted">Message no longer available</p>
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
              Load more
            </Button>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</div>
