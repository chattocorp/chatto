import { afterEach, describe, expect, it, vi } from 'vitest';
import { MessageLongPressGesture } from './messageLongPress.svelte';

afterEach(() => {
  vi.useRealTimers();
  // A fired gesture leaves its opening-click guard; a new press clears it.
  window.dispatchEvent(new Event('pointerdown'));
});

describe('MessageLongPressGesture', () => {
  it('stages long-press highlighting before it fires', () => {
    vi.useFakeTimers();
    const onLongPress = vi.fn();
    const gesture = new MessageLongPressGesture(onLongPress);

    expect(gesture.pending).toBe(false);
    gesture.start();
    expect(gesture.pending).toBe(true);
    vi.advanceTimersByTime(149);
    expect(gesture.active).toBe(false);
    expect(onLongPress).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1);
    expect(gesture.active).toBe(true);

    vi.advanceTimersByTime(350);
    expect(gesture.active).toBe(false);
    expect(onLongPress).toHaveBeenCalledOnce();
    // The press that opened the sheet still belongs to the gesture until it ends.
    expect(gesture.pending).toBe(true);
    gesture.cancel();
    expect(gesture.pending).toBe(true);

    window.dispatchEvent(new Event('touchend'));
    expect(gesture.pending).toBe(false);
    gesture.dispose();
  });

  it('cancels both long-press stages when pointer movement begins', () => {
    vi.useFakeTimers();
    const onLongPress = vi.fn();
    const gesture = new MessageLongPressGesture(onLongPress);

    gesture.start();
    vi.advanceTimersByTime(200);
    gesture.cancel();
    vi.runAllTimers();

    expect(gesture.active).toBe(false);
    expect(gesture.pending).toBe(false);
    expect(onLongPress).not.toHaveBeenCalled();
  });

  it('discards the click from a release that misses the message', () => {
    vi.useFakeTimers();
    const gesture = new MessageLongPressGesture(() => {});
    const button = document.createElement('button');
    const onClick = vi.fn();
    button.addEventListener('click', onClick);
    document.body.append(button);

    gesture.start();
    vi.advanceTimersByTime(500);

    window.dispatchEvent(new Event('touchend'));
    button.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, detail: 1 }));
    expect(onClick).not.toHaveBeenCalled();

    gesture.dispose();
    button.remove();
  });

  it('accepts an immediate new tap after the opening press', () => {
    vi.useFakeTimers();
    const gesture = new MessageLongPressGesture(() => {});
    const button = document.createElement('button');
    const onClick = vi.fn();
    button.addEventListener('click', onClick);
    document.body.append(button);

    gesture.start();
    vi.advanceTimersByTime(500);
    window.dispatchEvent(new Event('touchend'));
    button.dispatchEvent(new Event('touchstart', { bubbles: true }));
    button.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, detail: 1 }));
    expect(onClick).toHaveBeenCalledOnce();

    gesture.dispose();
    button.remove();
  });
});
