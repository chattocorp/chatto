import type { RoomEventViewFragment } from '$lib/gql/graphql';
import { MessageListStore } from './MessageListStore.svelte';
import { PAGE_SIZE, ThreadEventsQuery } from './queries';
import { isThreadEvent } from './filters';
import { type EventConnectionPage, threadRepliesConnection, unmask } from './helpers';

/**
 * Message store for a single thread. Loads the root plus a paginated reply
 * page, and only accepts MessagePostedEvents that target this thread. System
 * events (joined/left/etc.) are ignored.
 */
export class ThreadMessagesStore extends MessageListStore {
  private threadRootEventId = '';

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): RoomEventViewFragment[] {
    return this.events.filter((e) => isThreadEvent(e, this.roomId, this.threadRootEventId));
  }

  /** Switch to a thread (or force-refetch the current one). Always shows
   *  the skeleton. Silent reconnect / tab-resume catch-ups go through
   *  {@link catchUp}, not through this method.
   */
  setThread(roomId: string, threadRootEventId: string): void {
    this.roomId = roomId;
    this.threadRootEventId = threadRootEventId;

    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchThread(thisLoad);
  }

  protected catchUp(): void {
    if (!this.threadRootEventId) return;
    const thisLoad = this.startLoad();
    if (this.events.length === 0) {
      this.fetchThread(thisLoad);
    } else {
      this.catchUpForward(thisLoad);
    }
  }

  async refetchAll(): Promise<void> {
    const snapshot = [...this.threadEvents];
    for (const event of snapshot) {
      await this.refetchOne(event.id);
    }
  }

  protected async fetchOlderPage(before: string): Promise<EventConnectionPage | null> {
    const result = await this.client
      .query(ThreadEventsQuery, {
        roomId: this.roomId,
        threadRootEventId: this.threadRootEventId,
        limit: PAGE_SIZE,
        before
      })
      .toPromise();

    return threadRepliesConnection(result.data?.room?.event);
  }

  protected afterOlderPagePrepended(): void {
    this.sortThreadEvents();
  }

  protected onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
    if (eventData.threadRootEventId === this.threadRootEventId) {
      this.addEvent(spaceEvent);
    }
  }

  private fetchThread(thisLoad: number): void {
    this.client
      .query(ThreadEventsQuery, {
        roomId: this.roomId,
        threadRootEventId: this.threadRootEventId,
        limit: PAGE_SIZE
      })
      .toPromise()
      .then((result) => {
        if (this.isStale(thisLoad)) return;
        if (result.error) console.error('ThreadMessagesStore: fetch error:', result.error);
        const root = result.data?.room?.event;
        if (root) {
          // Merge with any subscription events that arrived during the
          // in-flight query (e.g. the user's own reply or a fast cross-user
          // reply). Overwriting would drop them.
          const page = threadRepliesConnection(root);
          const replies = page?.events ?? [];
          this.replaceMergingExisting([root, ...replies]);
          this.sortThreadEvents();
          this.oldestCursor = page?.startCursor ?? undefined;
          this.newestCursor = page?.endCursor ?? undefined;
          this.hasReachedStart = !(page?.hasOlder ?? false);
        }
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('ThreadMessagesStore: fetch failed:', error);
        this.isInitialLoading = false;
      });
  }

  private catchUpForward(thisLoad: number): void {
    if (!this.newestCursor) {
      this.fetchThread(thisLoad);
      return;
    }

    const after = this.newestCursor;
    this.client
      .query(ThreadEventsQuery, {
        roomId: this.roomId,
        threadRootEventId: this.threadRootEventId,
        limit: PAGE_SIZE,
        after
      })
      .toPromise()
      .then((result) => {
        if (this.isStale(thisLoad)) return;
        if (result.error) {
          console.error('ThreadMessagesStore: catchUp error:', result.error);
          return;
        }

        const page = threadRepliesConnection(result.data?.room?.event);
        if (!page) return;

        const newerReplies = unmask(page.events);
        if (page.endCursor) {
          this.newestCursor = page.endCursor;
        }

        this.appendMany(newerReplies);
        this.sortThreadEvents();

        if (page.hasNewer) {
          this.fetchThread(thisLoad);
        }
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('ThreadMessagesStore: catchUp failed:', error);
      });
  }

  private sortThreadEvents(): void {
    this.events = [...this.events].sort((a, b) => {
      if (a.id === this.threadRootEventId) return -1;
      if (b.id === this.threadRootEventId) return 1;

      const aTime = Date.parse(a.createdAt);
      const bTime = Date.parse(b.createdAt);
      if (aTime !== bTime) return aTime - bTime;
      return a.id.localeCompare(b.id);
    });
  }
}
