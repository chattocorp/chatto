import { describe, expect, it, vi } from 'vitest';
import { MessagePostedEvent } from '@chatto/api-types/realtime/v1/events_pb';
import { RealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import { Timestamp } from '@bufbuild/protobuf';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import type { UserAvatarUserView } from '$lib/render/users';
import type { RoomStores } from './roomStores.svelte';
import { TimelineSync, type TimelineSyncOptions } from './timelineSync';

function deferred() {
  let resolve!: () => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<void>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function fakeTimeline() {
  const reads: ReturnType<typeof deferred>[] = [];
  return {
    reads,
    refreshAnchorForMessageMutation: vi.fn((): string | null => null),
    refreshCurrentWindow: vi.fn(
      (
        _anchorEventId: string | null,
        _forward: boolean,
        _minimumCursor: string | undefined,
        _isCurrent: () => boolean
      ) => {
        const read = deferred();
        reads.push(read);
        return read.promise;
      }
    ),
    applyMessageRetraction: vi.fn(),
    ingestEvent: vi.fn()
  };
}

type FakeTimeline = ReturnType<typeof fakeTimeline>;

function makeSync(
  rooms: Record<string, { messages?: FakeTimeline; threads?: Record<string, FakeTimeline> }>,
  options: Partial<TimelineSyncOptions> = {}
) {
  const entries = Object.entries(rooms).map(
    ([id, room]) =>
      [id, { threads: {}, ...room }] as [
        string,
        { messages?: FakeTimeline; threads: Record<string, FakeTimeline> }
      ]
  );
  const timelines = (roomId?: string) =>
    entries
      .filter(([id]) => roomId === undefined || id === roomId)
      .flatMap(([, room]) => [
        ...(room.messages ? [room.messages] : []),
        ...Object.values(room.threads)
      ]);
  const track = vi.fn();
  const sync = new TimelineSync({
    rooms: {
      entries: () => entries,
      loaded: (roomId: string) => entries.find(([id]) => id === roomId)?.[1],
      timelines
    } as unknown as RoomStores,
    readMessages: vi.fn(async () => []),
    generation: () => 1,
    eventCursor: () => 'cursor',
    track,
    actor: () => ({ user: null, deleted: false }),
    ...options
  });
  return { sync, track };
}

async function flush(): Promise<void> {
  for (let index = 0; index < 5; index++) await Promise.resolve();
}

describe('TimelineSync', () => {
  it('runs one window read at a time for each timeline and keeps distinct requests', async () => {
    const room = fakeTimeline();
    const { sync, track } = makeSync({ R1: { messages: room } });

    sync.refreshWindows('R1', 'E1');
    sync.refreshWindows('R1', 'E1');
    sync.refreshWindows('R1', 'E2');
    sync.refreshWindows('R1', 'E2');
    expect(room.refreshCurrentWindow).toHaveBeenCalledOnce();
    expect(track).toHaveBeenCalledOnce();

    room.reads[0].resolve();
    await flush();
    room.reads[1].resolve();
    await flush();

    expect(room.refreshCurrentWindow.mock.calls.map((call) => call[0])).toEqual(['E1', 'E1', 'E2']);
    expect(room.refreshCurrentWindow.mock.calls[0]).toEqual([
      'E1',
      false,
      'cursor',
      expect.any(Function)
    ]);
  });

  it('drops queued window reads from an older generation', async () => {
    let generation = 1;
    const room = fakeTimeline();
    const { sync } = makeSync({ R1: { messages: room } }, { generation: () => generation });

    sync.refreshWindows('R1', 'E1');
    sync.refreshWindows('R1', 'E2');
    const isCurrent = room.refreshCurrentWindow.mock.calls[0][3];
    generation = 2;
    expect(isCurrent()).toBe(false);
    room.reads[0].resolve();
    await flush();

    expect(room.refreshCurrentWindow).toHaveBeenCalledOnce();
  });

  it('does not let a queued read from an older generation hide an identical new one', async () => {
    let generation = 1;
    const room = fakeTimeline();
    const { sync } = makeSync({ R1: { messages: room } }, { generation: () => generation });

    sync.refreshWindows('R1', 'E1');
    sync.refreshWindows('R1', 'E2');
    generation = 2;
    sync.refreshWindows('R1', 'E2');
    room.reads[0].resolve();
    await flush();

    expect(room.refreshCurrentWindow.mock.calls.map((call) => call[0])).toEqual(['E1', 'E2']);
  });

  it('gives a failed window read to track with its generation', async () => {
    const room = fakeTimeline();
    const { sync, track } = makeSync({ R1: { messages: room } });

    sync.refreshWindows('R1', 'E1');
    const [read, generation] = track.mock.calls[0] as [Promise<unknown>, number];
    const error = new Error('offline');
    room.reads[0].reject(error);

    await expect(read).rejects.toBe(error);
    expect(generation).toBe(1);
  });

  it('reads room timelines forward on request and refreshes every room without a room ID', () => {
    const room = fakeTimeline();
    const thread = fakeTimeline();
    const other = fakeTimeline();
    const { sync } = makeSync({
      R1: { messages: room, threads: { T: thread } },
      R2: { messages: other }
    });

    sync.refreshWindows('', 'E1', true);

    expect(room.refreshCurrentWindow.mock.calls[0].slice(0, 2)).toEqual(['E1', true]);
    expect(thread.refreshCurrentWindow.mock.calls[0].slice(0, 2)).toEqual(['E1', false]);
    expect(other.refreshCurrentWindow).toHaveBeenCalledOnce();
  });

  it('removes a retracted message from every timeline when the event has no room', () => {
    const room = fakeTimeline();
    const other = fakeTimeline();
    const { sync } = makeSync({ R1: { messages: room }, R2: { messages: other } });

    sync.retract('', 'M1', '2026-09-27T12:00:00.000Z');

    for (const store of [room, other]) {
      expect(store.applyMessageRetraction).toHaveBeenCalledWith('M1', '2026-09-27T12:00:00.000Z');
    }
  });

  it('shows a post preview with the fields of the event and empty resource fields', () => {
    const room = fakeTimeline();
    const thread = fakeTimeline();
    const actor: UserAvatarUserView = {
      id: 'U1',
      login: 'ann',
      displayName: 'Ann',
      deleted: false,
      presenceStatus: PresenceStatus.ONLINE
    };
    const { sync } = makeSync(
      { R1: { messages: room, threads: { ROOT: thread } } },
      { actor: () => ({ user: actor, deleted: false }) }
    );

    sync.ingestPost(
      new RealtimeEvent({
        id: 'E1',
        actorId: 'U1',
        createdAt: Timestamp.fromDate(new Date('2026-09-27T12:00:00.000Z')),
        event: {
          case: 'messagePosted',
          value: new MessagePostedEvent({
            roomId: 'R1',
            bodyPlaintext: 'hello',
            inReplyTo: 'E0',
            threadRootEventId: 'ROOT',
            echoOfEventId: 'ECHO',
            echoFromThreadRootEventId: 'ECHO-ROOT'
          })
        }
      })
    );

    const expected: TimelineEventView = {
      id: 'E1',
      createdAt: '2026-09-27T12:00:00.000Z',
      actorId: 'U1',
      actor,
      actorResolution: undefined,
      event: {
        kind: TimelineEventKind.MessagePosted,
        roomId: 'R1',
        body: 'hello',
        attachments: [],
        linkPreview: null,
        reactions: [],
        updatedAt: null,
        inReplyTo: 'E0',
        threadRootEventId: 'ROOT',
        echoOfEventId: 'ECHO',
        echoFromThreadRootEventId: 'ECHO-ROOT',
        channelEchoEventId: null,
        deletedAt: null,
        pinned: false,
        threadExists: false,
        replyCount: 0,
        lastReplyAt: null,
        threadParticipantCount: 0,
        threadParticipants: [],
        viewerIsFollowingThread: null,
        viewerHasUnreadThread: null
      }
    };
    expect(room.ingestEvent).toHaveBeenCalledExactlyOnceWith(expected);
    expect(thread.ingestEvent).toHaveBeenCalledExactlyOnceWith(expected);
  });

  it.each([
    { deleted: true, resolution: 'deleted' },
    { deleted: false, resolution: 'loading' }
  ])('shows a post by an author without a profile as $resolution', ({ deleted, resolution }) => {
    const room = fakeTimeline();
    const { sync } = makeSync(
      { R1: { messages: room } },
      { actor: () => ({ user: null, deleted }) }
    );

    sync.ingestPost(
      new RealtimeEvent({
        id: 'E1',
        actorId: 'U1',
        event: {
          case: 'messagePosted',
          value: new MessagePostedEvent({ roomId: 'R1', bodyPlaintext: 'hello' })
        }
      })
    );

    const posted = room.ingestEvent.mock.calls[0][0] as TimelineEventView;
    expect(posted).toMatchObject({ id: 'E1', actorId: 'U1', actorResolution: resolution });
  });
});
