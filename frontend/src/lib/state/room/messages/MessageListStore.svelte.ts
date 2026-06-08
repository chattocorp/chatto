import { tick } from 'svelte';
import { on } from 'svelte/events';
import { SvelteSet } from 'svelte/reactivity';
import type { Client } from '@urql/svelte';
import { useFragment } from '$lib/gql';
import {
  RoomEventViewFragmentDoc,
  type RoomEventViewFragment
} from '$lib/gql/graphql';
import type { EventEnvelope } from '$lib/eventBus.svelte';
import type { GraphQLClient } from '$lib/state/server/graphqlClient.svelte';
import { RefetchOneQuery } from './queries';
import { type EventConnectionPage, type RawEvent, unmask } from './helpers';

/**
 * Minimum hidden duration before a visibility→visible transition counts as
 * a "tab resume" worth catching up for. Mirrors the eventBus visibility-
 * resubscribe threshold and the GraphQL client's suspend-detector window
 * so all three layers react on the same horizon.
 */
const TAB_RESUME_GAP_MS = 30_000;

/**
 * Reactive store for a list of room events. The base class owns:
 *   - the events buffer and dedup set
 *   - list mutation primitives (add, prepend, replace, reset)
 *   - per-event refetch by id or messageEventId (reactions, edits, deletes,
 *     video processing all funnel through here)
 *   - the {@link ingestServerEvent} skeleton, which routes incoming
 *     subscription events to refetches or to subclass hooks
 *   - **its own lifecycle wiring**: a reconnect listener on
 *     `gqlClient.reconnectCount` and a tab-resume-after-gap listener on
 *     `visibilitychange`, both feeding into {@link catchUp}
 *
 * Subclasses fill in:
 *   - the initial query (room: paginated; thread: single fetch)
 *   - the visible-events filter for {@link refetchAll}
 *   - {@link onMessagePosted} — what to do with a new MessagePostedEvent
 *   - {@link onSystemEvent} — what to do with room system events (default ignore)
 *   - {@link catchUp} — silent refetch of newer events triggered by reconnect
 *     or tab resume
 *
 * The component owns the actual subscription (via `useEvent`) and
 * forwards events here. Cross-cutting side effects (e.g. cancelling an
 * in-progress edit, removing a typing indicator) stay in the component.
 *
 * **Disposal:** callers MUST call {@link dispose} on unmount to tear down
 * the reconnect and visibility listeners.
 */
export abstract class MessageListStore {
  events = $state<RoomEventViewFragment[]>([]);
  isInitialLoading = $state(true);
  isLoadingMore = $state(false);
  hasReachedStart = $state(false);

  protected readonly client: Client;
  protected seenIds: SvelteSet<string> = new SvelteSet<string>();
  protected roomId = '';
  protected oldestCursor: string | undefined;
  protected newestCursor: string | undefined;

  /** Increments on every load kickoff. Async callbacks compare against
   *  it via {@link isStale} to discard results from superseded loads. */
  #loadId = 0;

  #disposeLifecycle: (() => void) | null = null;

  /** Allocate a new load id; pair with {@link isStale} in async callbacks. */
  protected startLoad(): number {
    return ++this.#loadId;
  }

  /** True if a newer load has started — caller should discard its result. */
  protected isStale(thisLoad: number): boolean {
    return this.#loadId !== thisLoad;
  }

