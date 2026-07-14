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
  let disabledValue = $state(35);
</script>

<Story name="Default" asChild>
  <div class="max-w-md">
    <RangeField
      id="volume"
      label="Notification volume"
      icon="uil--volume"
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
