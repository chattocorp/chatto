import { afterEach, describe, expect, it, vi } from 'vitest';
import { panGesture } from './panGesture.svelte';

function hostElement() {
  const host = document.createElement('div');
  host.setPointerCapture = vi.fn();
  host.releasePointerCapture = vi.fn();
  document.body.append(host);
  return host;
}

function pointer(type: string, x: number, y = 24) {
  return new PointerEvent(type, {
    bubbles: true,
    cancelable: true,
    pointerId: 1,
    pointerType: 'mouse',
    clientX: x,
    clientY: y
  });
}

function touch(type: string, x: number, y = 24) {
  const event = new Event(type, { bubbles: true, cancelable: true }) as TouchEvent;
  const item = { identifier: 1, clientX: x, clientY: y };
  const currentTouches = type === 'touchend' || type === 'touchcancel' ? [] : [item];
  const touchList = (items: typeof currentTouches) =>
    Object.assign(items, { item: (i: number) => items[i] ?? null });
  Object.defineProperty(event, 'touches', {
    value: touchList(currentTouches)
  });
  Object.defineProperty(event, 'changedTouches', {
    value: touchList([item])
  });
  return event;
}

describe('panGesture', () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it('tracks pointer drags on the x axis after the pointer leaves the host', () => {
    const host = hostElement();
    const onStart = vi.fn();
    const onUpdate = vi.fn();
    const onEnd = vi.fn();
    const action = panGesture(host, {
      axis: 'x',
      shouldClaim: (dx) => dx < 0,
      onStart,
      onUpdate,
      onEnd
    });

    host.dispatchEvent(pointer('pointerdown', 320));
    window.dispatchEvent(pointer('pointermove', 20));
    window.dispatchEvent(pointer('pointerup', 20));

    expect(onStart).toHaveBeenCalledOnce();
    expect(onUpdate).toHaveBeenLastCalledWith(-300);
    expect(onEnd).toHaveBeenCalledWith(-300, expect.any(Number));
    expect(host.setPointerCapture).toHaveBeenCalledWith(1);

    action.destroy();
  });

  it('tracks touch drags and prevents default after claiming', () => {
    const host = hostElement();
    const onStart = vi.fn();
    const onUpdate = vi.fn();
    const onEnd = vi.fn();
    const action = panGesture(host, {
      axis: 'x',
      shouldClaim: (dx) => dx < 0,
      onStart,
      onUpdate,
      onEnd
    });

    host.dispatchEvent(touch('touchstart', 320));
    const move = touch('touchmove', 20);
    window.dispatchEvent(move);
    window.dispatchEvent(touch('touchend', 20));

    expect(move.defaultPrevented).toBe(true);
    expect(onStart).toHaveBeenCalledOnce();
    expect(onUpdate).toHaveBeenLastCalledWith(-300);
    expect(onEnd).toHaveBeenCalledWith(-300, expect.any(Number));

    action.destroy();
  });

  it('tracks pointer drags on the y axis', () => {
    const host = hostElement();
    const onUpdate = vi.fn();
    const onEnd = vi.fn();
    const action = panGesture(host, {
      axis: 'y',
      shouldClaim: (dy) => dy > 0,
      onUpdate,
      onEnd
    });

    host.dispatchEvent(pointer('pointerdown', 48, 20));
    window.dispatchEvent(pointer('pointermove', 48, 120));
    window.dispatchEvent(pointer('pointerup', 48, 120));

    expect(onUpdate).toHaveBeenLastCalledWith(100);
    expect(onEnd).toHaveBeenCalledWith(100, expect.any(Number));

    action.destroy();
  });

  it('reports taps without claiming a drag', () => {
    const host = hostElement();
    const onTap = vi.fn();
    const onStart = vi.fn();
    const onEnd = vi.fn();
    const action = panGesture(host, {
      axis: 'x',
      onTap,
      onStart,
      onEnd
    });

    host.dispatchEvent(pointer('pointerdown', 12));
    window.dispatchEvent(pointer('pointerup', 12));

    expect(onTap).toHaveBeenCalledWith(12, 24);
    expect(onStart).not.toHaveBeenCalled();
    expect(onEnd).not.toHaveBeenCalled();

    action.destroy();
  });

  it('drops perpendicular movement without claiming or canceling', () => {
    const host = hostElement();
    const onStart = vi.fn();
    const onCancel = vi.fn();
    const action = panGesture(host, {
      axis: 'y',
      onStart,
      onCancel
    });

    host.dispatchEvent(pointer('pointerdown', 20, 20));
    window.dispatchEvent(pointer('pointermove', 120, 28));
    window.dispatchEvent(pointer('pointerup', 120, 28));

    expect(onStart).not.toHaveBeenCalled();
    expect(onCancel).not.toHaveBeenCalled();

    action.destroy();
  });

  it('cleans up window listeners on destroy', () => {
    const host = hostElement();
    const onStart = vi.fn();
    const onEnd = vi.fn();
    const action = panGesture(host, {
      axis: 'x',
      shouldClaim: (dx) => dx < 0,
      onStart,
      onEnd
    });

    host.dispatchEvent(pointer('pointerdown', 320));
    action.destroy();
    window.dispatchEvent(pointer('pointermove', 20));
    window.dispatchEvent(pointer('pointerup', 20));

    expect(onStart).not.toHaveBeenCalled();
    expect(onEnd).not.toHaveBeenCalled();
  });
});