  constructor(
    protected readonly gqlClient: GraphQLClient,
    protected readonly getCurrentUserId: () => string | null
  ) {
    this.client = gqlClient.client;
    this.#disposeLifecycle = $effect.root(() => {
      // Reactive: re-run when reconnectCount changes, fire catchUp on
      // genuine increments.
      let lastSeen = this.gqlClient.reconnectCount;
      $effect(() => {
        const n = this.gqlClient.reconnectCount;
        if (n <= lastSeen) return;
        const prev = lastSeen;
        lastSeen = n;
        console.debug(
          '[MessageListStore] reconnectCount %d → %d, catching up',
          prev,
          n
        );
        this.catchUp();
      });

      // Non-reactive: register a document visibilitychange listener and
      // let $effect.root tear it down via the returned cleanup.
      if (typeof document === 'undefined') return;
      let lastVisibleAt = Date.now();
      return on(document, 'visibilitychange', () => {
        if (document.visibilityState !== 'visible') {
          lastVisibleAt = Date.now();
          return;
        }
        const gap = Date.now() - lastVisibleAt;
        lastVisibleAt = Date.now();
        if (gap > TAB_RESUME_GAP_MS) {
          console.debug(
            '[MessageListStore] visible after %ds hidden → catching up',
            Math.round(gap / 1000)
          );
          this.catchUp();
        }
      });
    });
  }

  /** Tear down lifecycle listeners. Idempotent. */
  dispose(): void {
    this.#disposeLifecycle?.();
    this.#disposeLifecycle = null;
  }

  /**
   * Silent refetch of newer events. Called by the reconnect / tab-resume
   * listeners wired in the constructor (the trigger reason is logged by
   * the base class before this is invoked). Subclasses implement the
   * actual fetch strategy. Must NOT toggle {@link isInitialLoading}.
   */
  protected abstract catchUp(): void;

  /**
   * Route a space event into the store. Handles all common message-list
   * mutations: inline edit/retract, refetch on reaction/video, full reset on
   * RoomDeletedEvent, full refetch on ServerMemberDeletedEvent. Delegates
   * MessagePostedEvent and room system events to subclass hooks.
   */
  ingestServerEvent(serverEvent: EventEnvelope): void {
    const eventData = serverEvent.event;
    if (!eventData) return;
    // Subscription and historical-query payloads share the same Event
    // envelope. Cast once at the room boundary so downstream code can keep
    // using the RoomEventViewFragment shape it renders with.
    const spaceEvent = serverEvent as unknown as RoomEventViewFragment;

    if (eventData.__typename === 'ServerMemberDeletedEvent') {
      this.refetchAll();
      return;
    }

    if (eventData.__typename === 'RoomDeletedEvent') {
      if (eventData.roomId === this.roomId) this.resetState();
      return;
    }

    // From here on, only events scoped to this room are interesting.
    const eventRoomId =
      'roomId' in eventData
        ? eventData.roomId
        : 'processingRoomId' in eventData
          ? eventData.processingRoomId
          : null;
    if (eventRoomId != null && eventRoomId !== this.roomId) return;

    if (eventData.__typename === 'MessageRetractedEvent') {
      this.applyDeletion(eventData.messageEventId);
      return;
    }

    if (eventData.__typename === 'MessageEditedEvent') {
      this.applyEdit(eventData.messageEventId, eventData);
      return;
    }

    if (
      eventData.__typename === 'ReactionAddedEvent' ||
      eventData.__typename === 'ReactionRemovedEvent'
    ) {
      this.refetchByMessageEventId(eventData.messageEventId);
      return;
    }

    if (
      eventData.__typename === 'VideoProcessingCompletedEvent' ||
      eventData.__typename === 'AssetProcessingStartedEvent' ||
      eventData.__typename === 'AssetProcessingSucceededEvent' ||
      eventData.__typename === 'AssetProcessingFailedEvent'
    ) {
      if (!eventData.processingMessageEventId) return;
      this.refetchByMessageEventId(eventData.processingMessageEventId);
      return;
    }

    if (eventData.__typename === 'MessagePostedEvent') {
      this.onMessagePosted(spaceEvent, eventData);
      return;
    }

    if (
      eventData.__typename === 'UserJoinedRoomEvent' ||
      eventData.__typename === 'UserLeftRoomEvent' ||
      eventData.__typename === 'RoomUpdatedEvent' ||
      eventData.__typename === 'RoomArchivedEvent' ||
      eventData.__typename === 'RoomUnarchivedEvent'
    ) {
      this.onSystemEvent(spaceEvent);
    }
  }

  /** Refetch every visible event. Used when a space member is deleted. */
  abstract refetchAll(): Promise<void>;

  async loadMore(): Promise<void> {
    if (this.isLoadingMore || this.hasReachedStart || !this.oldestCursor) return;

    const before = this.oldestCursor;
    this.isLoadingMore = true;

    try {
      const page = await this.fetchOlderPage(before);
      if (!page) return;

      const olderEvents = unmask(page.events);
      if (olderEvents.length === 0) {
        this.hasReachedStart = true;
      } else {
        if (page.startCursor) {
          this.oldestCursor = page.startCursor;
        }
        const added = this.prependEvents(olderEvents);
        this.afterOlderPagePrepended();
        if (added === 0) this.hasReachedStart = true;
      }

      if (!page.hasOlder) this.hasReachedStart = true;
    } catch (error) {
      console.error(`${this.constructor.name}: loadMore failed:`, error);
    } finally {
      // Yield a frame so the virtualizer can settle before another loadMore.
      await tick();
      await new Promise((r) => requestAnimationFrame(r));
      this.isLoadingMore = false;
    }
  }

  protected abstract fetchOlderPage(before: string): Promise<EventConnectionPage | null>;

  protected afterOlderPagePrepended(): void {
    // Subclasses can normalize ordering after older events are prepended.
  }

  protected abstract onMessagePosted(
    spaceEvent: RoomEventViewFragment,
    eventData: Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>
  ): void;

  /** Default: ignore room system events. RoomMessagesStore overrides to add. */
  protected onSystemEvent(_spaceEvent: RoomEventViewFragment): void {
    // intentionally empty
  }

  protected async refetchOne(eventId: string): Promise<void> {
    const result = await this.client
      .query(
        RefetchOneQuery,
        { roomId: this.roomId, eventId },
        { requestPolicy: 'network-only' }
      )
      .toPromise();

    const fetched = result.data?.room?.event;
    if (!fetched) return;
    const updated = useFragment(RoomEventViewFragmentDoc, fetched);
    if (!updated) return;
    const idx = this.events.findIndex((e) => e.id === eventId);
    if (idx !== -1) this.events[idx] = updated;
  }

  protected async refetchByMessageEventId(messageEventId: string): Promise<void> {
    // Match either the direct event id or an echo whose original points here.
    for (const e of this.events) {
      const evt = e.event;
      if (
        e.id === messageEventId ||
        (evt?.__typename === 'MessagePostedEvent' && evt.echoOfEventId === messageEventId)
      ) {
        await this.refetchOne(e.id);
      }
    }
  }

  /**
   * Apply a deletion locally. Direct echo retractions hide only the echo
   * artifact; original-message retractions tombstone the original and any
   * visible echoes that point at it.
   * Reactions and reply metadata are left intact so the tombstone row keeps
   * its existing engagement visible alongside the placeholder.
   */
  protected applyDeletion(messageEventId: string): void {
    const targetIndex = this.events.findIndex((e) => e.id === messageEventId);
    const target = targetIndex === -1 ? null : this.events[targetIndex];
    const targetPayload = target?.event;
    if (
      targetPayload?.__typename === 'MessagePostedEvent' &&
      targetPayload.echoOfEventId
    ) {
      this.events.splice(targetIndex, 1);
      return;
    }

    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (evt?.__typename !== 'MessagePostedEvent') continue;
      if (e.id !== messageEventId && evt.echoOfEventId !== messageEventId) continue;

      this.events[i] = {
        ...e,
        event: { ...evt, body: null, attachments: [] }
      };
    }
  }

  /**
   * Apply an edit payload directly to the matching MessagePostedEvent. The
   * backend emits one canonical edit event per linked post/echo, so we only
   * patch the direct event ID here; the linked event will arrive separately.
   */
  protected applyEdit(
    messageEventId: string,
    edit: Extract<EventEnvelope['event'], { __typename: 'MessageEditedEvent' }>
  ): void {
    for (let i = 0; i < this.events.length; i++) {
      const e = this.events[i];
      const evt = e.event;
      if (evt?.__typename !== 'MessagePostedEvent') continue;
      if (e.id !== messageEventId) continue;

      this.events[i] = {
        ...e,
        event: {
          ...evt,
          body: edit.body,
          attachments: edit.attachments,
          linkPreview: edit.linkPreview,
          updatedAt: edit.updatedAt
        }
      };
    }
  }

  protected addEvent(event: RoomEventViewFragment): boolean {
    if (this.seenIds.has(event.id)) return false;
    this.seenIds.add(event.id);
    this.events.push(event);
    return true;
  }

  protected appendMany(events: RoomEventViewFragment[]): void {
    for (const e of events) this.addEvent(e);
  }

  protected prependEvents(olderEvents: RoomEventViewFragment[]): number {
    const newOnes = olderEvents.filter((e) => !this.seenIds.has(e.id));
    for (const e of newOnes) this.seenIds.add(e.id);
    this.events.unshift(...newOnes);
    return newOnes.length;
  }

  /**
   * Replace the buffer with fetched events but preserve any subscription
   * events that arrived during the in-flight query. Always the right
   * choice when a paginated query result replaces the timeline — the
   * eventBus subscription has been live since layout mount, so any
   * MessagePostedEvent for this room that lands while the query is in
   * flight has already been added to {@link events} via
   * {@link ingestServerEvent} and must not be wiped by the result.
   */
  protected replaceMergingExisting(rawEvents: readonly RawEvent[]): void {
    const fetched = unmask(rawEvents);
    const newSeen = new SvelteSet<string>();
    const merged: RoomEventViewFragment[] = [];
    for (const e of fetched) {
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      merged.push(e);
    }
    for (const e of this.events) {
      if (newSeen.has(e.id)) continue;
      newSeen.add(e.id);
      merged.push(e);
    }
    this.events = merged;
    this.seenIds = newSeen;
  }

  protected resetState(): void {
    this.events = [];
    this.seenIds = new SvelteSet();
    this.oldestCursor = undefined;
    this.newestCursor = undefined;
    this.hasReachedStart = false;
    this.isLoadingMore = false;
  }

  protected replaceWithFetchedAndUpdateCursors(connection: {
    events: readonly RawEvent[];
    startCursor?: string | null;
    endCursor?: string | null;
  }): void {
    this.replaceMergingExisting(connection.events);
    this.oldestCursor = connection.startCursor ?? undefined;
    this.newestCursor = connection.endCursor ?? undefined;
    this.hasReachedStart = false;
  }
}
