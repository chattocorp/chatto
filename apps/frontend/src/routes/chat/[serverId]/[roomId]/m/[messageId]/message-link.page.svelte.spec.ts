import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { RoomTimelineAPI } from '$lib/api-client/roomTimeline';
import {
  TimelineEventKind,
  type MessagePostedPayload,
  type TimelineEventView
} from '$lib/render/timelineEvents';
import { PendingHighlightStore } from '$lib/state/server/pendingHighlight.svelte';
import { resolveAndRedirect } from './+page.svelte';

const { goto } = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock('$app/navigation', () => ({ goto }));
vi.mock('$app/state', () => ({ page: {} }));
vi.mock('$lib/state/server/scope.svelte', () => ({ useServerScope: vi.fn() }));
vi.mock('$lib/api-client/roomTimeline', () => ({ createRoomTimelineAPI: vi.fn() }));
vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    path.replace(/\[(\w+)\]/g, (_, key: string) => params[key])
}));

function message(overrides: Partial<MessagePostedPayload> = {}): TimelineEventView {
  return {
    id: 'message-1',
    createdAt: '2026-09-20T12:00:00Z',
    actor: null,
    event: {
      kind: TimelineEventKind.MessagePosted,
      roomId: 'room-1',
      body: 'Linked message',
      attachments: [],
      reactions: [],
      replyCount: 0,
      threadParticipants: [],
      ...overrides
    }
  };
}

describe('message link resolver', () => {
  const getMessage = vi.fn<RoomTimelineAPI['getMessage']>();
  let highlights: PendingHighlightStore;

  beforeEach(() => {
    vi.clearAllMocks();
    highlights = new PendingHighlightStore();
  });

  it.each([
    {
      name: 'root with replies',
      payload: { threadExists: true, replyCount: 2 },
      threadId: 'message-1'
    },
    {
      name: 'existing empty thread',
      payload: { threadExists: true, replyCount: 0 },
      threadId: 'message-1'
    },
    { name: 'message without a thread', payload: { threadExists: false }, threadId: null },
    { name: 'message without thread metadata', payload: {}, threadId: null },
    {
      name: 'thread reply',
      payload: { threadRootEventId: 'root-1', threadExists: true },
      threadId: 'root-1'
    }
  ])('opens the correct pane for a $name', async ({ payload, threadId }) => {
    getMessage.mockResolvedValue(message(payload));

    await resolveAndRedirect({ getMessage }, highlights, 'remote.example', 'room-1', 'message-1');

    expect(getMessage).toHaveBeenCalledWith({ roomId: 'room-1', eventId: 'message-1' });
    expect(goto).toHaveBeenCalledWith(
      `/chat/remote.example/room-1${threadId ? `/${threadId}` : ''}`,
      { replaceState: true }
    );
    expect(highlights.consume('room-1', threadId)).toEqual({
      eventId: 'message-1',
      notificationId: null
    });
    expect(highlights.consume('room-1', threadId)).toBeNull();
  });

  it('keeps the room highlight fallback for a missing target', async () => {
    getMessage.mockResolvedValue(null);
    await resolveAndRedirect({ getMessage }, highlights, '-', 'room-1', 'message-1');
    expect(goto).toHaveBeenCalledWith('/chat/-/room-1', { replaceState: true });
    expect(highlights.consume('room-1', null)?.eventId).toBe('message-1');
  });

  it('returns to the room without a highlight when the request fails', async () => {
    getMessage.mockRejectedValue(new Error('Request failed'));
    await resolveAndRedirect({ getMessage }, highlights, '-', 'room-1', 'message-1');
    expect(goto).toHaveBeenCalledWith('/chat/-/room-1', { replaceState: true });
    expect(highlights.consume('room-1', null)).toBeNull();
  });

  it.each([false, true])('ignores a stale request (failure: %s)', async (fails) => {
    let current = true;
    getMessage.mockImplementation(async () => {
      current = false;
      if (fails) throw new Error('Request failed');
      return message({ threadExists: true });
    });
    await resolveAndRedirect(
      { getMessage },
      highlights,
      '-',
      'room-1',
      'message-1',
      () => current
    );
    expect(goto).not.toHaveBeenCalled();
    expect(highlights.consume('room-1', 'message-1')).toBeNull();
    expect(highlights.consume('room-1', null)).toBeNull();
  });
});
