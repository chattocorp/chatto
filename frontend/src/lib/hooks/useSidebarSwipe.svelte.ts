import { sidebarNav, SIDEBAR_PANEL_WIDTH_PX } from '$lib/state/globals.svelte';
import { panGesture } from './panGesture.svelte';

/**
 * Svelte action: drives the mobile sidebar's open/close state from a
 * horizontal pointer drag on the host element.
 *
 * Ignored on desktop (gated by `sidebarNav.isMobile`). When closed, only
 * rightward drags claim; when open, only leftward drags claim. Taps and
 * long-presses are forwarded to the element directly underneath the host so
 * dedicated overlay zones (the 24px edge strip, the backdrop) still let
 * underlying content receive clicks and context menus.
 *
 * The host MUST have `touch-action: none` along the X axis — without it,
 * Chrome / iOS Safari fire `pointercancel` ~8px in when they decide a touch
 * is a back-navigation / text-selection drag.
 */
export function sidebarSwipe(node: HTMLElement) {
  function elementBelow(x: number, y: number) {
    return document
      .elementsFromPoint(x, y)
      .find((el) => el !== node && !node.contains(el));
  }

  function forwardLongPress(x: number, y: number) {
    elementBelow(x, y)?.dispatchEvent(
      new MouseEvent('contextmenu', {
        bubbles: true,
        cancelable: true,
        clientX: x,
        clientY: y,
        button: 2
      })
    );
  }

  function forwardTap(x: number, y: number) {
    const target = elementBelow(x, y);
    if (!target) return;
    const opts: MouseEventInit = {
      bubbles: true,
      cancelable: true,
      composed: true,
      clientX: x,
      clientY: y,
      button: 0
    };
    target.dispatchEvent(new PointerEvent('pointerdown', opts));
    target.dispatchEvent(new MouseEvent('mousedown', opts));
    target.dispatchEvent(new PointerEvent('pointerup', opts));
    target.dispatchEvent(new MouseEvent('mouseup', opts));
    target.dispatchEvent(new MouseEvent('click', opts));
  }

  return panGesture(node, {
    axis: 'x',
    enabled: () => sidebarNav.isMobile,
    shouldClaim: (dx) => (sidebarNav.isOpen ? dx < 0 : dx > 0),
    onStart: () => sidebarNav.startDrag(),
    onUpdate: (dx) => sidebarNav.updateDrag(dx),
    onEnd: (_dx, vx) => sidebarNav.endDrag(vx),
    onCancel: () => sidebarNav.endDrag(0),
    onTap: forwardTap,
    onLongPress: forwardLongPress
  });
}

export { SIDEBAR_PANEL_WIDTH_PX };
