/**
 * Install the LiveKit voice-call implementation for every server store, and
 * register its type with `@chatto/client`. Import this module before the
 * first store reads `voiceCall`; the root layout does.
 */

import { setVoiceCallFactory } from '@chatto/client/server/voiceCall';
import { CallPreferencesState } from './callPreferences.svelte';
import { VoiceCallState } from './voiceCall.svelte';

declare module '@chatto/client/register' {
  interface Register {
    voiceCall: VoiceCallState;
  }
}

setVoiceCallFactory(
  ({ serverId, api, permissions }) =>
    new VoiceCallState(api, permissions, new CallPreferencesState(serverId))
);
