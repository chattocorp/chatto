import { afterEach, describe, expect, it, vi } from 'vitest';
import { MessageEventInteractionState } from './messageEventInteractions.svelte';

afterEach(() => {
  vi.useRealTimers();
});

describe('MessageEventInteractionState', () => {
  it('stages long-press highlighting before opening the action sheet', () => {
    vi.useFakeTimers();
    const state = new MessageEventInteractionState();

    state.startLongPress();
    vi.advanceTimersByTime(149);
    expect(state.longPressActive).toBe(false);
    expect(state.showActionSheet).toBe(false);

    vi.advanceTimersByTime(1);
    expect(state.longPressActive).toBe(true);

    vi.advanceTimersByTime(350);
    expect(state.longPressActive).toBe(false);
    expect(state.showActionSheet).toBe(true);
  });

  it('cancels both long-press stages when pointer movement begins', () => {
    vi.useFakeTimers();
    const state = new MessageEventInteractionState();

    state.startLongPress();
    vi.advanceTimersByTime(200);
    state.cancelLongPress();
    vi.runAllTimers();

    expect(state.longPressActive).toBe(false);
    expect(state.showActionSheet).toBe(false);
  });

  it('tracks floating and sheet emoji-picker presentations independently', () => {
    const state = new MessageEventInteractionState();

    state.contextMenuPosition = { x: 20, y: 30 };
    state.openEmojiPicker();
    expect(state.emojiPickerPosition).toEqual({ x: 20, y: 30 });
    expect(state.emojiPickerPresentation).toBe('auto');

    state.openEmojiPicker('sheet');
    expect(state.emojiPickerPresentation).toBe('sheet');

    state.closeEmojiPicker();
    expect(state.emojiPickerPosition).toBeNull();
    expect(state.emojiPickerPresentation).toBe('auto');
  });
});
