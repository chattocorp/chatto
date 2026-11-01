<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import RenderedImage from './RenderedImage.svelte';
  const { Story } = defineMeta({ title: 'UI/RenderedImage', component: RenderedImage });
</script>

<script lang="ts">
  import MarkdownHtml from './MarkdownHtml.svelte';
  import { renderMarkdown } from '$lib/markdown';
  import RenderedImageViewerModal from '../../routes/chat/modals/RenderedImageViewerModal.svelte';
  import type { RenderedImageViewerModalState } from '$lib/modal';

  // Storybook has no app layout or router; mirror the modal owner for this preview.
  let modal = $state<RenderedImageViewerModalState>();
  const parameters = {
    sveltekit_experimental: {
      navigation: {
        pushState: (_url: string, state: App.PageState) => {
          if (state.modal?.type === 'renderedImageViewer') modal = state.modal;
        }
      }
    }
  };
  const source = 'flowchart LR\nA[Launch checks] --> B{Ready?}\nB -->|Yes| C[Depart]\nB -->|No| A';
  const html = renderMarkdown(
    `Code example:\n\n\u0060\u0060\u0060mermaid\n${source}\n\u0060\u0060\u0060\n\nVisual message:\n\n\u0060\u0060\u0060mermaid rendered\n${source}\n\u0060\u0060\u0060`
  );
  const aspectRatios = [
    { name: 'Landscape', width: 1200, height: 600 },
    { name: 'Portrait', width: 400, height: 800 },
    { name: 'Square', width: 500, height: 500 },
    { name: 'Very wide', width: 4000, height: 100 },
    { name: 'Very tall', width: 100, height: 4000 },
    { name: 'Small', width: 100, height: 60 }
  ].map(({ name, width, height }) => ({
    name,
    html: renderMarkdown(
      `\u0060\u0060\u0060svg rendered\n<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}"><rect width="100%" height="100%" fill="#164e63"/><circle cx="${width / 2}" cy="${height / 2}" r="${Math.min(width, height) * 0.3}" fill="#67e8f9"/></svg>\n\u0060\u0060\u0060`
    )
  }));
</script>

<Story name="Code and visual intent" {parameters} asChild>
  <div class="prose w-full max-w-2xl">
    {#await html then value}<MarkdownHtml html={value} />{/await}
  </div>
  {#if modal}
    <RenderedImageViewerModal {modal} onclose={() => (modal = undefined)} />
  {/if}
</Story>

<Story name="Aspect ratios" {parameters} asChild>
  <div class="prose w-full max-w-5xl">
    {#each aspectRatios as example (example.name)}
      <p>{example.name}</p>
      {#await example.html then value}<MarkdownHtml html={value} />{/await}
    {/each}
  </div>
  {#if modal}
    <RenderedImageViewerModal {modal} onclose={() => (modal = undefined)} />
  {/if}
</Story>
