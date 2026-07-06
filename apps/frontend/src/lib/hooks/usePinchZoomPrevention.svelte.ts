/**
 * Prevents pinch-to-zoom on trackpads via wheel and gesture events.
 * Call during component initialization — sets up listeners in an $effect.
 */
export function usePinchZoomPrevention() {
  $effect(() => {
    function onWheel(e: WheelEvent) {
      if (e.ctrlKey) e.preventDefault();
    }
    function onGesture(e: Event) {
      e.preventDefault();
    }

    document.addEventListener('wheel', onWheel, { passive: false });
    document.addEventListener('gesturestart', onGesture);
    document.addEventListener('gesturechange', onGesture);

    return () => {
      document.removeEventListener('wheel', onWheel);
      document.removeEventListener('gesturestart', onGesture);
      document.removeEventListener('gesturechange', onGesture);
    };
  });
}
