import { Codecs, globalSlot } from '$lib/storage/slot';

/**
 * Records whether the user has allowed Server Directory discovery on this
 * device. Discovery contacts servers outside the current Chatto server.
 */
export const serverDirectoryDiscoveryConsent = globalSlot(
  'server-directory-discovery-consent-v1',
  false,
  Codecs.boolean
);
