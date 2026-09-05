import type { Attachment } from 'svelte/attachments';

/** Whether content extends past each edge of a physical scroll axis. */
export type ScrollEdges = { start: boolean; end: boolean };

/** Read edge visibility with a one-pixel tolerance and clamp browser overscroll. */
export function readScrollEdges(element: HTMLElement, axis: 'x' | 'y'): ScrollEdges {
  const maximum = Math.max(
    0,
    axis === 'x'
      ? element.scrollWidth - element.clientWidth
      : element.scrollHeight - element.clientHeight
  );
  const position = Math.min(
    Math.max(axis === 'x' ? element.scrollLeft : element.scrollTop, 0),
    maximum
  );
  return {
    start: maximum > 1 && position > 1,
    end: maximum > 1 && maximum - position > 1
  };
}

/**
 * Report edges on mount, scroll, and viewport or direct-child size changes.
 * Track replacement children and release listeners and observers on detach.
 */
export function trackScrollEdges(
  axis: 'x' | 'y',
  onchange: (edges: ScrollEdges) => void
): Attachment<HTMLElement> {
  return (element) => {
    const update = () => onchange(readScrollEdges(element, axis));
    const resizeObserver = new ResizeObserver(update);
    const observeChildren = () => {
      resizeObserver.disconnect();
      resizeObserver.observe(element);
      for (const child of element.children) {
        if (child instanceof HTMLElement) resizeObserver.observe(child);
      }
    };
    const mutationObserver = new MutationObserver(() => {
      observeChildren();
      update();
    });

    update();
    element.addEventListener('scroll', update, { passive: true });
    observeChildren();
    mutationObserver.observe(element, { childList: true });

    return () => {
      element.removeEventListener('scroll', update);
      mutationObserver.disconnect();
      resizeObserver.disconnect();
    };
  };
}
