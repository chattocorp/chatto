/**
 * The LiveKit voice-call implementation of the frontend's server stores, and
 * its type registration with `@chatto/client`. `$lib/client` passes the
 * factory to its client.
 */

import type { VoiceCallFactory } from '@chatto/client/server/voiceCall';
import { CallPreferencesState } from './callPreferences.svelte';
import { VoiceCallState } from './voiceCall.svelte';

declare module '@chatto/client/register' {
  interface Register {
    voiceCall: VoiceCallState;
  }
}

/** Create the LiveKit controller of one server store. */
export const voiceCallFactory: VoiceCallFactory<VoiceCallState> = ({
  serverId,
  api,
  permissions
}) => new VoiceCallState(api, permissions, new CallPreferencesState(serverId));
