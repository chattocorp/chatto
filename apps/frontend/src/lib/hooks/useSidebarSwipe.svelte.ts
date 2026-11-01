import { sidebarNav } from '$lib/state/globals.svelte';
import { toInlineEndDelta } from '$lib/i18n/direction';
import { panGesture } from './panGesture.svelte';

/** Keep physical viewport edges available for browser back/forward gestures. */
const BROWSER_GESTURE_EDGE_PX = 24;

/** Reject edge starts before the sidebar can claim or animate a touch drag. */
function startsAwayFromViewportEdge(event: PointerEvent | TouchEvent): boolean {
  if (!('changedTouches' in event)) return false;
  const touch = event.changedTouches.item(0);
  return (
    touch !== null &&
    touch.clientX > BROWSER_GESTURE_EDGE_PX &&
    touch.clientX < window.innerWidth - BROWSER_GESTURE_EDGE_PX
  );
}

/**
 * Svelte action: drives the mobile sidebar's open/close state from a
 * horizontal touch drag inside the app-shell host, except within 24 CSS pixels
 * of either physical viewport edge. Edge starts are left to browser navigation,
 * regardless of text direction or whether the sidebar is open.
 *
 * Mouse and pen input are ignored, including on touch-capable devices.
 * Touch gestures require the narrow drawer layout (`sidebarNav.isMobile`). When closed, only
 * drags towards the inline end claim; when open, only drags towards the inline
 * start claim. Gestures that
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
        'input, textarea, select, [contenteditable]:not([contenteditable="false"]), audio, video, media-player, [role="dialog"][aria-modal="true"], [data-sidebar-swipe-ignore]'
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
    enabled: (event) =>
      event.type === 'touchstart' &&
      sidebarNav.isMobile &&
      startsAwayFromViewportEdge(event) &&
      !startsInsideExcludedSurface(event, node),
    shouldClaim: (dx) => (sidebarNav.isOpen ? toInlineEndDelta(dx) < 0 : toInlineEndDelta(dx) > 0),
    onStart: () => sidebarNav.startDrag(),
    onUpdate: (dx) => sidebarNav.updateDrag(toInlineEndDelta(dx)),
    onEnd: (_dx, vx) => sidebarNav.endDrag(toInlineEndDelta(vx)),
    onCancel: () => sidebarNav.endDrag(0)
  });
}
