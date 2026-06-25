import { PresenceStatus } from '$lib/gql/graphql';

export type PresenceMode = 'auto' | 'away' | 'doNotDisturb' | 'invisible';

class PresencePreference {
	mode = $state<PresenceMode>('auto');
	effectiveStatus = $state<PresenceStatus>(PresenceStatus.Online);
}

export const presencePreference = new PresencePreference();
