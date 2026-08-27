<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Combobox from './Combobox.svelte';

  const { Story } = defineMeta({
    title: 'Form/Combobox',
    component: Combobox,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Searchable selection field with keyboard navigation and a top-layer result popover.'
        }
      }
    }
  });
</script>

<script lang="ts">
  type Timezone = { id: string; label: string };

  const timezones: Timezone[] = [
    { id: 'Europe/Berlin', label: 'Berlin' },
    { id: 'Europe/London', label: 'London' },
    { id: 'America/New_York', label: 'New York' },
    { id: 'Asia/Tokyo', label: 'Tokyo' }
  ];
  const emptyTimezones: Timezone[] = [];

  let query = $state('');
  let value = $state('');
  let text = $state('');
  const filtered = $derived(
    timezones.filter((timezone) => timezone.label.toLowerCase().includes(query.toLowerCase()))
  );
</script>

<Story name="Searchable" asChild>
  <div class="max-w-md">
    <Combobox
      id="timezone"
      label="Timezone"
      description="Used for message timestamps and scheduled notifications."
      bind:value
      bind:text
      items={filtered}
      getValue={(timezone) => timezone.id}
      getLabel={(timezone) => timezone.label}
      allowFreeform={false}
      placeholder="Search timezones"
      ontextchange={(next) => (query = next)}
    />
  </div>
</Story>

<Story name="States" asChild>
  <div class="grid max-w-3xl gap-5 md:grid-cols-2">
    <Combobox
      id="combobox-loading"
      label="Loading"
      items={emptyTimezones}
      getValue={(timezone) => timezone.id}
      getLabel={(timezone) => timezone.label}
      loading
    />
    <Combobox
      id="combobox-disabled"
      label="Disabled"
      value="Europe/Berlin"
      text="Berlin"
      items={timezones}
      getValue={(timezone) => timezone.id}
      getLabel={(timezone) => timezone.label}
      disabled
    />
    <Combobox
      id="combobox-error"
      label="Error"
      items={timezones}
      getValue={(timezone) => timezone.id}
      getLabel={(timezone) => timezone.label}
      error="Choose a supported timezone."
    />
  </div>
</Story>
