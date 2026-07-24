import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';

export type PresenceMode = 'auto' | 'away' | 'doNotDisturb' | 'invisible';

class PresencePreference {
  mode = $state<PresenceMode>('auto');
  effectiveStatus = $state<PresenceStatus>(PresenceStatus.ONLINE);
}

export const presencePreference = new PresencePreference();
