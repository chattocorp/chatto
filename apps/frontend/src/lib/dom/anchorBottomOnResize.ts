type BottomAnchorOptions = {
  /** True when the scroller follows its end and must stay at the bottom. */
  followsBottom: () => boolean;
  /** True while another operation owns the scroll position. */
  isPaused?: () => boolean;
  /** Runs after each scroll position adjustment. */
  onAdjust?: () => void;
};

/**
 * Keep the bottom edge of a scroller in place when its height changes, as
 * native chat apps do.
 *
 * A browser keeps `scrollTop` when a scroller gets shorter, for example when
 * the virtual keyboard opens or a sibling composer grows. The content at the
 * top edge stays in place, and the content at the bottom edge moves out of
 * view. This attachment moves the scroll offset by the height change instead.
 * A scroller that follows its end goes to the bottom.
 */
export function anchorBottomOnResize({
  followsBottom,
  isPaused = () => false,
  onAdjust
}: BottomAnchorOptions) {
  return (node: HTMLElement) => {
    let previousHeight = node.clientHeight;
    const observer = new ResizeObserver(() => {
      const height = node.clientHeight;
      const delta = previousHeight - height;
      previousHeight = height;
      if (delta === 0 || isPaused()) return;

      if (followsBottom()) {
        node.scrollTop = node.scrollHeight;
      } else {
        node.scrollTop += delta;
      }
      onAdjust?.();
    });
    observer.observe(node);
    return () => observer.disconnect();
  };
}
