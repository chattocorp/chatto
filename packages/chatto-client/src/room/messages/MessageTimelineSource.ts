import type { EventConnectionPage, RoomTimelineAPI } from '../../api/roomTimeline.js';
import type { TimelineEventView } from '../../timeline/timelineEvents.js';
import { isRootRoomEvent, isThreadEvent } from './filters.js';

export type MessageTimelineScope = 'room' | 'thread';

/** Immutable room/thread identity plus its timeline query and ordering policy. */
export class MessageTimelineSource {
  readonly scope: MessageTimelineScope;

  private constructor(
    private readonly api: RoomTimelineAPI,
    readonly roomId: string,
    readonly threadRootEventId: string | null
  ) {
    this.scope = threadRootEventId === null ? 'room' : 'thread';
  }

  static room(api: RoomTimelineAPI, roomId: string): MessageTimelineSource {
    return new MessageTimelineSource(api, roomId, null);
  }

  static thread(
    api: RoomTimelineAPI,
    roomId: string,
    threadRootEventId: string
  ): MessageTimelineSource {
    return new MessageTimelineSource(api, roomId, threadRootEventId);
  }

  rootEventsFrom(events: TimelineEventView[]): TimelineEventView[] {
    return events.filter(isRootRoomEvent);
  }

  threadEventsFrom(events: TimelineEventView[]): TimelineEventView[] {
    return events.filter((event) =>
      isThreadEvent(event, this.roomId, this.threadRootEventId ?? '')
    );
  }

  eventsFrom(events: TimelineEventView[]): TimelineEventView[] {
    return this.scope === 'room' ? this.rootEventsFrom(events) : this.threadEventsFrom(events);
  }

  /**
   * Parsed `createdAt` per event object. Event views are replaced, not
   * mutated, so an object's timestamp never changes. Entries go away with
   * their events.
   */
  readonly #times = new WeakMap<TimelineEventView, number>();

  #time(event: TimelineEventView): number {
    let time = this.#times.get(event);
    if (time === undefined) {
      time = Date.parse(event.createdAt);
      this.#times.set(event, time);
    }
    return time;
  }

  /** Display order: the thread root first, then by creation time. Unparseable times compare equal. */
  #compare(a: TimelineEventView, b: TimelineEventView): number {
    if (this.threadRootEventId !== null) {
      const aIsRoot = a.id === this.threadRootEventId;
      const bIsRoot = b.id === this.threadRootEventId;
      if (aIsRoot !== bIsRoot) return aIsRoot ? -1 : 1;
    }
    return this.#time(a) - this.#time(b) || 0;
  }

  /**
   * Order events for display. Equal events keep their input order. Returns
   * `events` itself when it is already in order, which is the usual case for
   * a live append, so callers must pass an array that they own.
   */
  sort(events: TimelineEventView[]): TimelineEventView[] {
    let ordered = true;
    for (let i = 1; i < events.length && ordered; i++) {
      ordered = this.#compare(events[i - 1]!, events[i]!) <= 0;
    }
    if (ordered) return events;
    return events.toSorted((a, b) => this.#compare(a, b));
  }

  fetchPage(input: {
    limit: number;
    before?: string;
    after?: string;
    minimumCursor?: string;
  }): Promise<EventConnectionPage> {
    return this.threadRootEventId !== null
      ? this.api.getThreadEvents({
          roomId: this.roomId,
          threadRootEventId: this.threadRootEventId,
          ...input
        })
      : this.api.getRoomEvents({ roomId: this.roomId, ...input });
  }

  fetchAround(
    eventId: string,
    limit: number,
    threadRootEventId: string | null = this.threadRootEventId,
    minimumCursor?: string
  ): Promise<EventConnectionPage> {
    return threadRootEventId !== null
      ? this.api.getThreadEventsAround({
          roomId: this.roomId,
          threadRootEventId,
          eventId,
          limit,
          minimumCursor
        })
      : this.api.getRoomEventsAround({ roomId: this.roomId, eventId, limit, minimumCursor });
  }

  isContinuityEvent(event: TimelineEventView): boolean {
    return this.scope === 'room' || event.id !== this.threadRootEventId;
  }
}
