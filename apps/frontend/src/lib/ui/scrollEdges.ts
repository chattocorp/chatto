import type { Attachment } from 'svelte/attachments';

/**
 * Whether content extends past each edge of a scroll axis. On the `x` axis,
 * `start` and `end` follow the element's inline direction, so `start` is the
 * right edge in a right-to-left context. On the `y` axis, `start` is the top.
 */
export type ScrollEdges = { start: boolean; end: boolean };

/**
 * Read edge visibility with a one-pixel tolerance, including browser overscroll.
 * Horizontal positions are normalized to logical inline offsets: browsers report
 * `scrollLeft` from 0 towards negative values when the element's computed
 * `direction` is `rtl`.
 */
export function readScrollEdges(element: HTMLElement, axis: 'x' | 'y'): ScrollEdges {
  const maximum =
    axis === 'x'
      ? element.scrollWidth - element.clientWidth
      : element.scrollHeight - element.clientHeight;
  const position =
    axis === 'x'
      ? getComputedStyle(element).direction === 'rtl'
        ? -element.scrollLeft
        : element.scrollLeft
      : element.scrollTop;
  return {
    start: maximum > 1 && position > 1,
    end: maximum > 1 && maximum - position > 1
  };
}

/**
 * Attach to a stable content wrapper directly inside its scroll viewport.
 * The wrapper must size to its content on the tracked axis. Observe both
 * elements so content and viewport resizing update the edges without DOM watches.
 */
export function trackScrollEdges(
  axis: 'x' | 'y',
  onchange: (edges: ScrollEdges) => void
): Attachment<HTMLElement> {
  return (content) => {
    const viewport = content.parentElement!;
    const update = () => onchange(readScrollEdges(viewport, axis));
    const observer = new ResizeObserver(update);
    observer.observe(viewport);
    observer.observe(content);
    viewport.addEventListener('scroll', update, { passive: true });
    update();

    return () => {
      viewport.removeEventListener('scroll', update);
      observer.disconnect();
    };
  };
}
