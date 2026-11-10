<!-- @component Shared media actions for the call panel and app header. The parent owns the group layout. -->
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
  class={voiceCallState.isMuted ? 'pill-button' : 'pill-button-success'}
  iconClass="text-lg"
  label={m('voice.mute')}
  pressed={voiceCallState.isMuted}
  testId={testIds.mute}
  icon={voiceCallState.isMuted ? 'icon-[uil--microphone-slash]' : 'icon-[uil--microphone]'}
  onclick={() => voiceCallState.toggleMute()}
  pending={voiceCallState.isMicrophonePending}
  disabled={!voiceCallState.canUseVoice && voiceCallState.isMuted}
/>
<VoiceCallControlButton
  class={voiceCallState.isCameraEnabled ? 'pill-button-success' : 'pill-button'}
  iconClass="text-lg"
  label={voiceCallState.isCameraEnabled ? m('voice.turn_off_camera') : m('voice.turn_on_camera')}
  testId={testIds.camera}
  icon={voiceCallState.isCameraEnabled ? 'icon-[uil--video]' : 'icon-[uil--video-slash]'}
  onclick={() => voiceCallState.toggleCamera()}
  pending={voiceCallState.isCameraPending}
  disabled={!voiceCallState.canUseCamera && !voiceCallState.isCameraEnabled}
/>
<ScreenShareControlButton
  {voiceCallState}
  class={voiceCallState.isScreenShareEnabled ? 'pill-button-success' : 'pill-button'}
  iconClass="text-lg"
  testId={testIds.screenShare}
/>
<VoiceCallControlButton
  class="pill-button-danger"
  iconClass="text-lg"
  label={m('voice.leave')}
  testId={testIds.leave}
  icon="icon-[uil--phone-slash]"
  onclick={() => voiceCallState.leave()}
/>
