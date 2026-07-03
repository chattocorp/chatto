import { onMount } from 'svelte';
import type { EventBusCatchUpReason, EventBusCatchUpSignal } from '$lib/eventBus.svelte';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import { eventBusManager } from '$lib/state/server/eventBus.svelte';
import { useReconnectCallback } from './useReconnectCallback.svelte';
import {
  emitActiveServerResumeSignal,
  emitServerResumeSignal,
  registerServerResumeCallback,
  type ResumeSignal
} from './resumeCoordinator.svelte';

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
  callback: (signal: ResumeSignal) => boolean | void | Promise<boolean | void>
) {
  let lastSucceededAt = 0;
  let inFlight = false;
  let queuedSignal: ResumeSignal | null = null;

  async function run(signal: ResumeSignal): Promise<void> {
    inFlight = true;
    let succeeded = false;
    let nextSignal: ResumeSignal | null = null;
    console.debug('[room-refresh] maybe-missed signal', signal);
    try {
      const refreshed = await callback(signal);
      if (refreshed !== false) {
        lastSucceededAt = Date.now();
        succeeded = true;
      }
    } catch (error) {
      console.debug('[room-refresh] maybe-missed callback failed', { signal, error });
    } finally {
      inFlight = false;
      nextSignal = queuedSignal;
      queuedSignal = null;
    }

    if (nextSignal) {
      if (!succeeded || isEventBusReason(nextSignal.reason)) {
        console.debug('[room-refresh] running queued maybe-missed signal', nextSignal);
        void run(nextSignal);
      } else {
        console.debug('[room-refresh] skipped queued duplicate after successful refresh', {
          signal: nextSignal
        });
      }
    }
  }

  return {
    trigger(signal: ResumeSignal): void {
      const now = Date.now();
      if (inFlight) {
        queuedSignal = signal;
        console.debug('[room-refresh] queued maybe-missed signal while refresh is running', signal);
        return;
      }
      if (now - lastSucceededAt < DEDUPE_MS) {
        console.debug('[room-refresh] skipped duplicate maybe-missed signal', signal);
        return;
      }
      void run(signal);
    }
  };
}

/**
 * Run a callback when the tab/client has a credible chance of having missed
 * live room events. Bursty browser wake signals are collapsed so one phone
 * unlock does not fan out several identical room refreshes.
 */
export function useMayHaveMissedMessagesCallback(
  callback: (signal: ResumeSignal) => boolean | void | Promise<boolean | void>
): void {
  const runner = createRefreshRunner(callback);

  useReconnectCallback(() =>
    emitActiveServerResumeSignal({
      reason: 'reconnect',
      source: 'reconnect'
    })
  );

  $effect(() => {
    const serverId = getActiveServer();
    if (!serverId) return;

    return registerServerResumeCallback(serverId, (signal) => runner.trigger(signal));
  });

  $effect(() => {
    const serverId = getActiveServer();
    if (!serverId) return;

    const bus = eventBusManager.getBus(serverId);
    if (!bus) return;

    const catchUpHandler = (signal: EventBusCatchUpSignal) => {
      emitServerResumeSignal(serverId, {
        reason: reasonForEventBusCatchUp(signal.reason),
        phase: signal.phase,
        source: 'event-bus'
      });
    };
    bus.catchUpHandlers.add(catchUpHandler);
    return () => {
      bus.catchUpHandlers.delete(catchUpHandler);
    };
  });

  onMount(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.repeat || isEditableTarget(event.target)) return;

      // Temporary manual refresh shortcut for visual artifact testing.
      if (
        event.ctrlKey &&
        event.altKey &&
        event.shiftKey &&
        !event.metaKey &&
        event.code === 'KeyR'
      ) {
        event.preventDefault();
        emitActiveServerResumeSignal(
          {
            reason: 'manual-shortcut',
            source: 'manual'
          },
          { coalesceMs: 0 }
        );
      }
    };

    window.addEventListener('keydown', onKeyDown);

    return () => {
      window.removeEventListener('keydown', onKeyDown);
    };
  });
}
