<!-- @component Local microphone effects; changes apply to the active call or voice test. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { Button, Checkbox, RangeField } from '$lib/ui/form';
  import type { CallPreferencesState } from '$lib/state/server/callPreferences.svelte';
  import MicrophoneSensitivity from './MicrophoneSensitivity.svelte';

  let {
    preferences,
    level = 0,
    unavailable = false
  }: {
    preferences: CallPreferencesState;
    level?: number;
    unavailable?: boolean;
  } = $props();
</script>

<div class="flex flex-col gap-5">
  <MicrophoneSensitivity {preferences} {level} {unavailable} />
  <div class="flex flex-col gap-4">
    <Checkbox
      id="microphone-low-cut"
      label={m('voice.preferences.low_cut')}
      disabled={unavailable}
      bind:checked={
        () => preferences.effects.lowCut, (value) => preferences.setEffects({ lowCut: value })
      }
    />
    <Checkbox
      id="microphone-equalizer"
      label={m('voice.preferences.equalizer')}
      disabled={unavailable}
      bind:checked={
        () => preferences.effects.equalizer, (value) => preferences.setEffects({ equalizer: value })
      }
    />
    <div class="grid gap-4 sm:grid-cols-3">
      <RangeField
        id="microphone-bass"
        label={m('voice.preferences.bass')}
        min={-6}
        max={6}
        disabled={unavailable || !preferences.effects.equalizer}
        bind:value={
          () => preferences.effects.bass, (value) => preferences.setEffects({ bass: value ?? 0 })
        }
        displayValue={`${preferences.effects.bass} dB`}
      />
      <RangeField
        id="microphone-mid"
        label={m('voice.preferences.mid')}
        min={-6}
        max={6}
        disabled={unavailable || !preferences.effects.equalizer}
        bind:value={
          () => preferences.effects.mid, (value) => preferences.setEffects({ mid: value ?? 0 })
        }
        displayValue={`${preferences.effects.mid} dB`}
      />
      <RangeField
        id="microphone-treble"
        label={m('voice.preferences.treble')}
        min={-6}
        max={6}
        disabled={unavailable || !preferences.effects.equalizer}
        bind:value={
          () => preferences.effects.treble,
          (value) => preferences.setEffects({ treble: value ?? 0 })
        }
        displayValue={`${preferences.effects.treble} dB`}
      />
    </div>
    <Checkbox
      id="microphone-compressor"
      label={m('voice.preferences.compressor')}
      disabled={unavailable}
      bind:checked={
        () => preferences.effects.compressor,
        (value) => preferences.setEffects({ compressor: value })
      }
    />
    <RangeField
      id="microphone-compressor-amount"
      label={m('voice.preferences.amount')}
      min={0}
      max={100}
      disabled={unavailable || !preferences.effects.compressor}
      bind:value={
        () => preferences.effects.amount, (value) => preferences.setEffects({ amount: value ?? 50 })
      }
      displayValue={`${preferences.effects.amount}%`}
    />
    <div>
      <Button variant="secondary" onclick={() => preferences.resetProcessing()}
        >{m('voice.preferences.reset_processing')}</Button
      >
    </div>
  </div>
</div>
