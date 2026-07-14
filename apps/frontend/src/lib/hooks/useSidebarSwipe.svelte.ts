import { sidebarNav, SIDEBAR_PANEL_WIDTH_PX } from '$lib/state/globals.svelte';
import { panGesture } from './panGesture.svelte';

/**
 * Svelte action: drives the mobile sidebar's open/close state from a
 * horizontal pointer drag anywhere inside the app-shell host.
 *
 * Ignored on desktop (gated by `sidebarNav.isMobile`). When closed, only
 * rightward drags claim; when open, only leftward drags claim. Gestures that
 * begin inside horizontally scrollable content are ignored so galleries and
 * wide tables retain their native scrolling. Dialogs, popovers, fullscreen
 * surfaces, form fields, media controls, and elements marked with
 * `data-sidebar-swipe-ignore` also retain their own gestures. Unclaimed taps,
 * long presses, and vertical movement continue through normal browser event
 * flow.
 */
function startsInsideExcludedSurface(event: PointerEvent | TouchEvent, host: HTMLElement): boolean {
  for (const pathEntry of event.composedPath()) {
    if (pathEntry === host) return false;
    if (pathEntry === document.fullscreenElement) return true;
    if (!(pathEntry instanceof HTMLElement)) continue;

    const element = pathEntry;
    if (
      element instanceof HTMLDialogElement ||
      element.hasAttribute('popover') ||
      element.matches(
        'input, textarea, select, [contenteditable]:not([contenteditable="false"]), audio, video, media-player, [data-sidebar-swipe-ignore]'
      )
    ) {
      return true;
    }

    const { overflowX } = getComputedStyle(element);
    if (
      (overflowX === 'auto' || overflowX === 'scroll') &&
      element.scrollWidth > element.clientWidth
    ) {
      return true;
    }
  }

  return false;
}

export function sidebarSwipe(node: HTMLElement) {
  return panGesture(node, {
    axis: 'x',
    enabled: (event) => sidebarNav.isMobile && !startsInsideExcludedSurface(event, node),
    shouldClaim: (dx) => (sidebarNav.isOpen ? dx < 0 : dx > 0),
    onStart: () => sidebarNav.startDrag(),
    onUpdate: (dx) => sidebarNav.updateDrag(dx),
    onEnd: (_dx, vx) => sidebarNav.endDrag(vx),
    onCancel: () => sidebarNav.endDrag(0)
  });
}

export { SIDEBAR_PANEL_WIDTH_PX };
