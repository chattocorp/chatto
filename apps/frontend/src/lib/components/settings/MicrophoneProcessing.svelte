<!-- @component Local microphone effects; changes apply to the active call or voice test. -->
<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import ChoiceRow from '$lib/ui/ChoiceRow.svelte';
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
  <div role="radiogroup" aria-label={m('voice.preferences.processing')} class="flex flex-col gap-2">
    <ChoiceRow
      label={m('voice.preferences.processing_none')}
      selected={preferences.processingPreset === 'none'}
      disabled={unavailable}
      onclick={() => preferences.setProcessingPreset('none')}
    />
    <ChoiceRow
      label={m('voice.preferences.processing_subtle')}
      selected={preferences.processingPreset === 'subtle'}
      disabled={unavailable}
      onclick={() => preferences.setProcessingPreset('subtle')}
    />
    <ChoiceRow
      label={m('voice.preferences.processing_strong')}
      selected={preferences.processingPreset === 'strong'}
      disabled={unavailable}
      onclick={() => preferences.setProcessingPreset('strong')}
    />
  </div>
  <p class="text-sm text-muted">{m('voice.preferences.gate_separate')}</p>
</div>
