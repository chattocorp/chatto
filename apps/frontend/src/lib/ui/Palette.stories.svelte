<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Palette from './Palette.svelte';
  import { Button } from './form';

  const { Story } = defineMeta({
    title: 'UI/Palette',
    component: Palette,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Shared responsive shell for Quick Finder, notifications, and other compact app-wide palettes.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let modalVisible = $state(false);
  let anchoredVisible = $state(false);
</script>

{#snippet exampleContent()}
  <div class="menu-section px-3 py-2 font-medium">Palette heading</div>
  <div class="menu-section">
    <nav class="sidebar-nav">
      <button type="button" class="sidebar-item">First destination</button>
      <button type="button" class="sidebar-item">Second destination</button>
    </nav>
  </div>
{/snippet}

<Story name="Modal" asChild>
  <Button onclick={() => (modalVisible = true)}>Open modal palette</Button>
  <Palette
    visible={modalVisible}
    ariaLabel="Example modal palette"
    onclose={() => (modalVisible = false)}
  >
    {@render exampleContent()}
  </Palette>
</Story>

<Story name="Anchored" asChild>
  <div class="flex flex-col items-start gap-3">
    <Button onclick={() => (anchoredVisible = true)}>Open anchored palette</Button>
    <Palette
      visible={anchoredVisible}
      presentation="anchored"
      anchor={{ top: 48, bottom: 88, left: 24 }}
      ariaLabel="Example anchored palette"
      onclose={() => (anchoredVisible = false)}
    >
      {@render exampleContent()}
    </Palette>
  </div>
</Story>
