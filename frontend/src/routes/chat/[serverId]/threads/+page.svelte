<script lang="ts">
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';

  import { graphql } from '$lib/gql';
  import { useFragment } from '$lib/gql/fragment-masking';
  import type { MyFollowedThreadsQuery as MyFollowedThreadsQueryType } from '$lib/gql/graphql';
  import { RoomEventViewFragmentDoc, type RoomEventViewFragment } from '$lib/gql/graphql';
  import { EmptyState, Hint, PaneHeader } from '$lib/ui';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';
  import RoomEvent from '../[roomId]/RoomEvent.svelte';
  import { getUserSettings } from '$lib/state/userSettings.svelte';
  import { formatDate } from '$lib/utils/formatTime';
  import { onThreadFollowChanged } from '$lib/eventBus.svelte';
  import { useEvent } from '$lib/hooks';
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
  const PAGE_SIZE = 20;

  // prettier-ignore
  const MyFollowedThreadsDoc = graphql(`
		query MyFollowedThreads($limit: Int!, $offset: Int!) {
			viewer {
				followedThreads(limit: $limit, offset: $offset) {
					threads {
						roomId
						room {
							name
						}
						threadRootEventId
						rootMessage {
							...RoomEventView
						}
						replyCount
						lastReplyAt
						threadParticipants(first: 3) {
							...UserAvatarUser
						}
						hasUnread
					}
					totalCount
					hasMore
				}
			}
		}
	`);

  type RawThread = NonNullable<
    MyFollowedThreadsQueryType['viewer']
  >['followedThreads']['threads'][number];

  type FollowedThreadItem = {
    roomId: string;
    roomName: string;
    threadRootEventId: string;
    rootMessage: RoomEventViewFragment | null;
    replyCount: number;
    lastReplyAt: string | null;
    hasUnread: boolean;
  };

  function mapThread(t: RawThread): FollowedThreadItem {
    const rootMessage = t.rootMessage ? useFragment(RoomEventViewFragmentDoc, t.rootMessage) : null;

    return {
      roomId: t.roomId,
      roomName: t.room.name,
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
      const result = await connection()
        .client.query(MyFollowedThreadsDoc, {
          limit: PAGE_SIZE,
          offset: append ? threads.length : 0
        })
        .toPromise();

      if (thisId !== loadId) return;

      if (result.error) {
        error = result.error.message;
      } else if (result.data?.viewer) {
        const page = result.data.viewer.followedThreads;
        const nextThreads = page.threads.map(mapThread);
        threads = append ? mergeThreads(threads, nextThreads) : nextThreads;
        hasMore = page.hasMore;
        totalCount = page.totalCount;
      }
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
    if (event.__typename === 'MessagePostedEvent' && event.threadRootEventId) {
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
