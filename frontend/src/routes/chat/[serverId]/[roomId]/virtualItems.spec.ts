import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { buildVirtualItems, type VirtualItem } from './virtualItems';
import { computeEventMetadata, type EventWithMeta } from './messageGrouping';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import type { UserSettingsState } from '$lib/state/userSettings.svelte';

const utcSettings = {
  get effectiveTimezone() {
    return 'UTC';
  },
  get effectiveHour12() {
    return undefined;
  }
} as unknown as UserSettingsState;

function makeMessageEvent(
  overrides: Partial<{
    id: string;
    actorId: string;
    createdAt: string;
    body: string | null;
    attachments: unknown[];
    reactions: unknown[];
    replyCount: number;
    echoOfEventId: string | null;
  }> = {}
): RoomEventViewFragment {
  return {
    id: overrides.id ?? 'evt_' + Math.random().toString(36).slice(2),
    createdAt: overrides.createdAt ?? '2025-04-27T12:00:00Z',
    actorId: overrides.actorId ?? 'u_user1',
    actor: { id: overrides.actorId ?? 'u_user1', login: 'tester', avatarUrl: null },
    event: {
      __typename: 'MessagePostedEvent',
      roomId: 'r_test',
      body: 'body' in overrides ? overrides.body : 'Hello',
      attachments: overrides.attachments ?? [],
      linkPreview: null,
      reactions: overrides.reactions ?? [],
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId: null,
      replyCount: overrides.replyCount ?? 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: null,
      echoOfEventId: overrides.echoOfEventId ?? null
    }
  } as unknown as RoomEventViewFragment;
}

function makeSystemEvent(
  typename: 'UserJoinedRoomEvent' | 'UserLeftRoomEvent',
  overrides: Partial<{
    id: string;
    actorId: string;
    createdAt: string;
  }> = {}
): RoomEventViewFragment {
  const actorId = overrides.actorId ?? 'u_user1';
  return {
    id: overrides.id ?? 'evt_' + Math.random().toString(36).slice(2),
    createdAt: overrides.createdAt ?? '2025-04-27T12:00:00Z',
    actorId,
    actor: { id: actorId, login: 'tester', avatarUrl: null },
    event: {
      __typename: typename,
      roomId: 'r_test'
    }
  } as unknown as RoomEventViewFragment;
}

function meta(events: RoomEventViewFragment[]): EventWithMeta[] {
  return computeEventMetadata(events, utcSettings);
}

