import { afterEach, describe, expect, it, vi } from 'vitest';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { RoomEventView } from '$lib/render/types';
import type { UserSettingsState } from '$lib/state/userSettings.svelte';
import { computeEventMetadata } from './messageGrouping';
import { buildVirtualItems } from './virtualItems';
import {
  MESSAGE_TOMBSTONE_GRACE_MS,
  nextTombstoneExpiry,
  scheduleNextTombstoneExpiry,
  shouldHideTombstone,
  tombstoneExpiry,
  visibleTombstoneEvents,
  visibleUnreadMarkerEventId
} from './tombstoneVisibility';

const deletedAt = '2026-07-10T10:00:00.000Z';
const deletedAtMs = Date.parse(deletedAt);
const utcSettings = {
  get effectiveTimezone() {
    return 'UTC';
  },
  get effectiveHour12() {
    return undefined;
  }
} as unknown as UserSettingsState;

function message(overrides: Record<string, unknown> = {}): RoomEventView {
  return {
    id: String(overrides.id ?? 'message-1'),
    createdAt: String(overrides.createdAt ?? '2026-07-10T09:00:00.000Z'),
    actorId: 'user-1',
    actor: null,
    event: {
      kind: RoomEventKind.MessagePosted,
      roomId: 'room-1',
      body: null,
      attachments: [],
      linkPreview: null,
      reactions: [],
      updatedAt: null,
      inReplyTo: null,
      threadRootEventId: null,
      echoOfEventId: null,
      echoFromThreadRootEventId: null,
      channelEchoEventId: null,
      deletedAt,
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: null,
      ...overrides
    }
  } as RoomEventView;
}

