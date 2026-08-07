import { tick } from 'svelte';
import type { Attachment } from 'svelte/attachments';

type LoadMoreWhenVisibleOptions = {
  getCursor: () => string | null;
  loadMore: () => Promise<void>;
  hasError: () => boolean;
};

/** Load successive pages while a trailing sentinel remains near the viewport. */
export function useLoadMoreWhenVisible({
  getCursor,
  loadMore,
  hasError
}: LoadMoreWhenVisibleOptions): Attachment<HTMLElement> {
  return (node) => {
    if (typeof IntersectionObserver === 'undefined') return;
    let loadingVisiblePages = false;

    const loadVisiblePages = async (): Promise<void> => {
      if (loadingVisiblePages) return;
      loadingVisiblePages = true;
      try {
        do {
          const cursor = getCursor();
          await loadMore();
          await tick();
          if (hasError() || getCursor() === cursor) break;
          const bounds = node.getBoundingClientRect();
          if (bounds.top > window.innerHeight + 160 || bounds.bottom < -160) break;
        } while (getCursor() && node.isConnected);
      } finally {
        loadingVisiblePages = false;
      }
    };

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) void loadVisiblePages();
      },
      { rootMargin: '160px 0px' }
    );
    observer.observe(node);
    return () => observer.disconnect();
  };
}
