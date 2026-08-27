<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Pill from './Pill.svelte';

  const componentDescription = `
    Use Pill for compact status, source, or category labels. Pills should describe state rather than
    act like buttons; use ToggleChip or Button when the element changes something.
  `.trim();

  const { Story } = defineMeta({
    title: 'UI/Pill',
    component: Pill,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: { component: componentDescription }
      }
    }
  });
</script>

<script lang="ts">
  const tones = ['success', 'danger', 'action', 'neutral', 'muted', 'subtle', 'server'] as const;
</script>

<Story name="All tones" asChild>
  <div class="flex flex-wrap gap-2">
    {#each tones as tone (tone)}
      <Pill {tone}>{tone}</Pill>
    {/each}
  </div>
</Story>

<Story name="Dimmed" asChild>
  <div class="flex flex-wrap gap-2">
    {#each tones as tone (tone)}
      <Pill {tone} dimmed>{tone}</Pill>
    {/each}
  </div>
</Story>

<Story name="In context" asChild>
  <div class="flex items-center gap-2">
    <span>Override:</span>
    <Pill tone="success">Allow from space</Pill>
    <Pill tone="danger" dimmed>Inherited Allow (overridden)</Pill>
  </div>
</Story>
