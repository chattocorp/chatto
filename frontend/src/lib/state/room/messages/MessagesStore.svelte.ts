import { SvelteSet } from 'svelte/reactivity';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { JumpToMessageState } from '../composerContext.svelte';
import { MessageListStore } from './MessageListStore.svelte';
import {
  PAGE_SIZE,
  RoomAfterQuery,
  RoomAroundQuery,
  RoomBeforeQuery,
  RoomLatestQuery,
  ThreadEventsQuery
} from './queries';
import { isRootRoomEvent, isThreadEvent } from './filters';
import {
  type EventConnectionPage,
  getActorId,
  threadRepliesConnection,
  unmask
} from './helpers';

type MessageScope = 'room' | 'thread';

/**
 * Message store for both the main room timeline and a single thread pane.
 * The scope-specific methods (`setRoom` / `setThread`) choose which Core
 * GraphQL connection backs the list while the lifecycle, pagination, refetch,
 * and subscription ingestion behavior stays shared.
 */
export class MessagesStore extends MessageListStore {
  private scope: MessageScope | null = null;
  private threadRootEventId = '';

  /** Root-level events only (excludes thread replies). */
  get rootEvents(): RoomEventViewFragment[] {
    return this.events.filter(isRootRoomEvent);
  }

  /** Events that belong to this thread (root + replies). */
  get threadEvents(): RoomEventViewFragment[] {
    return this.events.filter((e) => isThreadEvent(e, this.roomId, this.threadRootEventId));
  }

  setRoom(roomId: string): void {
    this.scope = 'room';
    this.roomId = roomId;
    this.threadRootEventId = '';
    this.resetAndFetchLatest();
  }

  setThread(roomId: string, threadRootEventId: string): void {
    this.scope = 'thread';
    this.roomId = roomId;
    this.threadRootEventId = threadRootEventId;

    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchThread(thisLoad);
  }

  protected catchUp(): void {
    if (!this.scope || !this.roomId) return;
    if (this.scope === 'thread' && !this.threadRootEventId) return;

    const thisLoad = this.startLoad();
    if (this.events.length === 0) {
      this.fetchInitial(thisLoad);
    } else {
      this.catchUpForward(thisLoad);
    }
  }

  async refetchAll(): Promise<void> {
    const snapshot = this.scope === 'thread' ? [...this.threadEvents] : [...this.rootEvents];
    for (const event of snapshot) {
      await this.refetchOne(event.id);
    }
  }

  protected async fetchOlderPage(before: string): Promise<EventConnectionPage | null> {
    if (this.scope === 'thread') {
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

    const result = await this.client
      .query(RoomBeforeQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE,
        before
      })
      .toPromise();

    return result.data?.room?.events ?? null;
  }

  protected afterOlderPagePrepended(): void {
    if (this.scope === 'thread') {
      this.sortThreadEvents();
    }
  }

  async loadNewer(jumpState: JumpToMessageState): Promise<void> {
    if (this.scope !== 'room') return;
    if (jumpState.isLoadingNewer || jumpState.hasReachedEnd) return;
    if (!this.newestCursor) return;

    jumpState.isLoadingNewer = true;
    try {
      const result = await this.client
        .query(RoomAfterQuery, {
          roomId: this.roomId,
          limit: PAGE_SIZE,
          after: this.newestCursor
        })
        .toPromise();

      // User left jumped mode while in flight — abandon the result.
      if (!jumpState.isJumpedMode) return;

      const page = result.data?.room?.events;
      if (!page) return;

      const newer = unmask(page.events);
      if (newer.length === 0) {
        jumpState.hasReachedEnd = true;
      } else {
        if (page.endCursor) {
          this.newestCursor = page.endCursor;
        }
        this.appendMany(newer);
      }

      if (!page.hasNewer) jumpState.hasReachedEnd = true;
    } catch (error) {
      console.error('MessagesStore: loadNewer failed:', error);
    } finally {
      jumpState.isLoadingNewer = false;
    }
  }

  async jumpToMessage(eventId: string, jumpState: JumpToMessageState): Promise<void> {
    if (this.scope !== 'room') return;
    if (this.events.some((e) => e.id === eventId)) {
      jumpState.scrollToEventId = eventId;
      return;
    }

    this.isInitialLoading = true;
    try {
      const result = await this.client
        .query(RoomAroundQuery, {
          roomId: this.roomId,
          eventId,
          limit: PAGE_SIZE
        })
        .toPromise();

      const around = result.data?.room?.eventsAround;
      if (result.error || !around) {
        if (result.error) console.error('MessagesStore: jumpToMessage failed:', result.error);
        return;
      }

      const { events: rawEvents, hasOlder, hasNewer, startCursor, endCursor } = around;
      const parsed = unmask(rawEvents);

      this.events = [...parsed];
      this.seenIds = new SvelteSet(parsed.map((e) => e.id));
      this.oldestCursor = startCursor ?? undefined;
      this.newestCursor = endCursor ?? undefined;
      this.hasReachedStart = !hasOlder;

      // Only enter jumped mode when newer messages exist beyond this window.
      jumpState.isJumpedMode = hasNewer;
      jumpState.hasReachedEnd = !hasNewer;
      jumpState.hasOlderMessages = hasOlder;
      jumpState.scrollToEventId = eventId;
    } finally {
      this.isInitialLoading = false;
    }
  }

