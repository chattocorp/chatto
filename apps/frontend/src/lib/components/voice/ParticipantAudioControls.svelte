<!-- @component Listener-local volume controls for a remote participant inside a user context menu. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import type {
    ParticipantAudioPreferences,
    ParticipantVolumeControl
  } from '$lib/state/server/callPreferences.svelte';
  import MenuSection from '$lib/ui/MenuSection.svelte';
  import RangeField from '$lib/ui/form/RangeField.svelte';

  let {
    settings,
    boostAvailable = true,
    onVolumeChange
  }: {
    settings: Readonly<ParticipantAudioPreferences>;
    boostAvailable?: boolean;
    onVolumeChange: (control: ParticipantVolumeControl, value: number) => void;
  } = $props();
  const id = $props.id();
  const maximum = $derived(boostAvailable ? 200 : 100);
  const voice = $derived(Math.min(maximum, settings.voiceVolume));
  const stream = $derived(Math.min(maximum, settings.streamVolume));
</script>

<MenuSection>
  <p class="px-3 py-1 text-sm text-muted">{m('voice.participant_audio.local_only')}</p>
  <RangeField
    id={`${id}-voice`}
    label={m('voice.participant_audio.voice')}
    value={voice}
    displayValue={`${voice}%`}
    min={0}
    max={maximum}
    step={5}
    ticks={[100]}
    oninput={(event) =>
      onVolumeChange('voiceVolume', (event.currentTarget as HTMLInputElement).valueAsNumber)}
  />
  <RangeField
    id={`${id}-stream`}
    label={m('voice.participant_audio.stream')}
    value={stream}
    displayValue={`${stream}%`}
    min={0}
    max={maximum}
    step={5}
    ticks={[100]}
    oninput={(event) =>
      onVolumeChange('streamVolume', (event.currentTarget as HTMLInputElement).valueAsNumber)}
  />
  {#if !boostAvailable}
    <p class="px-3 py-1 text-sm text-muted">{m('voice.participant_audio.boost_unavailable')}</p>
  {/if}
</MenuSection>
