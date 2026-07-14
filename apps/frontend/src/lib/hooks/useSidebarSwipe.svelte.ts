import { sidebarNav, SIDEBAR_PANEL_WIDTH_PX } from '$lib/state/globals.svelte';
import { panGesture } from './panGesture.svelte';

/**
 * Svelte action: drives the mobile sidebar's open/close state from a
 * horizontal pointer drag anywhere inside the app-shell host.
 *
 * Ignored on desktop (gated by `sidebarNav.isMobile`). When closed, only
 * rightward drags claim; when open, only leftward drags claim. Gestures that
 * begin inside horizontally scrollable content are ignored so galleries and
 * wide tables retain their native scrolling. Unclaimed taps, long presses,
 * and vertical movement continue through normal browser event flow.
 */
function startsInsideHorizontalScroller(target: EventTarget | null, host: HTMLElement): boolean {
  let element = target instanceof Element ? target : null;

  while (element && element !== host) {
    if (element instanceof HTMLElement) {
      const { overflowX } = getComputedStyle(element);
      if (
        (overflowX === 'auto' || overflowX === 'scroll') &&
        element.scrollWidth > element.clientWidth
      ) {
        return true;
      }
    }
    element = element.parentElement;
  }

  return false;
}

export function sidebarSwipe(node: HTMLElement) {
  return panGesture(node, {
    axis: 'x',
    enabled: (target) => sidebarNav.isMobile && !startsInsideHorizontalScroller(target, node),
    shouldClaim: (dx) => (sidebarNav.isOpen ? dx < 0 : dx > 0),
    onStart: () => sidebarNav.startDrag(),
    onUpdate: (dx) => sidebarNav.updateDrag(dx),
    onEnd: (_dx, vx) => sidebarNav.endDrag(vx),
    onCancel: () => sidebarNav.endDrag(0)
  });
}

export { SIDEBAR_PANEL_WIDTH_PX };
