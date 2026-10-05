<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FormField from './FormField.svelte';
  import TimeInput from './TimeInput.svelte';

  const { Story } = defineMeta({
    title: 'Form/TimeInput',
    component: TimeInput,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Use TimeInput when the viewer’s 12-hour or 24-hour preference must apply. Native time inputs follow the browser locale instead. The value is always a 24-hour HH:mm string.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let time24 = $state('14:30');
  let time12 = $state('14:30');
  let dateValue = $state('2025-04-27');
  let groupedTime = $state('09:15');
</script>

<Story name="Twenty-four-hour clock" asChild>
  <div class="flex max-w-xs flex-col gap-2">
    <TimeInput bind:value={time24} hour12={false} />
    <p class="text-sm text-muted">Value: {time24 || '(incomplete)'}</p>
  </div>
</Story>

<Story name="Twelve-hour clock" asChild>
  <div class="flex max-w-xs flex-col gap-2">
    <TimeInput bind:value={time12} hour12 />
    <p class="text-sm text-muted">Value: {time12 || '(incomplete)'}</p>
  </div>
</Story>

<Story name="With date in a field group" asChild>
  <div class="max-w-sm">
    <FormField id="storybook-when" label="Date and time" group required>
      <div class="flex gap-2">
        <input class="input min-w-0 flex-1" type="date" aria-label="Date" bind:value={dateValue} />
        <TimeInput bind:value={groupedTime} hour12={false} />
      </div>
    </FormField>
  </div>
</Story>

<Story name="Disabled" asChild>
  <div class="max-w-xs">
    <TimeInput value="18:45" hour12 disabled />
  </div>
</Story>
