import type { Client } from '@urql/svelte';
import { graphql } from './gql';
import { PresenceStatus, PresenceStatusInput } from './gql/graphql';

const UpdateMyPresenceDoc = graphql(`
  mutation UpdateMyPresence($input: UpdateMyPresenceInput!) {
    updateMyPresence(input: $input)
  }
`);

const IDLE_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes of inactivity → AWAY
const HIDDEN_DELAY_MS = 10_000; // 10 seconds after tab hidden → AWAY
const NOISY_ACTIVITY_THROTTLE_MS = 1_000;

type ActivityState = 'active' | 'idle' | 'hidden';

// Module-level singleton to prevent duplicate tracking
let initialized = false;

function presenceInputToStatus(status: PresenceStatusInput): PresenceStatus {
  switch (status) {
    case PresenceStatusInput.Online:
      return PresenceStatus.Online;
    case PresenceStatusInput.Away:
      return PresenceStatus.Away;
    case PresenceStatusInput.DoNotDisturb:
      return PresenceStatus.DoNotDisturb;
  }
}

/**
 * Initialize presence tracking. Uses idle detection and page visibility
 * to automatically report ONLINE/AWAY status via the updateMyPresence mutation.
 *
 * Broadcasts presence to all clients returned by getClients (all registered instances).
 *
 * Singleton — multiple calls are safe (subsequent calls are no-ops).
 * Call in the chat layout after the GraphQL client is available.
 */
export function initPresenceTracking(
  getClients: () => Client[],
  onStatusChange?: (status: PresenceStatus) => void
): () => void {
  if (initialized) return () => {};
  initialized = true;

  let currentState: ActivityState = 'active';
  let idleTimer: ReturnType<typeof setTimeout> | null = null;
  let hiddenTimer: ReturnType<typeof setTimeout> | null = null;
  let lastTimerResetAt = 0;

  function setPresenceStatus(status: PresenceStatusInput) {
    onStatusChange?.(presenceInputToStatus(status));
    for (const client of getClients()) {
      client
        .mutation(UpdateMyPresenceDoc, { input: { status } })
        .toPromise()
        .catch(() => {});
    }
  }

  function resetIdleTimer() {
    if (idleTimer) clearTimeout(idleTimer);
    lastTimerResetAt = Date.now();
    idleTimer = setTimeout(() => transition('idle'), IDLE_TIMEOUT_MS);
  }

  function transition(newState: ActivityState) {
    if (newState === currentState) return;
    currentState = newState;

    if (newState === 'active') {
      setPresenceStatus(PresenceStatusInput.Online);
      resetIdleTimer();
    } else {
      // idle or hidden
      setPresenceStatus(PresenceStatusInput.Away);
    }
  }

  function onActivity(noisy = false) {
    if (currentState !== 'active') {
      transition('active');
      return;
    }

    if (!noisy || Date.now() - lastTimerResetAt >= NOISY_ACTIVITY_THROTTLE_MS) {
      resetIdleTimer();
    }
  }

  function onQuietActivity() {
    onActivity(false);
  }

  function onNoisyActivity() {
    onActivity(true);
  }

  const quietActivityEvents = ['pointerdown', 'keydown', 'touchstart'] as const;
  const noisyActivityEvents = ['pointermove', 'wheel', 'scroll'] as const;

  for (const event of quietActivityEvents) {
    document.addEventListener(event, onQuietActivity, { passive: true });
  }
  for (const event of noisyActivityEvents) {
    document.addEventListener(event, onNoisyActivity, { passive: true });
  }

  // Page visibility change
  function onVisibilityChange() {
    if (document.visibilityState === 'hidden') {
      // Brief delay before reporting AWAY — handles quick tab switches
      hiddenTimer = setTimeout(() => transition('hidden'), HIDDEN_DELAY_MS);
    } else {
      if (hiddenTimer) {
        clearTimeout(hiddenTimer);
        hiddenTimer = null;
      }
      transition('active');
    }
  }

  document.addEventListener('visibilitychange', onVisibilityChange);
  window.addEventListener('focus', onQuietActivity);

  // Start idle timer
  resetIdleTimer();

  return () => {
    for (const event of quietActivityEvents) {
      document.removeEventListener(event, onQuietActivity);
    }
    for (const event of noisyActivityEvents) {
      document.removeEventListener(event, onNoisyActivity);
    }
    document.removeEventListener('visibilitychange', onVisibilityChange);
    window.removeEventListener('focus', onQuietActivity);
    if (idleTimer) clearTimeout(idleTimer);
    if (hiddenTimer) clearTimeout(hiddenTimer);
    initialized = false;
  };
}
