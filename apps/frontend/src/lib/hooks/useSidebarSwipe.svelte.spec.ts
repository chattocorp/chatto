import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { sidebarEdgeSwipe, sidebarSwipe } from './useSidebarSwipe.svelte';
import { sidebarNav } from '$lib/state/globals.svelte';

const originalElementsFromPoint = document.elementsFromPoint;

function resetSidebar() {
  sidebarNav.setMobile(false);
  if (!sidebarNav.isOpen) sidebarNav.toggle();
  sidebarNav.setMobile(true);
}

function makeEdgeGestureHost() {
  const edge = document.createElement('div');
  const underlying = document.createElement('button');

  edge.setPointerCapture = vi.fn();
  edge.releasePointerCapture = vi.fn();
  document.body.append(underlying, edge);

  Object.defineProperty(document, 'elementsFromPoint', {
    configurable: true,
    value: vi.fn(() => [edge, underlying])
  });

  return { edge, underlying };
}

function pointer(type: string, x: number, y = 24) {
  return new PointerEvent(type, {
    bubbles: true,
    cancelable: true,
    pointerId: 1,
    clientX: x,
    clientY: y
  });
}

describe('sidebarEdgeSwipe', () => {
  beforeEach(() => {
    resetSidebar();
  });

  afterEach(() => {
    Object.defineProperty(document, 'elementsFromPoint', {
      configurable: true,
      value: originalElementsFromPoint
    });
    document.body.replaceChildren();
  });

  it('does not synthesize taps into the content behind the edge target', () => {
    const { edge, underlying } = makeEdgeGestureHost();
    const onUnderlyingPointerDown = vi.fn();
    const onUnderlyingClick = vi.fn();
    const onWindowPointerDown = vi.fn();
    underlying.addEventListener('pointerdown', onUnderlyingPointerDown);
    underlying.addEventListener('click', onUnderlyingClick);
    window.addEventListener('pointerdown', onWindowPointerDown);

    const action = sidebarEdgeSwipe(edge);
    try {
      edge.dispatchEvent(pointer('pointerdown', 2));
      edge.dispatchEvent(pointer('pointerup', 2));

      expect(onUnderlyingPointerDown).not.toHaveBeenCalled();
      expect(onUnderlyingClick).not.toHaveBeenCalled();
      expect(onWindowPointerDown).not.toHaveBeenCalled();
      expect(sidebarNav.isOpen).toBe(false);
    } finally {
      window.removeEventListener('pointerdown', onWindowPointerDown);
      action.destroy();
    }
  });

  it('still opens the mobile sidebar on a rightward edge drag', () => {
    const { edge } = makeEdgeGestureHost();
    const action = sidebarEdgeSwipe(edge);

    edge.dispatchEvent(pointer('pointerdown', 2));
    edge.dispatchEvent(pointer('pointermove', 210));
    edge.dispatchEvent(pointer('pointerup', 210));

    expect(sidebarNav.isOpen).toBe(true);

    action.destroy();
  });

  it('still closes the mobile sidebar on a leftward drag', () => {
    const { edge } = makeEdgeGestureHost();
    sidebarNav.isOpen = true;
    const action = sidebarSwipe(edge);

    edge.dispatchEvent(pointer('pointerdown', 320));
    edge.dispatchEvent(pointer('pointermove', 0));
    edge.dispatchEvent(pointer('pointerup', 0));

    expect(sidebarNav.isOpen).toBe(false);

    action.destroy();
  });
});
