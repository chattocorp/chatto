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

function pointer(type: string, x: number, pointerType: string) {
  return new PointerEvent(type, {
    bubbles: true,
    cancelable: true,
    composed: true,
    pointerId: 1,
    pointerType,
    clientX: x,
    clientY: 24
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
    document.documentElement.dir = 'ltr';
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

    child.dispatchEvent(touch('touchstart', 4));
    window.dispatchEvent(touch('touchend', 4));
    child.click();

    expect(onClick).toHaveBeenCalledOnce();
    expect(sidebarNav.isOpen).toBe(false);

    action.destroy();
  });

  it.each(['mouse', 'pen'])('ignores %s drags with an open or closed drawer', (pointerType) => {
    const { host, child } = makeGestureHost();
    const action = sidebarSwipe(host);

    for (const open of [false, true]) {
      sidebarNav.isOpen = open;
      const start = open ? 320 : 0;
      const end = open ? 0 : 320;
      child.dispatchEvent(pointer('pointerdown', start, pointerType));
      window.dispatchEvent(pointer('pointermove', end, pointerType));

      expect(sidebarNav.dragOffset).toBeNull();
      expect(host.setPointerCapture).not.toHaveBeenCalled();

      window.dispatchEvent(pointer('pointerup', end, pointerType));
      expect(sidebarNav.isOpen).toBe(open);
      expect(sidebarNav.dragOffset).toBeNull();
    }

    action.destroy();
  });

  it('ignores touch drags in the wide layout', () => {
    sidebarNav.setMobile(false);
    const { host, child } = makeGestureHost();
    const action = sidebarSwipe(host);

    child.dispatchEvent(touch('touchstart', 320));
    const move = touch('touchmove', 0);
    window.dispatchEvent(move);
    expect(sidebarNav.dragOffset).toBeNull();
    expect(move.defaultPrevented).toBe(false);
    window.dispatchEvent(touch('touchend', 0));
    expect(sidebarNav.isOpen).toBe(true);

    action.destroy();
  });

  it('opens the mobile sidebar from a rightward drag on app content', () => {
    const { host, child } = makeGestureHost();
    const action = sidebarSwipe(host);

    child.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));

    expect(sidebarNav.isOpen).toBe(true);

    action.destroy();
  });

  it.each(['ltr', 'rtl'])('leaves viewport-edge swipes to the browser in %s', (direction) => {
    document.documentElement.dir = direction;
    const { host, child } = makeGestureHost();
    const action = sidebarSwipe(host);

    for (const open of [false, true]) {
      for (const startX of [0, 24, window.innerWidth - 24, window.innerWidth - 1]) {
        sidebarNav.isOpen = open;
        const endX = startX < window.innerWidth / 2 ? startX + 210 : startX - 210;
        const start = touch('touchstart', startX);
        child.dispatchEvent(start);
        const move = touch('touchmove', endX);
        window.dispatchEvent(move);

        expect(sidebarNav.dragOffset).toBeNull();
        expect(start.defaultPrevented).toBe(false);
        expect(move.defaultPrevented).toBe(false);

        window.dispatchEvent(touch('touchend', endX));
        expect(sidebarNav.isOpen).toBe(open);
      }
    }

    action.destroy();
  });

  it.each(['ltr', 'rtl'])('opens from just inside the edge exclusion in %s', (direction) => {
    document.documentElement.dir = direction;
    const { host, child } = makeGestureHost();
    const action = sidebarSwipe(host);
    const startX = direction === 'ltr' ? 25 : window.innerWidth - 25;
    const endX = startX + (direction === 'ltr' ? 210 : -210);

    child.dispatchEvent(touch('touchstart', startX));
    const move = touch('touchmove', endX);
    window.dispatchEvent(move);
    expect(move.defaultPrevented).toBe(true);
    window.dispatchEvent(touch('touchend', endX));
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

    child.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));

    expect(sidebarNav.isOpen).toBe(false);
    expect(sidebarNav.dragOffset).toBeNull();

    action.destroy();
  });

  it('ignores rightward touch drags that start on native form controls', () => {
    const { host } = makeGestureHost();
    const range = document.createElement('input');
    range.type = 'range';
    host.append(range);
    const action = sidebarSwipe(host);

    range.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));

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

    dialogControl.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));
    popoverControl.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));

    expect(sidebarNav.isOpen).toBe(false);
    expect(sidebarNav.dragOffset).toBeNull();

    action.destroy();
  });

  it('ignores gestures inside modal dialog fallbacks that are not in the top layer', () => {
    const { host } = makeGestureHost();
    const modal = document.createElement('div');
    const control = document.createElement('button');
    modal.setAttribute('role', 'dialog');
    modal.setAttribute('aria-modal', 'true');
    modal.append(control);
    host.append(modal);
    const action = sidebarSwipe(host);

    control.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));

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

    control.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));

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

  it('opens and closes the mobile sidebar with mirrored RTL drags', () => {
    document.documentElement.dir = 'rtl';
    const { host, child } = makeGestureHost();
    const action = sidebarSwipe(host);

    child.dispatchEvent(touch('touchstart', 310));
    window.dispatchEvent(touch('touchmove', 100));
    window.dispatchEvent(touch('touchend', 100));
    expect(sidebarNav.isOpen).toBe(true);

    child.dispatchEvent(touch('touchstart', 100));
    window.dispatchEvent(touch('touchmove', 310));
    window.dispatchEvent(touch('touchend', 310));
    expect(sidebarNav.isOpen).toBe(false);

    action.destroy();
  });
});
