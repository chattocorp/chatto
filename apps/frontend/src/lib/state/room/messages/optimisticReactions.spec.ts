import { describe, expect, it } from 'vitest';
import type { RoomEventView } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';
import { beginOptimisticReaction } from './optimisticReactions';

type ReactionSummary = Extract<
  NonNullable<RoomEventView['event']>,
  { kind: typeof RoomEventKind.MessagePosted }
>['reactions'][number];

function messageEvent(
  id: string,
  reactions: ReactionSummary[] = [],
  links: Partial<
    Pick<
      Extract<NonNullable<RoomEventView['event']>, { kind: typeof RoomEventKind.MessagePosted }>,
      'echoOfEventId' | 'echoFromThreadRootEventId' | 'channelEchoEventId'
    >
  > = {}
): RoomEventView {
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
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: null,
      reactions,
      ...links
    }
  };
}

function reaction(emoji: string, count: number, hasReacted: boolean): ReactionSummary {
  return { emoji, count, hasReacted, users: [] };
}

function reactionsOf(event: RoomEventView): ReactionSummary[] {
  if (event.event?.kind !== RoomEventKind.MessagePosted) throw new Error('expected message');
  return event.event.reactions;
}

function begin(input: {
  events?: RoomEventView[];
  previews?: Map<string, RoomEventView | null>;
  messageEventId: string;
  emoji: string;
  action: 'add' | 'remove';
}) {
  const events = input.events ?? [];
  const previews = input.previews ?? new Map<string, RoomEventView | null>();
  return beginOptimisticReaction({
    messageEventId: input.messageEventId,
    emoji: input.emoji,
    action: input.action,
    getEvents: () => events,
    previews,
    registry: new OptimisticMutationRegistry(),
    setEvent: (eventId, event) => {
      const index = events.findIndex((candidate) => candidate.id === eventId);
      if (index !== -1) events[index] = event;
    },
    setPreview: (key, event) => {
      previews.set(key, event);
    }
  });
}

