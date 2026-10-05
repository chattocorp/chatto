import type { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  createPresenceTracker,
  type PresenceReporter,
  type PresenceTracker
} from '@chatto/client/server/presenceTracking';
import { presencePreferences, type PresenceScope } from './presencePreference';

export type { PresenceReporter };

/** The tracker of the mounted chat root. Components select through it. */
let active: PresenceTracker | null = null;

/** Save a deliberate selection on this server. Never report success before acknowledgement. */
export async function setPresenceStatus(scope: PresenceScope, status: PresenceStatus) {
  if (!active) throw new Error('Presence is not connected');
  await active.select(scope, status);
}

/** Reconcile a private device update; event payloads are invalidations, not stale choices. */
export function refreshPresencePreference(scope: PresenceScope) {
  active?.refresh(scope);
}

/** Start the frontend's presence tracker. The chat root stops it when it unmounts. */
export function initPresenceTracking(getReporters: () => PresenceReporter[]) {
  const tracker = createPresenceTracker(presencePreferences, getReporters);
  active = tracker;
  return {
    sync: tracker.sync,
    stop() {
      tracker.stop();
      if (active === tracker) active = null;
    }
  };
}
