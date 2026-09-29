/**
 * Install the LiveKit voice-call implementation for every server store, and
 * register its type with `@chatto/client`. Import this module before the
 * first server store is created: `hooks.client.ts` and Storybook do. Browser
 * tests that render call UI against a real store import it themselves.
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
