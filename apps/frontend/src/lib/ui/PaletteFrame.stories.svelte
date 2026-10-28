<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PaletteFrame from './PaletteFrame.svelte';
  import PaletteSearch from './PaletteSearch.svelte';
  import MenuSection from './MenuSection.svelte';

  const { Story } = defineMeta({
    title: 'UI/PaletteFrame',
    component: PaletteFrame,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let closed = $state(false);
</script>

<Story name="Responsive frame" asChild>
  <div class="h-[600px]">
    {#if closed}
      <button class="btn" onclick={() => (closed = false)}>Open palette</button>
    {:else}
      <PaletteFrame title="Quick switcher" onBack={() => (closed = true)}>
        <PaletteSearch label="Search" />
        <MenuSection class="max-h-80 min-h-0 overflow-y-auto max-md:max-h-none max-md:flex-1">
          {#each Array.from({ length: 40 }, (_, index) => index) as index (index)}
            <button type="button" class="command-palette-result">Conversation {index + 1}</button>
          {/each}
        </MenuSection>
      </PaletteFrame>
    {/if}
  </div>
</Story>
