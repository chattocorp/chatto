<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ChoiceRow from './ChoiceRow.svelte';

  const { Story } = defineMeta({
    title: 'UI/ChoiceRow',
    component: ChoiceRow,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'One-of-many settings choice. Group ChoiceRow instances in an accessible radiogroup.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let selected = $state('system');
</script>

<Story name="Group" asChild>
  <div class="flex max-w-md flex-col gap-2" role="radiogroup" aria-label="Display theme">
    <ChoiceRow
      label="System"
      description="Follow this device."
      selected={selected === 'system'}
      onclick={() => (selected = 'system')}
    />
    <ChoiceRow
      label="Light"
      description="Always use the light theme."
      selected={selected === 'light'}
      onclick={() => (selected = 'light')}
    />
    <ChoiceRow
      label="Dark"
      description="Always use the dark theme."
      selected={selected === 'dark'}
      onclick={() => (selected = 'dark')}
    />
  </div>
</Story>

<Story name="Disabled" asChild>
  <div class="flex max-w-md flex-col gap-2" role="radiogroup" aria-label="Managed setting">
    <ChoiceRow label="Automatic" description="Managed by the server." selected disabled />
    <ChoiceRow label="Custom" disabled />
  </div>
</Story>
