import { describe, expect, it } from 'vitest';
import type { RoomEventView } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';
import {
  beginOptimisticThreadFollow,
  clearOptimisticThreadFollowForEvent
} from './optimisticThreadFollow';

function messageEvent(id: string, isFollowing: boolean | null): RoomEventView {
  return {
    id,
    createdAt: '2026-05-27T00:00:00Z',
    actorId: 'u1',
    actor: null,
    event: {
      kind: RoomEventKind.MessagePosted,
      roomId: 'room-1',
      body: id,
      attachments: [],
      linkPreview: null,
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId: null,
      echoOfEventId: null,
      echoFromThreadRootEventId: null,
      channelEchoEventId: null,
      replyCount: 1,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: isFollowing,
      reactions: []
    }
  };
}

function followState(event: RoomEventView): boolean | null | undefined {
  if (event.event?.kind !== RoomEventKind.MessagePosted) throw new Error('expected message');
  return event.event.viewerIsFollowingThread;
}

function begin(input: {
  events: RoomEventView[];
  registry?: OptimisticMutationRegistry;
  threadRootEventId?: string;
  isFollowing: boolean;
}) {
  const registry = input.registry ?? new OptimisticMutationRegistry();
  return beginOptimisticThreadFollow({
    threadRootEventId: input.threadRootEventId ?? 'root',
    isFollowing: input.isFollowing,
    getEvents: () => input.events,
    registry,
    setEvent: (eventId, event) => {
      const index = input.events.findIndex((candidate) => candidate.id === eventId);
      if (index !== -1) input.events[index] = event;
    }
  });
}

describe('optimistic thread follow', () => {
  it('applies and rolls back only viewer follow state', () => {
    const events = [messageEvent('root', false)];

    const optimistic = begin({ events, isFollowing: true });

    expect(followState(events[0])).toBe(true);

    const event = events[0].event;
    if (event?.kind !== RoomEventKind.MessagePosted) throw new Error('expected message');
    events[0] = {
      ...events[0],
      event: {
        ...event,
        replyCount: 3
      }
    };

    optimistic.rollback();

    expect(followState(events[0])).toBe(false);
    expect(events[0].event).toMatchObject({ replyCount: 3 });
  });

  it('resolves the root by ID after the events array is replaced', () => {
    const input = {
      events: [messageEvent('root', false), messageEvent('other', null)],
      isFollowing: true
    };
    const optimistic = begin(input);

    input.events = [messageEvent('new', null), ...input.events];
    optimistic.rollback();

    expect(input.events.map((event) => event.id)).toEqual(['new', 'root', 'other']);
    expect(followState(input.events[1])).toBe(false);
    expect(followState(input.events[0])).toBeNull();
  });

  it('keeps independent rapid toggles from rolling each other back', () => {
    const events = [messageEvent('root', false)];
    const registry = new OptimisticMutationRegistry();

    const first = begin({ events, registry, isFollowing: true });
    begin({ events, registry, isFollowing: false });

    first.rollback();

    expect(followState(events[0])).toBe(false);
  });

  it('clears stale rollback when an authoritative row arrives', () => {
    const events = [messageEvent('root', false)];
    const registry = new OptimisticMutationRegistry();

    const optimistic = begin({ events, registry, isFollowing: true });
    clearOptimisticThreadFollowForEvent(registry, 'root');
    events[0] = messageEvent('root', true);
    optimistic.rollback();

    expect(followState(events[0])).toBe(true);
  });
});
