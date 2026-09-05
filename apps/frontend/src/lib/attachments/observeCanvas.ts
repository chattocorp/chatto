type CanvasObservers = {
  /** Resize the drawing buffer or schedule a new frame. */
  onResize: () => void;
  /** Start or pause drawing after tab or intersection visibility changes. */
  onVisibilityChange: () => void;
  /** Apply the current motion preference and redraw. */
  onMotionChange: () => void;
  /** Use a containing control when it defines the visible drawing surface. */
  visibilityTarget?: Element;
};

/**
 * Observe a mounted canvas. Callbacks run on browser events, not during setup.
 * The caller owns the initial draw and cancels scheduled work on detach.
 */
export function observeCanvas(
  canvas: HTMLCanvasElement,
  { onResize, onVisibilityChange, onMotionChange, visibilityTarget = canvas }: CanvasObservers
) {
  const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
  let active = true;
  let inViewport = true;
  const resize = new ResizeObserver(() => {
    if (active) onResize();
  });
  const intersection = new IntersectionObserver(([entry]) => {
    if (!active) return;
    inViewport = entry?.isIntersecting ?? true;
    onVisibilityChange();
  });
  const visibilityChanged = () => {
    if (active) onVisibilityChange();
  };
  const motionChanged = () => {
    if (active) onMotionChange();
  };
  resize.observe(canvas);
  intersection.observe(visibilityTarget);
  document.addEventListener('visibilitychange', visibilityChanged);
  motion.addEventListener('change', motionChanged);

  return {
    /** Visibility is optimistic until the first intersection notification. */
    get visible() {
      return active && inViewport && !document.hidden;
    },
    get reducedMotion() {
      return motion.matches;
    },
    /** Disconnect observers and ignore notifications already queued by the browser. */
    destroy() {
      active = false;
      resize.disconnect();
      intersection.disconnect();
      document.removeEventListener('visibilitychange', visibilityChanged);
      motion.removeEventListener('change', motionChanged);
    }
  };
}
