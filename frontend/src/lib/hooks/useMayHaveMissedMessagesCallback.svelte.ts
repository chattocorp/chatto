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
  callback: (reason: MayHaveMissedMessagesReason) => void | Promise<void>
): void {
  let lastTriggeredAt = 0;

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

  function trigger(reason: MayHaveMissedMessagesReason): void {
    const now = Date.now();
    if (now - lastTriggeredAt < DEDUPE_MS) {
      console.debug('[room-refresh] skipped duplicate maybe-missed signal', { reason });
      return;
    }
    lastTriggeredAt = now;
    console.debug('[room-refresh] maybe-missed signal', { reason });
    void callback(reason);
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
