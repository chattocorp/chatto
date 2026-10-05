type BottomAnchorOptions = {
  /** True when the scroller follows its end and must stay at the bottom. */
  followsBottom: () => boolean;
  /** True while another operation owns the scroll position. */
  isPaused?: () => boolean;
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
 *
 * When a scroller gets taller, the browser can clamp `scrollTop` during layout,
 * before the resize callback runs. The adjustment therefore starts from the
 * offset of the last scroll event, which the clamp has not changed yet. For the
 * same reason, a `requestAnimationFrame` write in the frame of a resize is not
 * recorded yet: callers pause the attachment while such an operation runs.
 */
export function anchorBottomOnResize({
  followsBottom,
  isPaused = () => false
}: BottomAnchorOptions) {
  return (node: HTMLElement) => {
    let previousHeight = node.clientHeight;
    let previousScrollTop = node.scrollTop;
    const recordScroll = () => {
      previousScrollTop = node.scrollTop;
    };
    const observer = new ResizeObserver(() => {
      const height = node.clientHeight;
      const delta = previousHeight - height;
      previousHeight = height;
      if (delta === 0 || isPaused()) return;

      node.scrollTop = followsBottom() ? node.scrollHeight : previousScrollTop + delta;
      recordScroll();
    });
    node.addEventListener('scroll', recordScroll, { passive: true });
    observer.observe(node);
    return () => {
      observer.disconnect();
      node.removeEventListener('scroll', recordScroll);
    };
  };
}
