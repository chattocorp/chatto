<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, userEvent, within } from 'storybook/test';
  import PaletteSearch from './PaletteSearch.svelte';

  const { Story } = defineMeta({
    title: 'UI/PaletteSearch',
    component: PaletteSearch,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let query = $state('');
</script>

<Story
  name="Search with keyboard hint"
  asChild
  play={async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(canvas.getByRole('textbox', { name: 'Search' }), 'bold');
    await expect(canvas.getByLabelText('Current query')).toHaveTextContent('bold');
  }}
>
  <div class="command-palette">
    <PaletteSearch label="Search" placeholder="Keyboard shortcuts" bind:value={query}>
      <kbd class="keycap">Esc</kbd>
    </PaletteSearch>
  </div>
  <output aria-label="Current query">{query}</output>
</Story>

<Story name="Loading" args={{ label: 'Search', value: 'project', loading: true }} />
