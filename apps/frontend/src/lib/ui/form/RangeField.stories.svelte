<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import RangeField from './RangeField.svelte';

  const { Story } = defineMeta({
    title: 'Form/RangeField',
    component: RangeField,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Use RangeField for bounded numeric settings. It keeps the label, current value, optional icon, and range control in one consistent row.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let volume = $state(70);
  let steppedVolume = $state(100);
  let disabledValue = $state(35);
  let awesome = $state(100);
  let warmingUp = $state(90);
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

<Story name="Rainbow at maximum" asChild>
  <div class="max-w-md">
    <RangeField
      id="rainbow-range"
      label="Voice Quality"
      min={0}
      max={100}
      bind:value={awesome}
      displayValue={awesome === 100 ? 'AWESOME' : `${awesome}%`}
      rainbow
    />
  </div>
</Story>
<Story name="Unavailable rainbow" asChild>
  <div class="max-w-md">
    <RangeField
      id="disabled-rainbow"
      label="Voice Quality"
      min={0}
      max={100}
      value={100}
      displayValue="AWESOME"
      rainbow
      disabled
    />
  </div>
</Story>

<Story name="Rainbow fading in" asChild>
  <div class="max-w-md">
    <RangeField
      id="partial-rainbow"
      label="Voice Quality"
      min={0}
      max={100}
      bind:value={warmingUp}
      displayValue={`${warmingUp}%`}
      rainbow
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
