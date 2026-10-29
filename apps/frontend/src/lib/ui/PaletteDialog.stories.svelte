<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';
  import PaletteDialog from './PaletteDialog.svelte';
  import PaletteSearch from './PaletteSearch.svelte';
  import MenuSection from './MenuSection.svelte';

  const { Story } = defineMeta({
    title: 'UI/PaletteDialog',
    component: PaletteDialog,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let visible = $state(false);
</script>

<Story name="Responsive palette" asChild>
  <Button onclick={() => (visible = true)}>Open palette</Button>
  <PaletteDialog bind:visible title="Keyboard shortcuts">
    {#snippet children(close)}
      <PaletteSearch label="Search" placeholder="Keyboard shortcuts">
        <button type="button" class="keycap hidden md:inline-flex" title="Close" onclick={close}
          >Esc</button
        >
      </PaletteSearch>
      <MenuSection>
        <p class="command-palette-heading">App</p>
        <div class="command-palette-row justify-between">
          <span>Open quick switcher</span><kbd class="keycap">⌘ K</kbd>
        </div>
      </MenuSection>
    {/snippet}
  </PaletteDialog>
</Story>
