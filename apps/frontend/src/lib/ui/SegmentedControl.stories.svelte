<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
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
</script>

<Story name="Sort mode" asChild>
  <SegmentedControl
    label="Sort messages"
    options={orderOptions}
    value={order}
    onchange={(value) => (order = value)}
  />
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
