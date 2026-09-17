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
    source = 'voiceVolume',
    boostAvailable = true,
    onVolumeChange
  }: {
    settings: Readonly<ParticipantAudioPreferences>;
    /** The card's audio source; each menu owns one volume control. */
    source?: ParticipantVolumeControl;
    boostAvailable?: boolean;
    onVolumeChange: (control: ParticipantVolumeControl, value: number) => void;
  } = $props();
  const id = $props.id();
  const maximum = $derived(boostAvailable ? 200 : 100);
  const volume = $derived(Math.min(maximum, settings[source]));
</script>

<MenuSection>
  <p class="px-3 py-1 text-sm text-muted">{m('voice.participant_audio.local_only')}</p>
  <RangeField
    id={`${id}-${source}`}
    label={source === 'voiceVolume'
      ? m('voice.participant_audio.voice')
      : m('voice.participant_audio.stream')}
    value={volume}
    displayValue={`${volume}%`}
    min={0}
    max={maximum}
    step={5}
    ticks={[100]}
    oninput={(event) =>
      onVolumeChange(source, (event.currentTarget as HTMLInputElement).valueAsNumber)}
  />
  {#if !boostAvailable}
    <p class="px-3 py-1 text-sm text-muted">{m('voice.participant_audio.boost_unavailable')}</p>
  {/if}
</MenuSection>
