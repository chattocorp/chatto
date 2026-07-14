import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { sidebarSwipe } from './useSidebarSwipe.svelte';
import { sidebarNav } from '$lib/state/globals.svelte';

function resetSidebar() {
  sidebarNav.setMobile(false);
  if (!sidebarNav.isOpen) sidebarNav.toggle();
  sidebarNav.setMobile(true);
}

function makeGestureHost() {
  const host = document.createElement('main');
  const child = document.createElement('button');

  host.setPointerCapture = vi.fn();
  host.releasePointerCapture = vi.fn();
  host.append(child);
  document.body.append(host);

  return { host, child };
}

function pointer(type: string, x: number, y = 24) {
  return new PointerEvent(type, {
    bubbles: true,
    cancelable: true,
    composed: true,
    pointerId: 1,
    clientX: x,
    clientY: y
  });
}

function touch(type: string, x: number, y = 24) {
  const event = new Event(type, {
    bubbles: true,
    cancelable: true,
    composed: true
  }) as TouchEvent;
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

describe('sidebarSwipe', () => {
  beforeEach(() => {
    resetSidebar();
  });

  afterEach(() => {
    Object.defineProperty(document, 'fullscreenElement', {
      configurable: true,
      value: null
    });
    document.body.replaceChildren();
  });

  it('leaves taps on child controls to normal browser event flow', () => {
    const { host, child } = makeGestureHost();
    const onClick = vi.fn();
    child.addEventListener('click', onClick);
    const action = sidebarSwipe(host);

    child.dispatchEvent(pointer('pointerdown', 4));
    window.dispatchEvent(pointer('pointerup', 4));
    child.click();

    expect(onClick).toHaveBeenCalledOnce();
    expect(sidebarNav.isOpen).toBe(false);

    action.destroy();
  });

  it('opens the mobile sidebar from a rightward drag on app content', () => {
    const { host, child } = makeGestureHost();
    const action = sidebarSwipe(host);

    child.dispatchEvent(pointer('pointerdown', 100));
    window.dispatchEvent(pointer('pointermove', 310));
    window.dispatchEvent(pointer('pointerup', 310));

    expect(sidebarNav.isOpen).toBe(true);

    action.destroy();
  });

  it('ignores drags that start inside horizontally scrollable content', () => {
    const { host } = makeGestureHost();
    const scroller = document.createElement('div');
    const child = document.createElement('button');
    scroller.style.overflowX = 'auto';
    Object.defineProperty(scroller, 'clientWidth', { value: 100 });
    Object.defineProperty(scroller, 'scrollWidth', { value: 300 });
    scroller.append(child);
    host.append(scroller);
    const action = sidebarSwipe(host);

    child.dispatchEvent(pointer('pointerdown', 100));
    window.dispatchEvent(pointer('pointermove', 310));
    window.dispatchEvent(pointer('pointerup', 310));

    expect(sidebarNav.isOpen).toBe(false);
    expect(sidebarNav.dragOffset).toBeNull();

    action.destroy();
  });

  it('ignores rightward pointer drags that start on native form controls', () => {
    const { host } = makeGestureHost();
    const range = document.createElement('input');
    range.type = 'range';
    host.append(range);
    const action = sidebarSwipe(host);

    range.dispatchEvent(pointer('pointerdown', 100));
    window.dispatchEvent(pointer('pointermove', 310));
    window.dispatchEvent(pointer('pointerup', 310));

    expect(sidebarNav.isOpen).toBe(false);
    expect(sidebarNav.dragOffset).toBeNull();

    action.destroy();
  });

  it('ignores leftward touch drags inside shadow-DOM media controls', () => {
    const { host } = makeGestureHost();
    const mediaPlayer = document.createElement('media-player');
    const control = document.createElement('button');
    mediaPlayer.attachShadow({ mode: 'open' }).append(control);
    host.append(mediaPlayer);
    sidebarNav.isOpen = true;
    const action = sidebarSwipe(host);

    control.dispatchEvent(touch('touchstart', 320));
    const move = touch('touchmove', 0);
    window.dispatchEvent(move);
    window.dispatchEvent(touch('touchend', 0));

    expect(move.defaultPrevented).toBe(false);
    expect(sidebarNav.isOpen).toBe(true);
    expect(sidebarNav.dragOffset).toBeNull();

    action.destroy();
  });

  it('ignores gestures that start inside dialogs and popovers', () => {
    const { host } = makeGestureHost();
    const dialog = document.createElement('dialog');
    const dialogControl = document.createElement('button');
    dialog.append(dialogControl);
    const popover = document.createElement('div');
    const popoverControl = document.createElement('button');
    popover.setAttribute('popover', 'auto');
    popover.append(popoverControl);
    host.append(dialog, popover);
    const action = sidebarSwipe(host);

    dialogControl.dispatchEvent(pointer('pointerdown', 100));
    window.dispatchEvent(pointer('pointermove', 310));
    window.dispatchEvent(pointer('pointerup', 310));
    popoverControl.dispatchEvent(pointer('pointerdown', 100));
    window.dispatchEvent(pointer('pointermove', 310));
    window.dispatchEvent(pointer('pointerup', 310));

    expect(sidebarNav.isOpen).toBe(false);
    expect(sidebarNav.dragOffset).toBeNull();

    action.destroy();
  });

  it('ignores gestures that start inside the fullscreen top layer', () => {
    const { host } = makeGestureHost();
    const fullscreenSurface = document.createElement('div');
    const control = document.createElement('button');
    fullscreenSurface.append(control);
    host.append(fullscreenSurface);
    Object.defineProperty(document, 'fullscreenElement', {
      configurable: true,
      value: fullscreenSurface
    });
    const action = sidebarSwipe(host);

    control.dispatchEvent(pointer('pointerdown', 100));
    window.dispatchEvent(pointer('pointermove', 310));
    window.dispatchEvent(pointer('pointerup', 310));

    expect(sidebarNav.isOpen).toBe(false);
    expect(sidebarNav.dragOffset).toBeNull();

    action.destroy();
  });

  it('closes the mobile sidebar on a leftward touch drag', () => {
    const { host, child } = makeGestureHost();
    sidebarNav.isOpen = true;
    const action = sidebarSwipe(host);

    child.dispatchEvent(touch('touchstart', 320));
    const move = touch('touchmove', 0);
    window.dispatchEvent(move);
    window.dispatchEvent(touch('touchend', 0));

    expect(move.defaultPrevented).toBe(true);
    expect(sidebarNav.isOpen).toBe(false);

    action.destroy();
  });
});