describe('buildVirtualItems', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-04-27T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns empty array for empty input', () => {
    expect(buildVirtualItems([], null, false)).toEqual([]);
  });

  it('omits start-marker when hasReachedStart is false', () => {
    const events = [makeMessageEvent({ id: 'e1' })];
    const items = buildVirtualItems(meta(events), null, false);
    expect(items.find((i) => i.type === 'start-marker')).toBeUndefined();
  });

  it('emits start-marker when hasReachedStart is true and there are events', () => {
    const events = [makeMessageEvent({ id: 'e1' })];
    const items = buildVirtualItems(meta(events), null, true);
    expect(items[0]).toMatchObject({ type: 'start-marker' });
  });

  it('does not emit start-marker for an empty event list even if hasReachedStart', () => {
    expect(buildVirtualItems([], null, true)).toEqual([]);
  });

  it('emits a day-separator before the first event', () => {
    const events = [makeMessageEvent({ id: 'e1', createdAt: '2025-04-27T10:00:00Z' })];
    const items = buildVirtualItems(meta(events), null, false);
    const sep = items.find((i) => i.type === 'day-separator');
    expect(sep).toBeDefined();
    expect(sep).toMatchObject({ type: 'day-separator', label: 'Today' });
  });

  it('emits a day-separator at day boundaries', () => {
    const events = [
      makeMessageEvent({ id: 'e1', createdAt: '2025-04-26T23:00:00Z' }),
      makeMessageEvent({ id: 'e2', createdAt: '2025-04-27T00:30:00Z' })
    ];
    const items = buildVirtualItems(meta(events), null, false);
    const separators = items.filter((i) => i.type === 'day-separator');
    expect(separators).toHaveLength(2);
  });

  it('does not duplicate day-separators within the same UTC day', () => {
    const events = [
      makeMessageEvent({ id: 'e1', createdAt: '2025-04-27T08:00:00Z' }),
      makeMessageEvent({ id: 'e2', createdAt: '2025-04-27T20:00:00Z', actorId: 'u_other' })
    ];
    const items = buildVirtualItems(meta(events), null, false);
    expect(items.filter((i) => i.type === 'day-separator')).toHaveLength(1);
  });

  it('emits an unread-separator before the matching event', () => {
    const events = [
      makeMessageEvent({ id: 'e1' }),
      makeMessageEvent({ id: 'e2' }),
      makeMessageEvent({ id: 'e3' })
    ];
    const items = buildVirtualItems(meta(events), 'e2', false);

    const e2Index = items.findIndex((i) => i.type === 'event' && i.key === 'e2');
    expect(e2Index).toBeGreaterThan(0);
    expect(items[e2Index - 1]).toMatchObject({ type: 'unread-separator' });
  });

  it('does not emit unread-separator when firstUnreadEventId is null', () => {
    const events = [makeMessageEvent({ id: 'e1' }), makeMessageEvent({ id: 'e2' })];
    const items = buildVirtualItems(meta(events), null, false);
    expect(items.find((i) => i.type === 'unread-separator')).toBeUndefined();
  });

  it('treats deleted messages like any other event (tombstone rendering, separators apply)', () => {
    const deleted = makeMessageEvent({
      id: 'deleted',
      body: null,
      attachments: [],
      reactions: [],
      replyCount: 0,
      createdAt: '2025-04-27T08:00:00Z'
    });
    const visible = makeMessageEvent({
      id: 'visible',
      createdAt: '2025-04-27T09:00:00Z'
    });
    const items = buildVirtualItems(meta([deleted, visible]), 'deleted', false);

    // Deleted message is rendered as a tombstone — it receives separators normally.
    const unread = items.findIndex((i) => i.type === 'unread-separator');
    expect(unread).toBeGreaterThan(-1);
    const deletedIndex = items.findIndex((i) => i.type === 'event' && i.key === 'deleted');
    expect(items[deletedIndex - 1]).toMatchObject({ type: 'unread-separator' });
  });

  it('passes isFirstInGroup through from event metadata', () => {
    const events = [
      makeMessageEvent({ id: 'e1', createdAt: '2025-04-27T12:00:00Z', actorId: 'u_a' }),
      makeMessageEvent({ id: 'e2', createdAt: '2025-04-27T12:00:30Z', actorId: 'u_a' })
    ];
    const items = buildVirtualItems(meta(events), null, false);
    const e2 = items.find((i) => i.type === 'event' && i.key === 'e2');
    expect(e2).toMatchObject({ type: 'event', isFirstInGroup: false });
  });

  it('produces stable, unique keys per item', () => {
    const events = [
      makeMessageEvent({ id: 'e1', createdAt: '2025-04-26T23:00:00Z' }),
      makeMessageEvent({ id: 'e2', createdAt: '2025-04-27T00:30:00Z' })
    ];
    const items = buildVirtualItems(meta(events), 'e2', true);
    const keys = items.map((i) => i.key);
    expect(new Set(keys).size).toBe(keys.length);
  });

  describe('system event grouping (join/leave)', () => {
    it('coalesces consecutive joins into a single system-group', () => {
      const events = [
        makeSystemEvent('UserJoinedRoomEvent', {
          id: 'j1',
          actorId: 'u_a',
          createdAt: '2025-04-27T12:00:00Z'
        }),
        makeSystemEvent('UserJoinedRoomEvent', {
          id: 'j2',
          actorId: 'u_b',
          createdAt: '2025-04-27T12:00:10Z'
        }),
        makeSystemEvent('UserJoinedRoomEvent', {
          id: 'j3',
          actorId: 'u_c',
          createdAt: '2025-04-27T12:00:20Z'
        })
      ];

      const items = buildVirtualItems(meta(events), null, false);
      const groups = items.filter((i) => i.type === 'system-group');
      expect(groups).toHaveLength(1);
      expect(groups[0]).toMatchObject({ kind: 'join' });
      expect((groups[0] as Extract<VirtualItem, { type: 'system-group' }>).events).toHaveLength(3);
      expect(items.find((i) => i.type === 'event')).toBeUndefined();
    });

    it('splits joins and leaves into separate groups', () => {
      const events = [
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j1', actorId: 'u_a' }),
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j2', actorId: 'u_b' }),
        makeSystemEvent('UserLeftRoomEvent', { id: 'l1', actorId: 'u_c' }),
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j3', actorId: 'u_d' }),
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j4', actorId: 'u_e' })
      ];

      const items = buildVirtualItems(meta(events), null, false);
      const groups = items.filter(
        (i): i is Extract<VirtualItem, { type: 'system-group' }> => i.type === 'system-group'
      );
      expect(groups.map((g) => ({ kind: g.kind, count: g.events.length }))).toEqual([
        { kind: 'join', count: 2 },
        { kind: 'leave', count: 1 },
        { kind: 'join', count: 2 }
      ]);
    });

    it('groups leave events by actor-only membership facts', () => {
      const events = [
        makeSystemEvent('UserLeftRoomEvent', { id: 'l1', actorId: 'u_a' }),
        makeSystemEvent('UserLeftRoomEvent', { id: 'l2', actorId: 'u_b' }),
        makeSystemEvent('UserLeftRoomEvent', { id: 'l3', actorId: 'u_c' })
      ];

      const items = buildVirtualItems(meta(events), null, false);
      const groups = items.filter(
        (i): i is Extract<VirtualItem, { type: 'system-group' }> => i.type === 'system-group'
      );
      expect(groups.map((g) => ({ kind: g.kind, count: g.events.length }))).toEqual([
        { kind: 'leave', count: 3 }
      ]);
    });

    it('breaks the group when interrupted by a normal message', () => {
      const events = [
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j1', actorId: 'u_a' }),
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j2', actorId: 'u_b' }),
        makeMessageEvent({ id: 'm1', actorId: 'u_a' }),
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j3', actorId: 'u_c' })
      ];

      const items = buildVirtualItems(meta(events), null, false);
      const groups = items.filter(
        (i): i is Extract<VirtualItem, { type: 'system-group' }> => i.type === 'system-group'
      );
      expect(groups).toHaveLength(2);
      expect(groups[0].events).toHaveLength(2);
      expect(groups[1].events).toHaveLength(1);
    });

    it('breaks the group at a day boundary', () => {
      const events = [
        makeSystemEvent('UserJoinedRoomEvent', {
          id: 'j1',
          actorId: 'u_a',
          createdAt: '2025-04-26T23:00:00Z'
        }),
        makeSystemEvent('UserJoinedRoomEvent', {
          id: 'j2',
          actorId: 'u_b',
          createdAt: '2025-04-27T00:30:00Z'
        })
      ];

      const items = buildVirtualItems(meta(events), null, false);
      const groups = items.filter(
        (i): i is Extract<VirtualItem, { type: 'system-group' }> => i.type === 'system-group'
      );
      expect(groups).toHaveLength(2);
      // Day separator should appear before each group's leading event
      expect(items.filter((i) => i.type === 'day-separator')).toHaveLength(2);
    });

    it('breaks the group at the unread separator', () => {
      const events = [
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j1', actorId: 'u_a' }),
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j2', actorId: 'u_b' }),
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j3', actorId: 'u_c' })
      ];

      const items = buildVirtualItems(meta(events), 'j2', false);
      const groups = items.filter(
        (i): i is Extract<VirtualItem, { type: 'system-group' }> => i.type === 'system-group'
      );
      expect(groups).toHaveLength(2);
      expect(groups[0].events.map((e) => e.id)).toEqual(['j1']);
      expect(groups[1].events.map((e) => e.id)).toEqual(['j2', 'j3']);

      // Unread separator should sit between the two groups
      const idxUnread = items.findIndex((i) => i.type === 'unread-separator');
      const idxFirstGroup = items.findIndex((i) => i.type === 'system-group');
      expect(idxUnread).toBeGreaterThan(idxFirstGroup);
    });

    it('still emits non-grouped system events (archive/unarchive) as plain events', () => {
      const archive: RoomEventViewFragment = {
        id: 'a1',
        createdAt: '2025-04-27T12:00:00Z',
        actorId: 'u_a',
        actor: { id: 'u_a', login: 'tester', avatarUrl: null },
        event: { __typename: 'RoomArchivedEvent', roomId: 'r_test' }
      } as unknown as RoomEventViewFragment;

      const items = buildVirtualItems(meta([archive]), null, false);
      expect(items.find((i) => i.type === 'system-group')).toBeUndefined();
      expect(items.find((i) => i.type === 'event')).toMatchObject({ key: 'a1' });
    });

    it('uses a stable, unique key for system-groups', () => {
      const events = [
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j1' }),
        makeSystemEvent('UserJoinedRoomEvent', { id: 'j2' }),
        makeSystemEvent('UserLeftRoomEvent', { id: 'l1' }),
        makeSystemEvent('UserLeftRoomEvent', { id: 'l2' })
      ];
      const items = buildVirtualItems(meta(events), null, false);
      const keys = items.map((i) => i.key);
      expect(new Set(keys).size).toBe(keys.length);
      // Keyed by the newest event in the group so the key stays stable when
      // pagination prepends older events that merge into the group.
      expect(keys).toContain('system-group-j2');
      expect(keys).toContain('system-group-l2');
    });
  });
});
