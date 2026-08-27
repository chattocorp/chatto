import { describe, expect, it, vi } from 'vitest';
import type { EventConnectionPage, RoomTimelineAPI } from '$lib/api-client/roomTimeline';
import type { TimelineEventView } from '$lib/render/timelineEvents';
import { MessageTimelineSource } from './MessageTimelineSource';

const emptyPage: EventConnectionPage = {
  events: [],
  hasOlder: false,
  hasNewer: false
};

function timelineApi(): RoomTimelineAPI {
  return {
    getRoomEvents: vi.fn().mockResolvedValue(emptyPage),
    getRoomEventsAround: vi.fn().mockResolvedValue(emptyPage),
    getMessage: vi.fn().mockResolvedValue(null),
    getThreadEvents: vi.fn().mockResolvedValue(emptyPage),
    getThreadEventsAround: vi.fn().mockResolvedValue(emptyPage)
  };
}

function event(id: string, createdAt: string, threadRootEventId: string | null): TimelineEventView {
  return {
    id,
    createdAt,
    actorId: 'user-1',
    actor: null,
    event: {
      kind: 'messagePosted',
      roomId: 'room-1',
      threadRootEventId
    }
  } as unknown as TimelineEventView;
}

describe('MessageTimelineSource', () => {
  it('owns room identity, reads, filtering, and chronological ordering', async () => {
    const api = timelineApi();
    const source = MessageTimelineSource.room(api, 'room-1');
    const newer = event('newer', '2026-08-02T11:00:00Z', null);
    const older = event('older', '2026-08-02T10:00:00Z', null);
    const reply = event('reply', '2026-08-02T10:30:00Z', 'root');

    await source.fetchPage({ limit: 50, before: 'cursor' });
    await source.fetchAround('older', 25);

    expect(source.matches('room', 'room-1')).toBe(true);
    expect(source.eventsFrom([newer, reply, older])).toEqual([newer, older]);
    expect(source.sort([newer, older])).toEqual([older, newer]);
    expect(api.getRoomEvents).toHaveBeenCalledWith({
      roomId: 'room-1',
      limit: 50,
      before: 'cursor'
    });
    expect(api.getRoomEventsAround).toHaveBeenCalledWith({
      roomId: 'room-1',
      eventId: 'older',
      limit: 25
    });
  });

  it('owns thread identity, reads, filtering, and root-first ordering', async () => {
    const api = timelineApi();
    const source = MessageTimelineSource.thread(api, 'room-1', 'root');
    const root = event('root', '2026-08-02T11:00:00Z', null);
    const firstReply = event('first-reply', '2026-08-02T10:00:00Z', 'root');
    const otherReply = event('other-reply', '2026-08-02T09:00:00Z', 'other-root');

    await source.fetchPage({ limit: 50, after: 'cursor' });
    await source.fetchAround('first-reply', 25);

    expect(source.matches('thread', 'room-1', 'root')).toBe(true);
    expect(source.eventsFrom([firstReply, otherReply, root])).toEqual([firstReply, root]);
    expect(source.sort([firstReply, root])).toEqual([root, firstReply]);
    expect(api.getThreadEvents).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 'root',
      limit: 50,
      after: 'cursor'
    });
    expect(api.getThreadEventsAround).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 'root',
      eventId: 'first-reply',
      limit: 25
    });
  });

  it('can deliberately use the room lookup for an off-window thread preview', async () => {
    const api = timelineApi();
    const source = MessageTimelineSource.thread(api, 'room-1', 'root');

    await source.fetchAround('preview', 1, null);

    expect(api.getRoomEventsAround).toHaveBeenCalledWith({
      roomId: 'room-1',
      eventId: 'preview',
      limit: 1
    });
    expect(api.getThreadEventsAround).not.toHaveBeenCalled();
  });

  it('keeps APIs isolated between server-owned sources', async () => {
    const firstApi = timelineApi();
    const secondApi = timelineApi();
    const first = MessageTimelineSource.room(firstApi, 'room-1');
    const second = MessageTimelineSource.room(secondApi, 'room-1');

    await second.fetchPage({ limit: 50 });

    expect(first.matches('room', 'room-1')).toBe(true);
    expect(firstApi.getRoomEvents).not.toHaveBeenCalled();
    expect(secondApi.getRoomEvents).toHaveBeenCalledOnce();
  });
});