describe('tombstone visibility', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('hides a context-free tombstone at the inclusive one-hour boundary', () => {
    const event = message();
    expect(shouldHideTombstone(event, deletedAtMs + MESSAGE_TOMBSTONE_GRACE_MS - 1)).toBe(false);
    expect(shouldHideTombstone(event, deletedAtMs + MESSAGE_TOMBSTONE_GRACE_MS)).toBe(true);
  });

  it('conservatively keeps unavailable messages without tombstone metadata', () => {
    expect(tombstoneExpiry(message({ deletedAt: null }))).toBeNull();
    expect(tombstoneExpiry(message({ deletedAt: 'invalid' }))).toBeNull();
  });

  it.each([
    ['body', { body: 'still available' }],
    ['attachment', { attachments: [{ id: 'asset-1' }] }],
    ['link preview', { linkPreview: { url: 'https://example.com' } }],
    ['reaction', { reactions: [{ emoji: '👍', count: 1, hasReacted: false, users: [] }] }],
    ['thread reply', { replyCount: 1 }]
  ])('keeps a tombstone with %s', (_label, overrides) => {
    expect(tombstoneExpiry(message(overrides))).toBeNull();
  });

  it.each([
    ['reply', { inReplyTo: 'target-1' }],
    ['thread message', { threadRootEventId: 'root-1' }],
    ['channel echo', { echoOfEventId: 'reply-1', echoFromThreadRootEventId: 'root-1' }]
  ])('does not retain a tombstone merely because it is a %s', (_label, overrides) => {
    expect(shouldHideTombstone(message(overrides), deletedAtMs + MESSAGE_TOMBSTONE_GRACE_MS)).toBe(
      true
    );
  });

  it('returns the next finite expiry only', () => {
    const later = message({ id: 'later', deletedAt: '2026-07-10T10:30:00.000Z' });
    const persistent = message({ id: 'persistent', replyCount: 1 });
    expect(nextTombstoneExpiry([later, persistent], deletedAtMs)).toBe(
      Date.parse('2026-07-10T11:30:00.000Z')
    );
  });

  it('reschedules from the first expiry to the next expiry', () => {
    vi.useFakeTimers();
    vi.setSystemTime(deletedAtMs);
    const first = message();
    const second = message({ id: 'second', deletedAt: '2026-07-10T10:30:00.000Z' });
    const expired: number[] = [];

    let cleanup = scheduleNextTombstoneExpiry([first, second], Date.now(), (expiresAt) => {
      expired.push(expiresAt);
    });
    expect(vi.getTimerCount()).toBe(1);

    vi.advanceTimersByTime(MESSAGE_TOMBSTONE_GRACE_MS);
    expect(expired).toEqual([deletedAtMs + MESSAGE_TOMBSTONE_GRACE_MS]);

    cleanup();
    cleanup = scheduleNextTombstoneExpiry([first, second], Date.now(), (expiresAt) => {
      expired.push(expiresAt);
    });
    expect(vi.getTimerCount()).toBe(1);
    vi.advanceTimersByTime(30 * 60 * 1000);
    expect(expired).toEqual([
      deletedAtMs + MESSAGE_TOMBSTONE_GRACE_MS,
      Date.parse('2026-07-10T11:30:00.000Z')
    ]);
    cleanup();
  });

  it('cancels a stale timer when context begins retaining the tombstone', () => {
    vi.useFakeTimers();
    vi.setSystemTime(deletedAtMs);
    const onExpire = vi.fn();

    const cleanup = scheduleNextTombstoneExpiry([message()], Date.now(), onExpire);
    expect(vi.getTimerCount()).toBe(1);
    cleanup();
    scheduleNextTombstoneExpiry([message({ replyCount: 1 })], Date.now(), onExpire);
    expect(vi.getTimerCount()).toBe(0);

    vi.advanceTimersByTime(MESSAGE_TOMBSTONE_GRACE_MS);
    expect(onExpire).not.toHaveBeenCalled();
  });

  it('does not leave a timer when retaining context disappears after the deadline', () => {
    vi.useFakeTimers();
    vi.setSystemTime(deletedAtMs + MESSAGE_TOMBSTONE_GRACE_MS + 1);
    const onExpire = vi.fn();

    scheduleNextTombstoneExpiry([message()], Date.now(), onExpire);

    expect(shouldHideTombstone(message(), Date.now())).toBe(true);
    expect(onExpire).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('clears the scheduled expiry during teardown', () => {
    vi.useFakeTimers();
    vi.setSystemTime(deletedAtMs);
    const onExpire = vi.fn();

    const cleanup = scheduleNextTombstoneExpiry([message()], Date.now(), onExpire);
    cleanup();
    vi.advanceTimersByTime(MESSAGE_TOMBSTONE_GRACE_MS);

    expect(onExpire).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('moves an unread marker from an expired tombstone to the next visible event', () => {
    const expired = message({ id: 'expired' });
    const next = message({ id: 'next', body: 'visible', deletedAt: null });
    expect(visibleUnreadMarkerEventId([expired, next], [next], 'expired')).toBe('next');
    expect(visibleUnreadMarkerEventId([expired], [], 'expired')).toBeNull();
  });

  it('recomputes grouping, day separators, and unread placement after expiry', () => {
    const expired = message({ id: 'expired', createdAt: '2026-07-09T23:59:00.000Z' });
    const next = message({
      id: 'next',
      createdAt: '2026-07-10T00:01:00.000Z',
      body: 'visible',
      deletedAt: null
    });
    const timeline = [expired, next];
    const visible = visibleTombstoneEvents(timeline, deletedAtMs + MESSAGE_TOMBSTONE_GRACE_MS);
    const unreadId = visibleUnreadMarkerEventId(timeline, visible, expired.id);
    const items = buildVirtualItems(
      computeEventMetadata(visible, utcSettings, 'en'),
      unreadId,
      false
    );

    expect(visible.map((event) => event.id)).toEqual(['next']);
    expect(items.filter((item) => item.type === 'day-separator')).toHaveLength(1);
    expect(items.map((item) => item.type)).toEqual(['day-separator', 'unread-separator', 'event']);
    expect(items.at(-1)).toMatchObject({ type: 'event', key: 'next', isFirstInGroup: true });
  });

  it('removes separators when the last event expires', () => {
    const visible = visibleTombstoneEvents([message()], deletedAtMs + MESSAGE_TOMBSTONE_GRACE_MS);
    expect(buildVirtualItems(computeEventMetadata(visible, utcSettings, 'en'), null, true)).toEqual(
      []
    );
  });
});
