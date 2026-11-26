import { sidebarNav } from '$lib/state/globals.svelte';
import { fromInlineEndOffset, toInlineEndDelta } from '$lib/i18n/direction';
import { panGesture, type PanGestureConfig } from './panGesture.svelte';

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

/** Both mobile navigation gestures yield to browser edges and nested controls. */
function canStartMobileSwipe(event: PointerEvent | TouchEvent, node: HTMLElement): boolean {
  return (
    event.type === 'touchstart' &&
    sidebarNav.isMobile &&
    startsAwayFromViewportEdge(event) &&
    !startsInsideExcludedSurface(event, node)
  );
}

type SwipeTarget = Pick<PanGestureConfig, 'onStart'> &
  Required<Pick<PanGestureConfig, 'shouldClaim' | 'onUpdate' | 'onEnd' | 'onCancel'>>;

/** The shell owns one touch sequence, including swipes started outside the thread. */
const threadOverlays = new WeakMap<HTMLElement, SwipeTarget>();

/** Register the visible overlay as the shell's first destination for a back swipe. */
export function threadDismissSwipe(node: HTMLElement, close: () => void) {
  const shell = node.closest<HTMLElement>('[data-mobile-swipe-host]');
  if (!shell) return;
  const reset = () => node.style.removeProperty('translate');
  const drag: SwipeTarget = {
    shouldClaim: (dx) => toInlineEndDelta(dx) > 0,
    onUpdate: (dx) => {
      const offset = Math.max(0, toInlineEndDelta(dx));
      node.style.translate = `${fromInlineEndOffset(offset)}px`;
    },
    onEnd: (dx, vx) => {
      const distance = toInlineEndDelta(dx);
      if (distance >= node.clientWidth / 2 || (distance >= 32 && toInlineEndDelta(vx) > 0.5)) {
        // Finish sliding from the finger's release position during the route outro.
        node.dataset.swipeDismissed = '';
        node.style.translate = `${fromInlineEndOffset(100)}%`;
        close();
      } else {
        reset();
      }
    },
    onCancel: reset
  };
  threadOverlays.set(shell, drag);
  return () => {
    if (threadOverlays.get(shell) === drag) threadOverlays.delete(shell);
    delete node.dataset.swipeDismissed;
    reset();
  };
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
export function sidebarSwipe(node: HTMLElement) {
  const sidebar: SwipeTarget = {
    shouldClaim: (dx) => (sidebarNav.isOpen ? toInlineEndDelta(dx) < 0 : toInlineEndDelta(dx) > 0),
    onStart: () => sidebarNav.startDrag(),
    onUpdate: (dx) => sidebarNav.updateDrag(toInlineEndDelta(dx)),
    onEnd: (_dx, vx) => sidebarNav.endDrag(toInlineEndDelta(vx)),
    onCancel: () => sidebarNav.endDrag(0)
  };
  let owner = sidebar;
  const stillMounted = () => owner === sidebar || threadOverlays.get(node) === owner;
  return panGesture(node, {
    axis: 'x',
    enabled: (event) => {
      if (!canStartMobileSwipe(event, node)) return false;
      owner = sidebarNav.isOpen ? sidebar : (threadOverlays.get(node) ?? sidebar);
      return true;
    },
    shouldClaim: (dx) => stillMounted() && owner.shouldClaim(dx),
    onStart: () => owner.onStart?.(),
    onUpdate: (dx) => {
      if (stillMounted()) owner.onUpdate(dx);
    },
    onEnd: (dx, vx) => {
      if (stillMounted()) owner.onEnd(dx, vx);
    },
    onCancel: () => owner.onCancel()
  });
}
