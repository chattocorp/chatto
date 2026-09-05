import { tick } from 'svelte';
import type { Attachment } from 'svelte/attachments';

type LoadMoreWhenVisibleOptions = {
  /** Cursor or offset; null means there are no more pages. */
  getCursor: () => string | number | null;
  /** The owner reports errors and provides manual retry controls. */
  loadMore: () => Promise<void>;
  hasError?: () => boolean;
};

/**
 * Load successive pages while a sentinel intersects the viewport and its scroll
 * ancestors. Re-observe after each advancing page so the browser checks the new
 * layout. Detach stops continuation; the owner controls in-flight requests.
 */
export function useLoadMoreWhenVisible({
  getCursor,
  loadMore,
  hasError = () => false
}: LoadMoreWhenVisibleOptions): Attachment<HTMLElement> {
  return (node) => {
    if (typeof IntersectionObserver === 'undefined') return;
    let active = true;
    let loading = false;

    const loadVisiblePages = async (): Promise<void> => {
      const cursor = getCursor();
      if (!active || loading || cursor === null || hasError()) return;
      loading = true;
      try {
        await loadMore();
        await tick();
        if (!active || hasError() || getCursor() === null || getCursor() === cursor) return;
        observer.unobserve(node);
        observer.observe(node);
      } catch {
        // A rejected load must not start an automatic retry loop.
      } finally {
        loading = false;
      }
    };

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void loadVisiblePages();
      },
      { rootMargin: '160px 0px' }
    );
    observer.observe(node);
    return () => {
      active = false;
      observer.disconnect();
    };
  };
}
