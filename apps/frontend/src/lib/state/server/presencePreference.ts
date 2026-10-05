import { PresencePreferences } from '@chatto/client/server/presencePreference';

export type { PresenceScope } from '@chatto/client/server/presencePreference';

/** The frontend's presence choices for every account it shows. */
export const presencePreferences = new PresencePreferences();
