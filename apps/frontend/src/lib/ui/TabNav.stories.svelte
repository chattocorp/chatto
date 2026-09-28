<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PaneHeader from './PaneHeader.svelte';
  import TabNav from './TabNav.svelte';

  const { Story } = defineMeta({
    title: 'UI/TabNav',
    component: TabNav,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Routed section tabs for one resource page, rendered in the tabs snippet of PaneHeader. Tabs show labels in wide headers and only icons in narrow headers. Use SegmentedControl for view modes that do not change the route.'
        }
      }
    }
  });

  const items = [
    { href: '#overview', label: 'Overview', icon: 'icon-[uil--robot]', current: true },
    { href: '#integrations', label: 'Integrations', icon: 'icon-[uil--plug]', current: false },
    {
      href: '#permissions',
      label: 'Permissions',
      icon: 'icon-[uil--shield-check]',
      current: false
    }
  ];
</script>

<Story name="Wide pane header" asChild>
  <div class="w-[960px] rounded-md border border-border">
    <PaneHeader title="Helper Bot" subtitle="@helper_bot" backHref="#">
      {#snippet tabs()}
        <TabNav label="Bot sections" {items} />
      {/snippet}
    </PaneHeader>
  </div>
</Story>

<Story name="Narrow pane header" asChild>
  <div class="w-[360px] rounded-md border border-border">
    <PaneHeader title="Helper Bot" subtitle="@helper_bot" backHref="#">
      {#snippet tabs()}
        <TabNav label="Bot sections" {items} />
      {/snippet}
    </PaneHeader>
  </div>
</Story>