  jumpToPresent(jumpState: JumpToMessageState): void {
    if (this.scope !== 'room') return;
    jumpState.reset();
    this.resetAndFetchLatest();
  }

  protected onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
    if (this.scope === 'thread') {
      if (eventData.threadRootEventId === this.threadRootEventId) {
        this.addEvent(spaceEvent);
        this.sortThreadEvents();
      }
      return;
    }

    // Thread replies don't enter the room timeline; instead, update
    // metadata on the root message (replyCount, lastReplyAt, participants,
    // viewerIsFollowingThread auto-follow).
    if (eventData.threadRootEventId) {
      this.applyThreadReplyToRoot(spaceEvent, eventData);
      return;
    }
    this.addEvent(spaceEvent);
  }

  protected onSystemEvent(spaceEvent: RoomEventViewFragment): void {
    if (this.scope === 'room') {
      this.addEvent(spaceEvent);
    }
  }

  private resetAndFetchLatest(): void {
    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchLatest(thisLoad);
  }

  private fetchInitial(thisLoad: number): void {
    if (this.scope === 'thread') {
      this.fetchThread(thisLoad);
    } else {
      this.fetchLatest(thisLoad);
    }
  }

  private fetchLatest(thisLoad: number): void {
    this.client
      .query(RoomLatestQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE
      })
      .toPromise()
      .then((result) => {
        if (this.isStale(thisLoad)) return;
        if (result.error) console.error('MessagesStore: fetchLatest error:', result.error);
        const page = result.data?.room?.events;
        if (page) {
          this.replaceWithFetchedAndUpdateCursors(page);
          this.hasReachedStart = !page.hasOlder;
        }
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('MessagesStore: fetchLatest failed:', error);
        this.isInitialLoading = false;
      });
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
        if (result.error) console.error('MessagesStore: fetchThread error:', result.error);
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
        console.error('MessagesStore: fetchThread failed:', error);
        this.isInitialLoading = false;
      });
  }

  private catchUpForward(thisLoad: number): void {
    if (!this.newestCursor) {
      this.fetchInitial(thisLoad);
      return;
    }

    if (this.scope === 'thread') {
      this.catchUpThreadForward(thisLoad, this.newestCursor);
    } else {
      this.catchUpRoomForward(thisLoad, this.newestCursor);
    }
  }

  private catchUpRoomForward(thisLoad: number, after: string): void {
    this.client
      .query(RoomAfterQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE,
        after
      })
      .toPromise()
      .then((result) => {
        if (this.isStale(thisLoad)) return;
        if (result.error) {
          console.error('MessagesStore: room catchUp error:', result.error);
          return;
        }
        const page = result.data?.room?.events;
        if (!page) return;

        const fetched = unmask(page.events);
        const strategy = page.hasNewer ? 'replace' : 'append';
        console.debug(
          '[MessagesStore] catchUpForward: roomId=%s after=%s fetched=%d hasNewer=%s strategy=%s',
          this.roomId,
          after,
          fetched.length,
          page.hasNewer,
          strategy
        );
        if (page.hasNewer) {
          this.replaceWithFetchedAndUpdateCursors(page);
        } else {
          if (page.endCursor) {
            this.newestCursor = page.endCursor;
          }
          this.appendMany(fetched);
        }
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('MessagesStore: room catchUp failed:', error);
      });
  }

  private catchUpThreadForward(thisLoad: number, after: string): void {
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
          console.error('MessagesStore: thread catchUp error:', result.error);
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
        console.error('MessagesStore: thread catchUp failed:', error);
      });
  }

  /**
   * Mirror the backend's auto-follow behavior on the root message when a
   * thread reply arrives, so the UI updates instantly without refetching.
   */
  private applyThreadReplyToRoot(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
    const rootIdx = this.events.findIndex((e) => e.id === eventData.threadRootEventId);
    if (rootIdx === -1) return;

    const rootEvent = this.events[rootIdx];
    if (rootEvent.event?.__typename !== 'MessagePostedEvent') return;

    const actorId = getActorId(spaceEvent.actor);
    const existingParticipants = rootEvent.event.threadParticipants;
    const isNewParticipant =
      !!actorId && !existingParticipants.some((p) => getActorId(p) === actorId);

    const isFirstReply = rootEvent.event.replyCount === 0;
    const currentUserId = this.getCurrentUserId();
    const viewerIsRootAuthor = currentUserId !== null && rootEvent.actorId === currentUserId;
    const viewerIsReplier = currentUserId !== null && actorId === currentUserId;
    const viewerIsFollowingThread =
      viewerIsReplier || (isFirstReply && viewerIsRootAuthor)
        ? true
        : rootEvent.event.viewerIsFollowingThread;

    this.events[rootIdx] = {
      ...rootEvent,
      event: {
        ...rootEvent.event,
        replyCount: rootEvent.event.replyCount + 1,
        lastReplyAt: spaceEvent.createdAt,
        viewerIsFollowingThread,
        threadParticipants:
          isNewParticipant && spaceEvent.actor
            ? [...existingParticipants, spaceEvent.actor]
            : existingParticipants
      }
    };
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
