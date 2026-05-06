import { sidebarNav, SIDEBAR_PANEL_WIDTH_PX } from '$lib/state/globals.svelte';

const DIRECTION_LOCK_PX = 8;
const VELOCITY_SAMPLE_MS = 100;
/** Hold time before a stationary touch is forwarded as a contextmenu event. */
const LONG_PRESS_MS = 500;
/** Movement (px) that cancels the long-press timer. */
const LONG_PRESS_CANCEL_PX = 4;

type Sample = { x: number; t: number };

/**
 * Svelte action: attaches a horizontal swipe handler to an element that
 * drives the mobile sidebar's open/close state.
 *
 * The host element MUST have `touch-action: none` (or at least block
 * horizontal browser gestures) — otherwise Chrome / iOS Safari fire
 * `pointercancel` once they decide the drag is a navigation/selection
 * gesture, and the slide aborts mid-way.
 *
 * Direction lock: we wait until movement reaches {@link DIRECTION_LOCK_PX}
 * and X dominates Y before claiming the gesture; vertical drags release the
 * pointer without ever calling `startDrag()`.
 *
 *   <div use:sidebarSwipe class="touch-none ..." />
 */
export function sidebarSwipe(node: HTMLElement) {
  let pointerId: number | null = null;
  let startX = 0;
  let startY = 0;
  let claimed = false;
  let captured = false;
  let baselineOpen = false;
  let samples: Sample[] = [];
  let longPressTimer: number | null = null;

  function reset() {
    if (pointerId !== null && captured) {
      node.releasePointerCapture?.(pointerId);
    }
    clearLongPress();
    pointerId = null;
    claimed = false;
    captured = false;
    samples = [];
  }

  function clearLongPress() {
    if (longPressTimer !== null) {
      window.clearTimeout(longPressTimer);
      longPressTimer = null;
    }
  }

  /**
   * Find the topmost interactive element underneath this swipe surface at
   * (x, y), skipping the surface itself and anything inside it.
   */
  function elementBelow(x: number, y: number): Element | undefined {
    return document
      .elementsFromPoint(x, y)
      .find((el) => el !== node && !node.contains(el));
  }

  /**
   * Forward a synthetic contextmenu event to the element below. Used when the
   * user taps-and-holds without moving — preserves long-press / context-menu
   * UX on content that the gesture surface visually overlaps (avatars, etc.).
   */
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

  /**
   * Forward a synthetic click sequence to the element below. Used when the
   * user taps the gesture surface without dragging — preserves click UX on
   * underlying interactive elements (back buttons, links, etc.) that happen
   * to fall inside the gesture surface's hit-test area.
   */
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

  function onDown(e: PointerEvent) {
    if (pointerId !== null) return;
    if (!sidebarNav.isMobile) return;
    pointerId = e.pointerId;
    startX = e.clientX;
    startY = e.clientY;
    baselineOpen = sidebarNav.isOpen;
    claimed = false;
    captured = false;
    samples = [{ x: e.clientX, t: e.timeStamp }];
    longPressTimer = window.setTimeout(() => {
      longPressTimer = null;
      forwardLongPress(e.clientX, e.clientY);
      reset();
    }, LONG_PRESS_MS);
  }

  function onMove(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    const dx = e.clientX - startX;
    const dy = e.clientY - startY;

    // Any meaningful movement cancels the long-press hand-off.
    if (Math.abs(dx) >= LONG_PRESS_CANCEL_PX || Math.abs(dy) >= LONG_PRESS_CANCEL_PX) {
      clearLongPress();
    }

    if (!claimed) {
      if (Math.abs(dx) < DIRECTION_LOCK_PX && Math.abs(dy) < DIRECTION_LOCK_PX) return;
      if (Math.abs(dy) > Math.abs(dx)) {
        // Vertical movement won — bow out.
        reset();
        return;
      }
      // Reject drags in the wrong direction for the current state:
      // closed → must drag right; open → must drag left.
      if (baselineOpen ? dx > 0 : dx < 0) {
        reset();
        return;
      }
      claimed = true;
      sidebarNav.startDrag();
      // Capture only once we've claimed the gesture so taps and short presses
      // can still bubble / be forwarded to the underlying content.
      node.setPointerCapture(e.pointerId);
      captured = true;
    }

    sidebarNav.updateDrag(dx);
    samples.push({ x: e.clientX, t: e.timeStamp });
    const cutoff = e.timeStamp - VELOCITY_SAMPLE_MS;
    while (samples.length > 2 && samples[0].t < cutoff) samples.shift();
  }

  function onUp(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    if (!claimed) {
      // Tap (movement didn't cross the swipe threshold). Forward as a click so
      // taps on underlying interactive content (back buttons, links, etc.)
      // still work even though the gesture surface is on top. Note: if a
      // long-press handoff already fired, `reset()` cleared `pointerId`, so
      // we'd have early-returned above.
      const movedFar =
        Math.abs(e.clientX - startX) >= LONG_PRESS_CANCEL_PX ||
        Math.abs(e.clientY - startY) >= LONG_PRESS_CANCEL_PX;
      if (!movedFar) {
        forwardTap(e.clientX, e.clientY);
      }
      reset();
      return;
    }
    const last = samples[samples.length - 1];
    const first = samples[0];
    const dt = last.t - first.t;
    const vx = dt > 0 ? (last.x - first.x) / dt : 0;
    sidebarNav.endDrag(vx);
    reset();
  }

  function onCancel(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    if (claimed) sidebarNav.endDrag(0);
    reset();
  }

  node.addEventListener('pointerdown', onDown);
  node.addEventListener('pointermove', onMove);
  node.addEventListener('pointerup', onUp);
  node.addEventListener('pointercancel', onCancel);

  return {
    destroy() {
      clearLongPress();
      node.removeEventListener('pointerdown', onDown);
      node.removeEventListener('pointermove', onMove);
      node.removeEventListener('pointerup', onUp);
      node.removeEventListener('pointercancel', onCancel);
    }
  };
}

export { SIDEBAR_PANEL_WIDTH_PX };
