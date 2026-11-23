<!-- @component
Shared media actions for the call panel and app header. The parent owns the group layout.
Microphone, camera, and screen share show a green glyph while they send media and a
neutral, slashed glyph while off. Microphone and camera are toggles with a stable name and
`aria-pressed`. Leave is a plain button with a red hang-up glyph: the handset rotated 135°, as in
common call apps. Unicons has no hang-up icon, and its `phone-slash` slants the
other way from the other slashed glyphs.
-->
<script lang="ts">
  import type { VoiceCallState } from '$lib/state/server/voiceCall.svelte';
  import { m } from '$lib/i18n/messages';
  import VoiceCallControlButton from './VoiceCallControlButton.svelte';
  import ScreenShareControlButton from './ScreenShareControlButton.svelte';

  let {
    voiceCallState,
    testIds
  }: {
    voiceCallState: VoiceCallState;
    /** Stable selectors supplied by each control surface. */
    testIds: { mute: string; camera: string; screenShare: string; leave: string };
  } = $props();
</script>

<VoiceCallControlButton
  class={voiceCallState.isMuted ? 'pill-button' : 'pill-button-live'}
  iconClass="text-lg"
  label={m('voice.microphone')}
  pressed={!voiceCallState.isMuted}
  testId={testIds.mute}
  icon={voiceCallState.isMuted ? 'icon-[uil--microphone-slash]' : 'icon-[uil--microphone]'}
  onclick={() => voiceCallState.toggleMute()}
  pending={voiceCallState.isMicrophonePending}
  disabled={!voiceCallState.canUseVoice && voiceCallState.isMuted}
/>
<VoiceCallControlButton
  class={voiceCallState.isCameraEnabled ? 'pill-button-live' : 'pill-button'}
  iconClass="text-lg"
  label={m('voice.camera')}
  pressed={voiceCallState.isCameraEnabled}
  testId={testIds.camera}
  icon={voiceCallState.isCameraEnabled ? 'icon-[uil--video]' : 'icon-[uil--video-slash]'}
  onclick={() => voiceCallState.toggleCamera()}
  pending={voiceCallState.isCameraPending}
  disabled={!voiceCallState.canUseCamera && !voiceCallState.isCameraEnabled}
/>
<ScreenShareControlButton
  {voiceCallState}
  class={voiceCallState.isScreenShareEnabled ? 'pill-button-live' : 'pill-button'}
  iconClass="text-lg"
  testId={testIds.screenShare}
/>
<VoiceCallControlButton
  class="pill-button"
  iconClass="text-lg text-danger rotate-135"
  label={m('voice.leave')}
  testId={testIds.leave}
  icon="icon-[uil--phone]"
  onclick={() => voiceCallState.leave()}
/>
