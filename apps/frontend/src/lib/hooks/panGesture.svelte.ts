/**
 * Generic single-axis pan-gesture primitive.
 *
 * Detects a press → optional long-press → drag claim → continuous update →
 * release-with-velocity flow on a single host element, and exposes the
 * decisions as plain callbacks. Specific use-sites (sidebar swipe, bottom-sheet
 * drag-to-dismiss, ...) wire these callbacks to their own state.
 *
 * Mouse/pen input uses pointer events. Touch input uses touch events so a
 * horizontal app gesture can keep running even on browsers that cancel pointer
 * events when native panning/back-navigation detection starts.
 *
 * Direction lock: we wait until movement reaches {@link DIRECTION_LOCK_PX} and
 * the primary axis dominates the perpendicular axis before claiming the
 * gesture. Perpendicular drags release the pointer without ever calling
 * `onStart`. The optional {@link PanGestureConfig.shouldClaim} predicate gives
 * call-sites a final say (e.g. reject "wrong direction" drags based on current
 * state).
 *
 * Move/release tracking is installed on `window` after press start so the
 * gesture can still claim if the contact leaves the host before the first
 * direction-locking move. Pointer capture is still deferred until claim so taps
 * and short presses bubble naturally; only confirmed drags lock the pointer.
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
  let touchId: number | null = null;
  let startX = 0;
  let startY = 0;
  let claimed = false;
  let captured = false;
  let samples: Sample[] = [];
  let longPressTimer: number | null = null;
  let trackingPointerWindow = false;
  let trackingTouchWindow = false;

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
    if (trackingPointerWindow) {
      window.removeEventListener('pointermove', onMove, true);
      window.removeEventListener('pointerup', onUp, true);
      window.removeEventListener('pointercancel', onCancel, true);
      trackingPointerWindow = false;
    }
    if (trackingTouchWindow) {
      window.removeEventListener('touchmove', onTouchMove, true);
      window.removeEventListener('touchend', onTouchEnd, true);
      window.removeEventListener('touchcancel', onTouchCancel, true);
      trackingTouchWindow = false;
    }
    clearLongPress();
    pointerId = null;
    touchId = null;
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

  function move(
    x: number,
    y: number,
    timeStamp: number,
    options: { pointerId?: number; preventDefault?: () => void } = {}
  ) {
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
      options.preventDefault?.();
      cfg.onStart?.();
      if (options.pointerId !== undefined) {
        node.setPointerCapture(options.pointerId);
        captured = true;
      }
    }

    if (claimed) options.preventDefault?.();
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

  function onDown(e: PointerEvent) {
    if (e.pointerType === 'touch') return;
    if (pointerId !== null || touchId !== null) return;
    if (cfg.enabled && !cfg.enabled()) return;
    pointerId = e.pointerId;
    begin(e.clientX, e.clientY, e.timeStamp);
    window.addEventListener('pointermove', onMove, true);
    window.addEventListener('pointerup', onUp, true);
    window.addEventListener('pointercancel', onCancel, true);
    trackingPointerWindow = true;
  }

  function onMove(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    move(e.clientX, e.clientY, e.timeStamp, { pointerId: e.pointerId });
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

  function touchById(touches: TouchList, id: number) {
    for (let i = 0; i < touches.length; i += 1) {
      const touch = touches.item(i);
      if (touch?.identifier === id) return touch;
    }
    return null;
  }

  function onTouchStart(e: TouchEvent) {
    if (pointerId !== null || touchId !== null) return;
    if (cfg.enabled && !cfg.enabled()) return;
    const touch = e.changedTouches.item(0);
    if (!touch) return;
    touchId = touch.identifier;
    begin(touch.clientX, touch.clientY, e.timeStamp);
    window.addEventListener('touchmove', onTouchMove, { capture: true, passive: false });
    window.addEventListener('touchend', onTouchEnd, true);
    window.addEventListener('touchcancel', onTouchCancel, true);
    trackingTouchWindow = true;
  }

  function onTouchMove(e: TouchEvent) {
    if (touchId === null) return;
    const touch = touchById(e.touches, touchId);
    if (!touch) return;
    move(touch.clientX, touch.clientY, e.timeStamp, {
      preventDefault: () => {
        if (e.cancelable) e.preventDefault();
      }
    });
  }

  function onTouchEnd(e: TouchEvent) {
    if (touchId === null) return;
    const touch = touchById(e.changedTouches, touchId);
    if (!touch) return;
    end(touch.clientX, touch.clientY);
  }

  function onTouchCancel(e: TouchEvent) {
    if (touchId === null) return;
    if (!touchById(e.changedTouches, touchId)) return;
    if (claimed) cfg.onCancel?.();
    reset();
  }

  node.addEventListener('pointerdown', onDown);
  node.addEventListener('touchstart', onTouchStart, { passive: true });

  return {
    update(next: PanGestureConfig) {
      cfg = next;
    },
    destroy() {
      reset();
      node.removeEventListener('pointerdown', onDown);
      node.removeEventListener('touchstart', onTouchStart);
    }
  };
}
