import { SvelteSet } from 'svelte/reactivity';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { JumpToMessageState } from '../composerContext.svelte';
import { MessageListStore } from './MessageListStore.svelte';
import {
  PAGE_SIZE,
  RoomAfterQuery,
  RoomAroundQuery,
  RoomBeforeQuery,
  RoomLatestQuery
} from './queries';
import { isRootRoomEvent } from './filters';
import { type EventConnectionPage, getActorId, unmask } from './helpers';

/**
 * Message store for a room's main timeline. Adds pagination, jumped-mode
 * navigation (jump-to-message + load-newer + jump-to-present), reconnect
 * catch-up, root-event filtering, and thread-reply metadata fan-out.
 */
export class RoomMessagesStore extends MessageListStore {
  /** Root-level events only (excludes thread replies). */
  get rootEvents(): RoomEventViewFragment[] {
    return this.events.filter(isRootRoomEvent);
  }

  /**
   * Switch to a room (or force-refetch the current one). Always shows the
   * skeleton and clears state. Silent reconnect / tab-resume catch-ups go
   * through {@link catchUp} (driven internally by the base class), not
   * through this method.
   */
  setRoom(roomId: string): void {
    this.roomId = roomId;
    this.resetAndFetchLatest();
  }

  protected catchUp(): void {
    if (!this.roomId) return;
    const thisLoad = this.startLoad();
    if (this.events.length === 0) {
      this.fetchLatest(thisLoad);
    } else {
      this.catchUpForward(thisLoad);
    }
  }

  /** Shared by {@link setRoom} and {@link jumpToPresent}: clear state, show
   *  the skeleton, kick off a fresh fetchLatest under a new load id. */
  private resetAndFetchLatest(): void {
    const thisLoad = this.startLoad();
    this.resetState();
    this.isInitialLoading = true;
    this.fetchLatest(thisLoad);
  }

  protected async fetchOlderPage(before: string): Promise<EventConnectionPage | null> {
    const result = await this.client
      .query(RoomBeforeQuery, {
        roomId: this.roomId,
        limit: PAGE_SIZE,
        before
      })
      .toPromise();

    return result.data?.room?.events ?? null;
  }

  /**
   * Forward pagination — only meaningful in jumped mode (i.e. when the local
   * timeline doesn't include the latest events). Updates {@link jumpState} to
   * reflect end-of-history.
   */
  async loadNewer(jumpState: JumpToMessageState): Promise<void> {
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
      console.error('RoomMessagesStore: loadNewer failed:', error);
    } finally {
      jumpState.isLoadingNewer = false;
    }
  }

  async jumpToMessage(eventId: string, jumpState: JumpToMessageState): Promise<void> {
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
        if (result.error) console.error('RoomMessagesStore: jumpToMessage failed:', result.error);
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

  /** Exit jumped mode and refetch the latest events. */
  jumpToPresent(jumpState: JumpToMessageState): void {
    jumpState.reset();
    this.resetAndFetchLatest();
  }

  async refetchAll(): Promise<void> {
    const snapshot = [...this.rootEvents];
    for (const event of snapshot) {
      await this.refetchOne(event.id);
    }
  }

  protected onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void {
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
    this.addEvent(spaceEvent);
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
        if (result.error) console.error('RoomMessagesStore: fetchLatest error:', result.error);
        const page = result.data?.room?.events;
        if (page) {
          this.replaceWithFetchedAndUpdateCursors(page);
          this.hasReachedStart = !page.hasOlder;
        }
        this.isInitialLoading = false;
      })
      .catch((error: unknown) => {
        if (this.isStale(thisLoad)) return;
        console.error('RoomMessagesStore: fetchLatest failed:', error);
        this.isInitialLoading = false;
      });
  }

  /**
   * Reconnect catch-up: fetch only events newer than what we already have.
   * If the gap is larger than a page (server reports hasNewer), replace the
   * timeline to avoid holes.
   *
   * Uses `newestCursor` (last cursor returned by a query) rather than
   * scanning local events for a max timestamp. Subscription-delivered events
   * arrived after `newestCursor` was set, so this re-fetches them — but
   * `appendMany` dedupes by ID so the cost is duplicate network bytes, not
   * duplicate UI items.
   */
  private catchUpForward(thisLoad: number): void {
    if (!this.newestCursor) {
      this.fetchLatest(thisLoad);
      return;
    }

    const after = this.newestCursor;
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
          console.error('RoomMessagesStore: catchUp error:', result.error);
          return;
        }
        const page = result.data?.room?.events;
        if (!page) return;

        const fetched = unmask(page.events);
        const strategy = page.hasNewer ? 'replace' : 'append';
        console.debug(
          '[RoomMessagesStore] catchUpForward: roomId=%s after=%s fetched=%d hasNewer=%s strategy=%s',
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
        console.error('RoomMessagesStore: catchUp failed:', error);
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
}
