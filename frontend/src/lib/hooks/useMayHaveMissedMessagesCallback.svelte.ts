import { onMount } from 'svelte';
import { useReconnectCallback } from './useReconnectCallback.svelte';

export type MayHaveMissedMessagesReason =
  | 'visibility'
  | 'pageshow'
  | 'online'
  | 'reconnect'
  | 'manual-shortcut';

const DEDUPE_MS = 1_000;

/**
 * Run a callback when the tab/client has a credible chance of having missed
 * live room events. Bursty browser wake signals are collapsed so one phone
 * unlock does not fan out several identical room refreshes.
 */
export function useMayHaveMissedMessagesCallback(
  callback: (reason: MayHaveMissedMessagesReason) => boolean | void | Promise<boolean | void>
): void {
  let lastSucceededAt = 0;
  let inFlight = false;
  let queuedReason: MayHaveMissedMessagesReason | null = null;

  function isEditableTarget(target: EventTarget | null): boolean {
    if (!(target instanceof HTMLElement)) return false;
    const tagName = target.tagName.toLowerCase();
    return (
      tagName === 'input' ||
      tagName === 'textarea' ||
      tagName === 'select' ||
      target.isContentEditable
    );
  }

  async function run(reason: MayHaveMissedMessagesReason): Promise<void> {
    inFlight = true;
    console.debug('[room-refresh] maybe-missed signal', { reason });
    try {
      const refreshed = await callback(reason);
      if (refreshed !== false) {
        lastSucceededAt = Date.now();
      }
    } catch (error) {
      console.debug('[room-refresh] maybe-missed callback failed', { reason, error });
    } finally {
      inFlight = false;
      const nextReason = queuedReason;
      queuedReason = null;
      if (nextReason && Date.now() - lastSucceededAt >= DEDUPE_MS) {
        void run(nextReason);
      }
    }
  }

  function trigger(reason: MayHaveMissedMessagesReason): void {
    const now = Date.now();
    if (inFlight) {
      queuedReason = reason;
      console.debug('[room-refresh] queued maybe-missed signal while refresh is running', { reason });
      return;
    }
    if (now - lastSucceededAt < DEDUPE_MS) {
      console.debug('[room-refresh] skipped duplicate maybe-missed signal', { reason });
      return;
    }
    void run(reason);
  }

  useReconnectCallback(() => trigger('reconnect'));

  onMount(() => {
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') trigger('visibility');
    };
    const onPageShow = () => trigger('pageshow');
    const onOnline = () => trigger('online');
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.repeat || isEditableTarget(event.target)) return;

      // Temporary manual refresh shortcut for visual artifact testing.
      if (event.ctrlKey && event.altKey && event.shiftKey && !event.metaKey && event.code === 'KeyR') {
        event.preventDefault();
        trigger('manual-shortcut');
      }
    };

    document.addEventListener('visibilitychange', onVisibilityChange);
    window.addEventListener('pageshow', onPageShow);
    window.addEventListener('online', onOnline);
    window.addEventListener('keydown', onKeyDown);

    return () => {
      document.removeEventListener('visibilitychange', onVisibilityChange);
      window.removeEventListener('pageshow', onPageShow);
      window.removeEventListener('online', onOnline);
      window.removeEventListener('keydown', onKeyDown);
    };
  });
}