describe('optimistic reactions', () => {
  it('applies and rolls back an add', () => {
    const events = [messageEvent('m1', [reaction('heart', 1, false)])];

    const optimistic = begin({
      events,
      messageEventId: 'm1',
      emoji: 'heart',
      action: 'add'
    });

    expect(reactionsOf(events[0])).toMatchObject([{ emoji: 'heart', count: 2, hasReacted: true }]);

    optimistic.rollback();

    expect(reactionsOf(events[0])).toMatchObject([{ emoji: 'heart', count: 1, hasReacted: false }]);
  });

  it('applies and rolls back a remove', () => {
    const events = [messageEvent('m1', [reaction('heart', 2, true)])];

    const optimistic = begin({
      events,
      messageEventId: 'm1',
      emoji: 'heart',
      action: 'remove'
    });

    expect(reactionsOf(events[0])).toMatchObject([{ emoji: 'heart', count: 1, hasReacted: false }]);

    optimistic.rollback();

    expect(reactionsOf(events[0])).toMatchObject([{ emoji: 'heart', count: 2, hasReacted: true }]);
  });

  it('tracks independent pending reactions per emoji', () => {
    const events = [messageEvent('m1')];
    const registry = new OptimisticMutationRegistry();
    const input = {
      getEvents: () => events,
      previews: new Map<string, RoomEventView | null>(),
      registry,
      setEvent: (eventId: string, event: RoomEventView) => {
        const index = events.findIndex((candidate) => candidate.id === eventId);
        if (index !== -1) events[index] = event;
      },
      setPreview: () => {}
    };

    const heart = beginOptimisticReaction({
      ...input,
      messageEventId: 'm1',
      emoji: 'heart',
      action: 'add'
    });
    beginOptimisticReaction({
      ...input,
      messageEventId: 'm1',
      emoji: 'thumbsup',
      action: 'add'
    });

    heart.rollback();

    expect(reactionsOf(events[0])).toMatchObject([
      { emoji: 'thumbsup', count: 1, hasReacted: true }
    ]);
  });

  it('reconciles the touched emoji from the RPC response', () => {
    const events = [messageEvent('m1', [reaction('heart', 1, false)])];

    const optimistic = begin({
      events,
      messageEventId: 'm1',
      emoji: 'heart',
      action: 'add'
    });
    optimistic.applyServerReaction({ emoji: 'heart', count: 5, hasReacted: true });
    optimistic.rollback();

    expect(reactionsOf(events[0])).toMatchObject([{ emoji: 'heart', count: 5, hasReacted: true }]);
  });

  it('resolves the target by ID after the events array is replaced', () => {
    let events = [messageEvent('m1', [reaction('heart', 1, false)]), messageEvent('m2')];
    const optimistic = beginOptimisticReaction({
      messageEventId: 'm1',
      emoji: 'heart',
      action: 'add',
      getEvents: () => events,
      previews: new Map(),
      registry: new OptimisticMutationRegistry(),
      setEvent: (eventId, event) => {
        const index = events.findIndex((candidate) => candidate.id === eventId);
        if (index !== -1) events[index] = event;
      },
      setPreview: () => {}
    });

    events = [messageEvent('new'), ...events];
    optimistic.applyServerReaction({ emoji: 'heart', count: 5, hasReacted: true });

    expect(events.map((event) => event.id)).toEqual(['new', 'm1', 'm2']);
    expect(reactionsOf(events[1])).toMatchObject([{ emoji: 'heart', count: 5, hasReacted: true }]);
    expect(reactionsOf(events[0])).toEqual([]);
  });

  it('rolls back only the touched reaction summary', () => {
    const events = [messageEvent('m1', [reaction('heart', 1, false)])];

    const optimistic = begin({
      events,
      messageEventId: 'm1',
      emoji: 'heart',
      action: 'add'
    });
    const event = events[0].event;
    if (event?.kind !== RoomEventKind.MessagePosted) throw new Error('expected message');
    events[0] = {
      ...events[0],
      event: {
        ...event,
        body: 'edited while reaction was pending',
        reactions: [...reactionsOf(events[0]), reaction('thumbsup', 1, true)]
      }
    };

    optimistic.rollback();

    expect(events[0].event).toMatchObject({ body: 'edited while reaction was pending' });
    expect(reactionsOf(events[0])).toMatchObject([
      { emoji: 'heart', count: 1, hasReacted: false },
      { emoji: 'thumbsup', count: 1, hasReacted: true }
    ]);
  });

  it('patches loaded original and echo messages before the original has an echo backlink', () => {
    const events = [
      messageEvent('reply'),
      messageEvent('echo', [], { echoOfEventId: 'reply', echoFromThreadRootEventId: 'root' })
    ];

    begin({
      events,
      messageEventId: 'echo',
      emoji: 'heart',
      action: 'add'
    });

    expect(reactionsOf(events[0])).toMatchObject([{ emoji: 'heart', count: 1, hasReacted: true }]);
    expect(reactionsOf(events[1])).toMatchObject([{ emoji: 'heart', count: 1, hasReacted: true }]);
  });

  it('patches and rolls back preview cache rows', () => {
    const previews = new Map<string, RoomEventView | null>([
      ['room-1\u0000preview', messageEvent('preview', [reaction('heart', 1, false)])]
    ]);

    const optimistic = begin({
      previews,
      messageEventId: 'preview',
      emoji: 'heart',
      action: 'add'
    });

    expect(reactionsOf(previews.get('room-1\u0000preview')!)).toMatchObject([
      { emoji: 'heart', count: 2, hasReacted: true }
    ]);

    optimistic.rollback();

    expect(reactionsOf(previews.get('room-1\u0000preview')!)).toMatchObject([
      { emoji: 'heart', count: 1, hasReacted: false }
    ]);
  });
});
