<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Panel from './Panel.svelte';
  import { Button } from '$lib/ui/form';

  const componentDescription = `
  Bordered admin surface for grouped management content. Use \`Panel\` when a
  section needs a title band, optional summary copy, and optional header
  actions. It owns the canonical \`panel-shell panel-shell-raised\` container
  plus \`panel-header\` / \`panel-body\` treatment.
  `.trim();

  const { Story } = defineMeta({
    title: 'Admin/Panel',
    component: Panel,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component: componentDescription
        }
      }
    }
  });
</script>

<Story
  name="With header actions"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use header actions for small panel-scoped commands; keep primary page actions in the surrounding page header.'
      }
    }
  }}
>
  <div class="max-w-2xl">
    <Panel
      title="Spaces"
      subtitle="All spaces hosted on this instance"
      icon="iconify uil--building"
      count={5}
    >
      {#snippet actions()}
        <Button size="sm" variant="secondary">
          <span class="iconify uil--filter"></span>
          Filter
        </Button>
      {/snippet}

      <p class="text-sm text-muted">
        Panel content can be form sections, summary rows, or a table. Use <code>noPadding</code>
        when a child component owns its own edge-to-edge spacing.
      </p>
    </Panel>
  </div>
</Story>

<Story
  name="No padding"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'Use `noPadding` for tables and dense list components that need to meet the panel edge.'
      }
    }
  }}
>
  <div class="max-w-2xl">
    <Panel title="Recent activity" noPadding>
      <div class="divide-y divide-border text-sm">
        <div class="flex items-center justify-between px-4 py-3">
          <span>User registered</span>
          <span class="text-muted">2 minutes ago</span>
        </div>
        <div class="flex items-center justify-between px-4 py-3">
          <span>Room archived</span>
          <span class="text-muted">12 minutes ago</span>
        </div>
      </div>
    </Panel>
  </div>
</Story>
