<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import StatCard from './StatCard.svelte';

  const componentDescription = `
  Compact metric card for operational dashboards. Use it for countable facts
  that benefit from quick comparison across a row. Tone describes the metric
  category or health, not arbitrary decoration.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/Stat card',
    component: StatCard,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<Story name="Tones" asChild>
  <div class="grid max-w-4xl grid-cols-1 gap-4 md:grid-cols-4">
    <StatCard
      color="action"
      icon="iconify icon-[uil--users-alt]"
      value={713}
      label="Users"
      subtitle="+12 this week"
    />
    <StatCard color="success" icon="iconify icon-[uil--building]" value={5} label="Spaces" />
    <StatCard
      color="warning"
      icon="iconify icon-[uil--message]"
      value="48,221"
      label="Messages"
      subtitle="all-time"
    />
    <StatCard
      color="danger"
      icon="iconify icon-[uil--exclamation-triangle]"
      value={3}
      label="Failed jobs"
      subtitle="last 24h"
    />
  </div>
</Story>
