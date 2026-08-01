import { describe, expect, it } from 'vitest';
import { RoomNavigationState } from './roomNavigationState.svelte';

describe('RoomNavigationState', () => {
  it('prepares one-shot thread highlight, quote, and reply hand-offs', () => {
    const state = new RoomNavigationState();

    state.prepareThreadOpen('thread-1', {
      highlightEventId: 'highlight-1',
      quoteText: 'selected quote',
      reply: {
        eventId: 'reply-1',
        actorDisplayName: 'Alice',
        excerpt: 'Reply excerpt'
      }
    });

    expect(state.pendingThreadHighlight).toBe('highlight-1');
    expect(state.pendingThreadQuote).toEqual({
      id: 1,
      text: 'selected quote'
    });
    expect(state.pendingThreadReply).toEqual({
      id: 1,
      threadRootEventId: 'thread-1',
      eventId: 'reply-1',
      actorDisplayName: 'Alice',
      excerpt: 'Reply excerpt'
    });

    state.clearThreadHighlight();
    state.clearThreadQuote();
    state.clearThreadReply();
    expect(state.pendingThreadHighlight).toBeNull();
    expect(state.pendingThreadQuote).toBeNull();
    expect(state.pendingThreadReply).toBeNull();
  });

  it('clears stale hand-offs when a later thread open omits them', () => {
    const state = new RoomNavigationState();
    state.prepareThreadOpen('thread-1', {
      highlightEventId: 'highlight-1',
      quoteText: 'selected quote',
      reply: {
        eventId: 'reply-1',
        actorDisplayName: 'Alice',
        excerpt: 'Reply excerpt'
      }
    });

    state.prepareThreadOpen('thread-2');

    expect(state.pendingThreadHighlight).toBeNull();
    expect(state.pendingThreadQuote).toBeNull();
    expect(state.pendingThreadReply).toBeNull();
  });

  it('consumes each nested thread message route once', () => {
    const state = new RoomNavigationState();

    expect(state.consumeThreadMessageRoute('room-1', 'thread-1', 'message-1')).toBe('message-1');
    expect(state.consumeThreadMessageRoute('room-1', 'thread-1', 'message-1')).toBeNull();
    expect(state.consumeThreadMessageRoute('room-1', 'thread-1', 'message-2')).toBe('message-2');
    expect(state.consumeThreadMessageRoute('room-2', 'thread-1', 'message-2')).toBe('message-2');
  });

  it('allows the same nested route after leaving it', () => {
    const state = new RoomNavigationState();

    expect(state.consumeThreadMessageRoute('room-1', 'thread-1', 'message-1')).toBe('message-1');
    expect(state.consumeThreadMessageRoute('room-1', undefined, undefined)).toBeUndefined();
    expect(state.consumeThreadMessageRoute('room-1', 'thread-1', 'message-1')).toBe('message-1');
  });

  it('fences stale failed main-room highlight requests', () => {
    const state = new RoomNavigationState();
    const firstRequest = state.beginHighlight('message-1', false);
    const secondRequest = state.beginHighlight('message-2', false);

    expect(firstRequest).toBeTypeOf('number');
    expect(secondRequest).toBeTypeOf('number');
    expect(state.failMainHighlight(firstRequest!, 'message-1')).toBe(false);
    expect(state.pendingMainHighlightId).toBe('message-2');
    expect(state.failMainHighlight(secondRequest!, 'message-2')).toBe(true);
    expect(state.pendingMainHighlightId).toBeNull();
  });

  it('fences a cleared main-room highlight from a newer request', () => {
    const state = new RoomNavigationState();
    const clearedRequest = state.beginHighlight('message-1', false);
    state.clearMainHighlight();
    const currentRequest = state.beginHighlight('message-2', false);

    expect(state.failMainHighlight(clearedRequest!, 'message-1')).toBe(false);
    expect(state.pendingMainHighlightId).toBe('message-2');
    expect(state.failMainHighlight(currentRequest!, 'message-2')).toBe(true);
  });

  it('routes thread highlights without creating a main-room request', () => {
    const state = new RoomNavigationState();

    expect(state.beginHighlight('message-1', true)).toBeNull();
    expect(state.pendingThreadHighlight).toBe('message-1');
    expect(state.pendingMainHighlightId).toBeNull();
  });
});
