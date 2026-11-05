<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Panel from '../Panel.svelte';
  import RangeField from './RangeField.svelte';

  const { Story } = defineMeta({
    title: 'Form/RangeField',
    component: RangeField,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Use RangeField for bounded numeric settings. A flat label and value row sits above a slim rail. The parent owns the surrounding surface and padding; the native input keeps a 44px pointer target.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let volume = $state(70);
  let steppedVolume = $state(100);
  let disabledValue = $state(35);
  let contrastAge = $state(30);
  let threshold = $state(-35);
</script>

<Story name="Default" asChild>
  <div class="max-w-md">
    <RangeField
      id="volume"
      label="Notification volume"
      icon="icon-[uil--volume]"
      min={0}
      max={100}
      bind:value={volume}
      displayValue={`${volume}%`}
    />
  </div>
</Story>

<Story name="Settings panel" asChild>
  <div class="max-w-xl">
    <Panel title="Microphone">
      <div class="flex flex-col gap-4">
        <RangeField
          id="noise-gate"
          label="Noise gate threshold"
          min={-60}
          max={0}
          bind:value={threshold}
          displayValue={`${threshold} dB`}
          describedBy="noise-gate-help"
        />
        <p id="noise-gate-help" class="text-sm text-muted">
          Sounds below this level are muted. Raise it to block more background noise.
        </p>
        <RangeField
          id="panel-volume"
          label="Notification volume"
          icon="icon-[uil--volume]"
          min={0}
          max={100}
          bind:value={volume}
          displayValue={`${volume}%`}
        />
      </div>
    </Panel>
  </div>
</Story>

<Story name="Stops" asChild>
  <div class="w-full">
    <RangeField
      id="ui-contrast"
      label="Contrast"
      min={20}
      max={40}
      step={2}
      ticks={[20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40]}
      bind:value={contrastAge}
      displayValue={`${Math.round((contrastAge - 20) * 5)}%`}
    />
  </div>
</Story>

<Story name="Disabled" asChild>
  <div class="max-w-md">
    <RangeField
      id="disabled-range"
      label="Background volume"
      min={0}
      max={100}
      bind:value={disabledValue}
      displayValue={`${disabledValue}%`}
      disabled
    />
  </div>
</Story>

<Story name="Steps and reference mark" asChild>
  <div class="max-w-md">
    <RangeField
      id="stepped-volume"
      label="Participant volume"
      min={0}
      max={200}
      step={5}
      ticks={[100]}
      bind:value={steppedVolume}
      displayValue={`${steppedVolume}%`}
    />
  </div>
</Story>
