import { PresenceStatus } from '$lib/render/types';

export type PresenceMode = 'auto' | 'away' | 'doNotDisturb' | 'invisible';

class PresencePreference {
	mode = $state<PresenceMode>('auto');
	effectiveStatus = $state<PresenceStatus>(PresenceStatus.Online);
}

export const presencePreference = new PresencePreference();
