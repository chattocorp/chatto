/**
 * Generic single-axis pan-gesture primitive.
 *
 * Detects a press → optional long-press → drag claim → continuous update →
 * release-with-velocity flow on a single host element, and exposes the
 * decisions as plain callbacks. Specific use-sites (sidebar swipe, bottom-sheet
 * drag-to-dismiss, ...) wire these callbacks to their own state. Pointer events
 * are primary; mouse events are a fallback for desktop/trackpad drags in
 * browser contexts that do not produce pointer moves for synthetic mouse input.
 *
 * The host element MUST have `touch-action: none` (or at minimum block native
 * panning along the chosen axis) — otherwise the browser fires `pointercancel`
 * once it decides the drag is a scroll/back-navigation gesture and the slide
 * aborts mid-way.
 *
 * Direction lock: we wait until movement reaches {@link DIRECTION_LOCK_PX} and
 * the primary axis dominates the perpendicular axis before claiming the
 * gesture. Perpendicular drags release the pointer without ever calling
 * `onStart`. The optional {@link PanGestureConfig.shouldClaim} predicate gives
 * call-sites a final say (e.g. reject "wrong direction" drags based on current
 * state).
 *
 * Pointer capture is deferred until claim so taps and short presses bubble
 * naturally; only confirmed drags lock the pointer.
 */

const DIRECTION_LOCK_PX = 8;
const VELOCITY_SAMPLE_MS = 100;
/** Hold time before a stationary press is reported via `onLongPress`. */
const LONG_PRESS_MS = 500;
/** Movement (px) that cancels the pending long-press timer. */
const LONG_PRESS_CANCEL_PX = 4;

export type PanGestureConfig = {
  /** The axis the gesture tracks. The other axis is treated as perpendicular. */
  axis: 'x' | 'y';
  /** Optional gate evaluated on every `pointerdown`; if it returns false, the
   *  press is ignored entirely. */
  enabled?: () => boolean;
  /** Final claim predicate, called once the direction lock has fired. Receives
   *  the primary-axis delta (signed). Return false to abandon the gesture. */
  shouldClaim?: (delta: number) => boolean;
  /** Fired once when the gesture is claimed. */
  onStart?: () => void;
  /** Fired on every move while claimed. `delta` is signed primary-axis px. */
  onUpdate?: (delta: number) => void;
  /** Fired on release. `velocity` is signed primary-axis px/ms (sampled over
   *  the last {@link VELOCITY_SAMPLE_MS}). */
  onEnd?: (delta: number, velocity: number) => void;
  /** Fired when the browser cancels a claimed gesture mid-drag. */
  onCancel?: () => void;
  /** Fired on release without meaningful movement. */
  onTap?: (x: number, y: number) => void;
  /** Fired when the press is held still for {@link LONG_PRESS_MS}. */
  onLongPress?: (x: number, y: number) => void;
};

type Sample = { v: number; t: number };

