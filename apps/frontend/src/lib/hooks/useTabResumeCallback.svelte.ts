import { onMount } from 'svelte';

/**
 * Run a callback on mount and whenever the browser tab becomes visible again.
 *
 * This is for browser presentation work such as re-measuring a virtual list or
 * advancing a local expiry clock. Canonical server data catches up through the
 * resumable realtime projection instead of this callback.
 *
 * Must be called during component initialization.
 */
export function useTabResumeCallback(callback: () => void) {
  onMount(() => {
    callback();

    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') callback();
    };
    const onPageShow = () => callback();

    document.addEventListener('visibilitychange', onVisibilityChange);
    window.addEventListener('pageshow', onPageShow);

    return () => {
      document.removeEventListener('visibilitychange', onVisibilityChange);
      window.removeEventListener('pageshow', onPageShow);
    };
  });
}
