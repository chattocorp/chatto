<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { Button } from '$lib/ui/form';
  import PaletteDialog from './PaletteDialog.svelte';

  const { Story } = defineMeta({
    title: 'UI/PaletteDialog',
    component: PaletteDialog,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let visible = $state(false);
</script>

<Story name="Compact palette" asChild>
  <Button onclick={() => (visible = true)}>Open palette</Button>
  <PaletteDialog bind:visible title="Keyboard shortcuts">
    {#snippet children(close)}
      <div class="menu-section">
        <div class="flex min-h-10 items-center gap-2 px-3 py-1.5">
          <input
            aria-label="Search"
            placeholder="Keyboard shortcuts"
            class="min-w-0 flex-1 bg-transparent text-[16px] text-text outline-none placeholder:text-muted"
          />
          <button type="button" class="keycap" title="Close" onclick={close}>Esc</button>
        </div>
      </div>
      <div class="menu-section">
        <p class="px-3 pt-2 pb-0.5 text-xs font-medium text-muted uppercase">App</p>
        <div class="flex min-h-8 items-center justify-between gap-2 px-2 py-1">
          <span>Open quick switcher</span><kbd class="keycap">⌘ K</kbd>
        </div>
      </div>
    {/snippet}
  </PaletteDialog>
</Story>
