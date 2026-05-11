import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { buildVirtualItems } from './virtualItems';
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
      inThread: null,
      replyCount: overrides.replyCount ?? 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: null,
      echoOfEventId: overrides.echoOfEventId ?? null
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

  it('skips day-separator and unread-separator for hidden (deleted) events', () => {
    const hidden = makeMessageEvent({
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
    const items = buildVirtualItems(meta([hidden, visible]), 'deleted', false);

    // The deleted event itself is still emitted (caller decides whether to render it),
    // but no day or unread separator is attached to it.
    expect(items.find((i) => i.type === 'unread-separator')).toBeUndefined();
    // Only one day-separator (in front of the hidden event since it's the first).
    // Verify that no second day-separator is wrongly inserted before the visible event.
    expect(items.filter((i) => i.type === 'day-separator')).toHaveLength(1);
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
});
