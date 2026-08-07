import type { EventConnectionPage, RoomTimelineAPI } from '$lib/api-client/roomTimeline';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import { isRootRoomEvent, isThreadEvent } from './filters';

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

  matches(scope: MessageTimelineScope, roomId: string, threadRootEventId = ''): boolean {
    return (
      this.scope === scope &&
      this.roomId === roomId &&
      (this.threadRootEventId ?? '') === threadRootEventId
    );
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

  sort(events: TimelineEventView[]): TimelineEventView[] {
    return events
      .map((event, index) => ({ event, index }))
      .sort((a, b) => {
        if (this.threadRootEventId !== null) {
          const aIsRoot = a.event.id === this.threadRootEventId;
          const bIsRoot = b.event.id === this.threadRootEventId;
          if (aIsRoot !== bIsRoot) return aIsRoot ? -1 : 1;
        }
        return Date.parse(a.event.createdAt) - Date.parse(b.event.createdAt) || a.index - b.index;
      })
      .map(({ event }) => event);
  }

  fetchPage(input: {
    limit: number;
    before?: string;
    after?: string;
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
    threadRootEventId: string | null = this.threadRootEventId
  ): Promise<EventConnectionPage> {
    return threadRootEventId !== null
      ? this.api.getThreadEventsAround({
          roomId: this.roomId,
          threadRootEventId,
          eventId,
          limit
        })
      : this.api.getRoomEventsAround({ roomId: this.roomId, eventId, limit });
  }

  isContinuityEvent(event: TimelineEventView): boolean {
    return this.scope === 'room' || event.id !== this.threadRootEventId;
  }
}
