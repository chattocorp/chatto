import { describe, expect, it } from 'vitest';
import { RoomNavigationState } from './roomNavigationState.svelte';

describe('RoomNavigationState', () => {
  const reply = { eventId: 'reply-1', actorDisplayName: 'Alice', excerpt: 'Reply excerpt' };

  it('prepares one-shot thread highlight and composer input for the opened thread', () => {
    const state = new RoomNavigationState();

    state.prepareThreadOpen('room-1', 'thread-1', {
      highlightEventId: 'highlight-1',
      quoteText: 'selected quote',
      reply
    });

    const highlight = state.highlightFor('room-1', 'thread-1');
    expect(highlight).toEqual({
      roomId: 'room-1',
      threadRootEventId: 'thread-1',
      eventId: 'highlight-1',
      notificationId: null
    });
    const input = state.composerInputFor('room-1', 'thread-1');
    expect(input).toEqual({
      roomId: 'room-1',
      threadRootEventId: 'thread-1',
      quote: 'selected quote',
      reply
    });
    expect(state.highlightFor('room-1', null)).toBeNull();
    expect(state.composerInputFor('room-1', 'thread-2')).toBeNull();

    state.clearHighlight(highlight!);
    state.clearComposerInput(input!);
    expect(state.highlight).toBeNull();
    expect(state.composerInput).toBeNull();
  });

  it('clears stale thread hand-offs when a later thread open omits them', () => {
    const state = new RoomNavigationState();
    state.prepareThreadOpen('room-1', 'thread-1', {
      highlightEventId: 'highlight-1',
      quoteText: 'selected quote',
      reply
    });

    state.prepareThreadOpen('room-1', 'thread-2');

    expect(state.highlight).toBeNull();
    expect(state.composerInput).toBeNull();
  });

  it('keeps a pending room highlight when a thread opens without one', () => {
    const state = new RoomNavigationState();
    state.beginHighlight('room-1', null, 'message-1');

    state.prepareThreadOpen('room-1', 'thread-1');

    expect(state.highlightFor('room-1', null)?.eventId).toBe('message-1');
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

  it('consumes a highlight parameter once until the parameter disappears', () => {
    const state = new RoomNavigationState();

    expect(state.consumeHighlightParam('room-1', undefined, 'message-1')).toBe('message-1');
    expect(state.consumeHighlightParam('room-1', undefined, 'message-1')).toBeNull();
    expect(state.consumeHighlightParam('room-1', 'thread-1', 'message-1')).toBe('message-1');
    expect(state.consumeHighlightParam('room-1', 'thread-1', null)).toBeNull();
    expect(state.consumeHighlightParam('room-1', 'thread-1', 'message-1')).toBe('message-1');
  });

  it('ignores a late completion for a replaced highlight', () => {
    const state = new RoomNavigationState();
    state.beginHighlight('room-1', null, 'message-1', 'notification-1');
    const first = state.highlight!;
    state.beginHighlight('room-1', null, 'message-2');

    state.clearHighlight(first);

    expect(state.highlight?.eventId).toBe('message-2');
  });

  it('drops only a room-timeline highlight when the room becomes inactive', () => {
    const state = new RoomNavigationState();
    state.beginHighlight('room-1', null, 'message-1');
    state.clearMainHighlight();
    expect(state.highlight).toBeNull();

    state.beginHighlight('room-1', 'thread-1', 'message-2');
    state.clearMainHighlight();
    expect(state.highlightFor('room-1', 'thread-1')?.eventId).toBe('message-2');
  });
});
