import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

/**
 * Presence modes are always explicit user choices; there is no automatic
 * idle-away mode. The default is the explicit 'online' mode.
 */
export type PresenceMode = 'online' | 'away' | 'doNotDisturb' | 'invisible';

class PresencePreference {
  mode = $state<PresenceMode>('online');
  effectiveStatus = $state<PresenceStatus>(PresenceStatus.ONLINE);
}

export const presencePreference = new PresencePreference();
