<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import StatCard from './StatCard.svelte';

  const componentDescription = `
  Compact metric card for admin dashboards. Use it for countable operational
  facts that benefit from quick comparison across a row. Tone should describe
  the metric category or health, not decorate arbitrary numbers.
  `.trim();

  const { Story } = defineMeta({
    title: 'Admin/StatCard',
    component: StatCard,
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
  name="Tones"
  asChild
  parameters={{
    docs: {
      description: {
        story:
          'The four tones cover neutral totals, healthy counts, warning-level attention, and failure/error metrics.'
      }
    }
  }}
>
  <div class="grid max-w-4xl grid-cols-1 gap-4 md:grid-cols-4">
    <StatCard
      color="action"
      icon="iconify uil--users-alt"
      value={713}
      label="Users"
      subtitle="+12 this week"
    />
    <StatCard color="success" icon="iconify uil--building" value={5} label="Spaces" />
    <StatCard
      color="warning"
      icon="iconify uil--message"
      value="48,221"
      label="Messages"
      subtitle="all-time"
    />
    <StatCard
      color="danger"
      icon="iconify uil--exclamation-triangle"
      value={3}
      label="Failed jobs"
      subtitle="last 24h"
    />
  </div>
</Story>
