import { describe, expect, it } from 'vitest';
import { TimelineEventKind, type TimelineEventView } from '$lib/render/timelineEvents';
import type { TimeFormatSettings } from '$lib/utils/formatTime';
import { computeEventMetadata } from './messageGrouping';
import { buildVirtualItems } from './virtualItems';
import {
  shouldHideTombstone,
  visibleTombstoneEvents,
  visibleUnreadMarkerEventId
} from './tombstoneVisibility';

const utcSettings = {
  effectiveTimezone: 'UTC',
  effectiveHour12: undefined
} satisfies TimeFormatSettings;

function message(overrides: Record<string, unknown> = {}): TimelineEventView {
  return {
    id: String(overrides.id ?? 'message-1'),
    createdAt: String(overrides.createdAt ?? '2026-07-10T09:00:00.000Z'),
    actorId: 'user-1',
    actor: null,
    event: {
      kind: TimelineEventKind.MessagePosted,
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
      deletedAt: '2026-07-10T10:00:00.000Z',
      replyCount: 0,
      lastReplyAt: null,
      threadParticipants: [],
      viewerIsFollowingThread: null,
      ...overrides
    }
  } as TimelineEventView;
}

describe('tombstone visibility', () => {
  it('hides a confirmed context-free tombstone immediately', () => {
    expect(shouldHideTombstone(message())).toBe(true);
  });

  it('conservatively keeps unavailable messages without deletion metadata', () => {
    expect(shouldHideTombstone(message({ deletedAt: null }))).toBe(false);
  });

  it('immediately hides an attachment-only message after its final attachment is removed', () => {
    expect(
      shouldHideTombstone(
        message({ body: '', deletedAt: null, updatedAt: '2026-07-10T10:30:00.000Z' })
      )
    ).toBe(true);
  });

  it('does not infer deletion from an edited message whose body is unavailable', () => {
    expect(
      shouldHideTombstone(
        message({ body: null, deletedAt: null, updatedAt: '2026-07-10T10:30:00.000Z' })
      )
    ).toBe(false);
  });

  it.each([
    ['body', { body: 'still available' }],
    ['attachment', { attachments: [{ id: 'asset-1' }] }],
    ['link preview', { linkPreview: { url: 'https://example.com' } }],
    ['reaction', { reactions: [{ emoji: '👍', count: 1, hasReacted: false, users: [] }] }],
    ['thread reply', { replyCount: 1 }]
  ])('keeps a tombstone with %s', (_label, overrides) => {
    expect(shouldHideTombstone(message(overrides))).toBe(false);
  });

  it.each([
    ['reply', { inReplyTo: 'target-1' }],
    ['thread message', { threadRootEventId: 'root-1' }],
    ['channel echo', { echoOfEventId: 'reply-1', echoFromThreadRootEventId: 'root-1' }]
  ])('does not retain a tombstone merely because it is a %s', (_label, overrides) => {
    expect(shouldHideTombstone(message(overrides))).toBe(true);
  });

  it('moves an unread marker from a hidden tombstone to the next visible event', () => {
    const hidden = message({ id: 'hidden' });
    const next = message({ id: 'next', body: 'visible', deletedAt: null });
    expect(visibleUnreadMarkerEventId([hidden, next], [next], 'hidden')).toBe('next');
    expect(visibleUnreadMarkerEventId([hidden], [], 'hidden')).toBeNull();
  });

  it('recomputes grouping, day separators, and unread placement after immediate removal', () => {
    const hidden = message({ id: 'hidden', createdAt: '2026-07-09T23:59:00.000Z' });
    const next = message({
      id: 'next',
      createdAt: '2026-07-10T00:01:00.000Z',
      body: 'visible',
      deletedAt: null
    });
    const timeline = [hidden, next];
    const visible = visibleTombstoneEvents(timeline);
    const unreadId = visibleUnreadMarkerEventId(timeline, visible, hidden.id);
    const items = buildVirtualItems(
      computeEventMetadata(visible, utcSettings, 'en-GB'),
      unreadId,
      false
    );

    expect(visible.map((event) => event.id)).toEqual(['next']);
    expect(items.filter((item) => item.type === 'day-separator')).toHaveLength(1);
    expect(items.map((item) => item.type)).toEqual(['day-separator', 'unread-separator', 'event']);
    expect(items.at(-1)).toMatchObject({ type: 'event', key: 'next', isFirstInGroup: true });
  });

  it('removes separators when the last event is a context-free tombstone', () => {
    const visible = visibleTombstoneEvents([message()]);
    expect(
      buildVirtualItems(computeEventMetadata(visible, utcSettings, 'en-GB'), null, true)
    ).toEqual([]);
  });
});
