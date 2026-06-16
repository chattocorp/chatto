import { onMount } from 'svelte';
import type { EventBusCatchUpReason } from '$lib/eventBus.svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { eventBusManager } from '$lib/state/server/eventBus.svelte';
import { useReconnectCallback } from './useReconnectCallback.svelte';

export type MayHaveMissedMessagesReason =
  | 'visibility'
  | 'pageshow'
  | 'online'
  | 'reconnect'
  | 'event-bus-subscription-ended'
  | 'event-bus-ws-reconnected'
  | 'event-bus-heartbeat-stalled'
  | 'manual-shortcut';

const DEDUPE_MS = 1_000;

function isEventBusReason(reason: MayHaveMissedMessagesReason): boolean {
  return reason.startsWith('event-bus-');
}

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

function reasonForEventBusCatchUp(reason: EventBusCatchUpReason): MayHaveMissedMessagesReason {
  switch (reason) {
    case 'subscription-ended':
      return 'event-bus-subscription-ended';
    case 'ws-reconnected':
      return 'event-bus-ws-reconnected';
    case 'heartbeat-stalled':
      return 'event-bus-heartbeat-stalled';
  }
}

function createRefreshRunner(
  callback: (reason: MayHaveMissedMessagesReason) => boolean | void | Promise<boolean | void>
) {
  let lastSucceededAt = 0;
  let inFlight = false;
  let queuedReason: MayHaveMissedMessagesReason | null = null;

  async function run(reason: MayHaveMissedMessagesReason): Promise<void> {
    inFlight = true;
    let succeeded = false;
    let nextReason: MayHaveMissedMessagesReason | null = null;
    console.debug('[room-refresh] maybe-missed signal', { reason });
    try {
      const refreshed = await callback(reason);
      if (refreshed !== false) {
        lastSucceededAt = Date.now();
        succeeded = true;
      }
    } catch (error) {
      console.debug('[room-refresh] maybe-missed callback failed', { reason, error });
    } finally {
      inFlight = false;
      nextReason = queuedReason;
      queuedReason = null;
    }

    if (nextReason) {
      if (!succeeded || isEventBusReason(nextReason)) {
        console.debug('[room-refresh] running queued maybe-missed signal', { reason: nextReason });
        void run(nextReason);
      } else {
        console.debug('[room-refresh] skipped queued duplicate after successful refresh', {
          reason: nextReason
        });
      }
    }
  }

  return {
    trigger(reason: MayHaveMissedMessagesReason): void {
      const now = Date.now();
      if (inFlight) {
        queuedReason = reason;
        console.debug('[room-refresh] queued maybe-missed signal while refresh is running', {
          reason
        });
        return;
      }
      if (now - lastSucceededAt < DEDUPE_MS) {
        console.debug('[room-refresh] skipped duplicate maybe-missed signal', { reason });
        return;
      }
      void run(reason);
    }
  };
}

/**
 * Run a callback when the tab/client has a credible chance of having missed
 * live room events. Bursty browser wake signals are collapsed so one phone
 * unlock does not fan out several identical room refreshes.
 */
export function useMayHaveMissedMessagesCallback(
  callback: (reason: MayHaveMissedMessagesReason) => boolean | void | Promise<boolean | void>
): void {
  const runner = createRefreshRunner(callback);
  const trigger = (reason: MayHaveMissedMessagesReason) => runner.trigger(reason);

  useReconnectCallback(() => trigger('reconnect'));

  $effect(() => {
    const serverId = getActiveServer();
    if (!serverId) return;

    const bus = eventBusManager.getBus(serverId);
    if (!bus) return;

    const catchUpHandler = (reason: EventBusCatchUpReason) => {
      trigger(reasonForEventBusCatchUp(reason));
    };
    bus.catchUpHandlers.add(catchUpHandler);
    return () => {
      bus.catchUpHandlers.delete(catchUpHandler);
    };
  });

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