export function panGesture(node: HTMLElement, config: PanGestureConfig) {
  let cfg = config;
  let pointerId: number | null = null;
  let mouseActive = false;
  let startX = 0;
  let startY = 0;
  let claimed = false;
  let captured = false;
  let samples: Sample[] = [];
  let longPressTimer: number | null = null;

  const primary = (x: number, y: number) => (cfg.axis === 'x' ? x : y);
  const secondary = (x: number, y: number) => (cfg.axis === 'x' ? y : x);

  function clearLongPress() {
    if (longPressTimer !== null) {
      window.clearTimeout(longPressTimer);
      longPressTimer = null;
    }
  }

  function reset() {
    if (pointerId !== null && captured) node.releasePointerCapture?.(pointerId);
    if (mouseActive) {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    }
    clearLongPress();
    pointerId = null;
    mouseActive = false;
    claimed = false;
    captured = false;
    samples = [];
  }

  function begin(x: number, y: number, timeStamp: number) {
    startX = x;
    startY = y;
    claimed = false;
    captured = false;
    samples = [{ v: primary(x, y), t: timeStamp }];
    if (cfg.onLongPress) {
      longPressTimer = window.setTimeout(() => {
        longPressTimer = null;
        cfg.onLongPress?.(x, y);
        reset();
      }, LONG_PRESS_MS);
    }
  }

  function onDown(e: PointerEvent) {
    if (pointerId !== null || mouseActive) return;
    if (cfg.enabled && !cfg.enabled()) return;
    pointerId = e.pointerId;
    begin(e.clientX, e.clientY, e.timeStamp);
  }

  function move(x: number, y: number, timeStamp: number, capturePointerId?: number) {
    const dPrim = primary(x, y) - primary(startX, startY);
    const dSec = secondary(x, y) - secondary(startX, startY);

    if (Math.abs(dPrim) >= LONG_PRESS_CANCEL_PX || Math.abs(dSec) >= LONG_PRESS_CANCEL_PX) {
      clearLongPress();
    }

    if (!claimed) {
      if (Math.abs(dPrim) < DIRECTION_LOCK_PX && Math.abs(dSec) < DIRECTION_LOCK_PX) return;
      if (Math.abs(dSec) > Math.abs(dPrim)) {
        // Perpendicular movement won — release the pointer.
        reset();
        return;
      }
      if (cfg.shouldClaim && !cfg.shouldClaim(dPrim)) {
        reset();
        return;
      }
      claimed = true;
      cfg.onStart?.();
      if (capturePointerId !== undefined) {
        node.setPointerCapture(capturePointerId);
        captured = true;
      }
    }

    cfg.onUpdate?.(dPrim);
    samples.push({ v: primary(x, y), t: timeStamp });
    const cutoff = timeStamp - VELOCITY_SAMPLE_MS;
    while (samples.length > 2 && samples[0].t < cutoff) samples.shift();
  }

  function end(x: number, y: number) {
    if (!claimed) {
      const movedFar =
        Math.abs(x - startX) >= LONG_PRESS_CANCEL_PX ||
        Math.abs(y - startY) >= LONG_PRESS_CANCEL_PX;
      if (!movedFar) cfg.onTap?.(x, y);
      reset();
      return;
    }
    const dPrim = primary(x, y) - primary(startX, startY);
    const last = samples[samples.length - 1];
    const first = samples[0];
    const dt = last.t - first.t;
    const v = dt > 0 ? (last.v - first.v) / dt : 0;
    cfg.onEnd?.(dPrim, v);
    reset();
  }

  function onMove(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    move(e.clientX, e.clientY, e.timeStamp, e.pointerId);
  }

  function onUp(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    end(e.clientX, e.clientY);
  }

  function onCancel(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    if (claimed) cfg.onCancel?.();
    reset();
  }

  function onMouseDown(e: MouseEvent) {
    if (pointerId !== null || mouseActive) return;
    if (e.button !== 0) return;
    if (cfg.enabled && !cfg.enabled()) return;
    mouseActive = true;
    begin(e.clientX, e.clientY, e.timeStamp);
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  function onMouseMove(e: MouseEvent) {
    if (!mouseActive) return;
    move(e.clientX, e.clientY, e.timeStamp);
  }

  function onMouseUp(e: MouseEvent) {
    if (!mouseActive) return;
    end(e.clientX, e.clientY);
  }

  node.addEventListener('pointerdown', onDown);
  node.addEventListener('pointermove', onMove);
  node.addEventListener('pointerup', onUp);
  node.addEventListener('pointercancel', onCancel);
  node.addEventListener('mousedown', onMouseDown);

  return {
    update(next: PanGestureConfig) {
      cfg = next;
    },
    destroy() {
      reset();
      node.removeEventListener('pointerdown', onDown);
      node.removeEventListener('pointermove', onMove);
      node.removeEventListener('pointerup', onUp);
      node.removeEventListener('pointercancel', onCancel);
      node.removeEventListener('mousedown', onMouseDown);
    }
  };
}
