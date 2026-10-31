<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, waitFor } from 'storybook/test';
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
            'Compact one-of-many mode switch with a recessed track and a softly raised selected pill for alternate views, filters, and sort orders. Use ToggleChip when choices can be selected independently.'
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
  const inboxOptions = [
    { value: 'all', label: 'All' },
    { value: 'unread', label: 'Unread' },
    { value: 'mentions', label: 'Mentions' }
  ] as const;
  let order = $state<(typeof orderOptions)[number]['value']>('relevance');
  let inboxFilter = $state<(typeof inboxOptions)[number]['value']>('all');
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

<Story name="Three modes" asChild>
  <SegmentedControl
    label="Filter messages"
    options={inboxOptions}
    value={inboxFilter}
    onchange={(value) => (inboxFilter = value)}
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
        leadingIcon="icon-[uil--search]"
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

<Story name="Long labels in a narrow control" asChild>
  <div class="w-64">
    <SegmentedControl
      label="Notification type"
      options={[
        { value: 'all', label: 'Alle Typen' },
        { value: 'replies', label: 'Antworten' },
        { value: 'mentions', label: 'Erwähnungen' },
        { value: 'reactions', label: 'Reaktionen' }
      ]}
      value="all"
      onchange={() => {}}
    />
  </div>
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

<Story
  name="Responsive filter groups"
  asChild
  play={async ({ canvasElement }) => {
    const container = canvasElement.querySelector<HTMLElement>('[data-filter-demo]')!;
    const group = container.querySelector<HTMLElement>('.segmented-control-group')!;
    const controls = [...group.querySelectorAll<HTMLElement>('[role="radiogroup"]')];
    container.style.width = '900px';
    await waitFor(() => {
      expect(controls[0].getBoundingClientRect().top).toBe(controls[1].getBoundingClientRect().top);
      expect(group.getBoundingClientRect().width).toBeLessThan(
        container.getBoundingClientRect().width
      );
    });
    container.style.width = '320px';
    await waitFor(() => {
      expect(controls[1].getBoundingClientRect().top).toBeGreaterThan(
        controls[0].getBoundingClientRect().bottom
      );
      for (const control of controls) {
        expect(
          Math.abs(control.getBoundingClientRect().width - container.getBoundingClientRect().width)
        ).toBeLessThan(1);
        const selected = control.querySelector('input:checked')!.closest('label')!;
        expect(
          Math.abs(
            parseFloat(control.style.getPropertyValue('--segmented-indicator-width')) -
              selected.getBoundingClientRect().width
          )
        ).toBeLessThan(1);
      }
    });
  }}
>
  <div data-filter-demo class="max-w-full" style:width="900px">
    <div class="segmented-control-group">
      <SegmentedControl
        label="Notification status"
        options={[
          { value: 'all', label: 'All notifications' },
          { value: 'unread', label: 'Unread' }
        ]}
        value="all"
        onchange={() => {}}
      />
      <SegmentedControl
        label="Notification type"
        options={[
          { value: 'all', label: 'All types' },
          { value: 'replies', label: 'Replies' },
          { value: 'mentions', label: 'Mentions' },
          { value: 'reactions', label: 'Reactions' }
        ]}
        value="all"
        onchange={() => {}}
      />
    </div>
  </div>
</Story>
