<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button, TextInput } from '$lib/ui/form';
  import SegmentedControl from './SegmentedControl.svelte';

  const { Story } = defineMeta({
    title: 'UI/SegmentedControl',
    component: SegmentedControl,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Compact one-of-many mode switch for alternate views, filters, and sort orders. Use ToggleChip when choices can be selected independently.'
        }
      }
    }
  });
</script>

<script lang="ts">
  const orderOptions = [
    { value: 'relevance', label: 'Most relevant' },
    { value: 'newest', label: 'Newest' }
  ] as const;
  let order = $state<(typeof orderOptions)[number]['value']>('relevance');
  let query = $state('cake');
</script>

<Story name="Sort mode" asChild>
  <SegmentedControl
    label="Sort messages"
    options={orderOptions}
    value={order}
    onchange={(value) => (order = value)}
  />
</Story>

<Story name="Search controls" asChild>
  <form
    class="flex max-w-4xl flex-wrap items-stretch gap-2"
    onsubmit={(event) => event.preventDefault()}
  >
    <div class="min-w-64 flex-1">
      <TextInput
        label="Search query"
        labelHidden
        bind:value={query}
        placeholder="Search messages"
        leadingIcon="uil--search"
      />
    </div>
    <Button type="submit">Search</Button>
    <SegmentedControl
      label="Sort messages"
      options={orderOptions}
      value={order}
      onchange={(value) => (order = value)}
    />
  </form>
</Story>

<Story name="Disabled" asChild>
  <SegmentedControl
    label="Sort messages"
    options={orderOptions}
    value="newest"
    onchange={() => {}}
    disabled
  />
</Story>
