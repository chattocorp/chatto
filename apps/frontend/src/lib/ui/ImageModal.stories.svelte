<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ImageModal from './ImageModal.svelte';

  const { Story } = defineMeta({
    title: 'UI/Image modal',
    component: ImageModal,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  const svgImage = (label: string, colour: string) =>
    `data:image/svg+xml,${encodeURIComponent(
      `<svg xmlns="http://www.w3.org/2000/svg" width="960" height="640" viewBox="0 0 960 640"><rect width="960" height="640" fill="${colour}"/><circle cx="480" cy="280" r="150" fill="white" fill-opacity=".18"/><text x="480" y="540" fill="white" font-family="sans-serif" font-size="48" text-anchor="middle">${label}</text></svg>`
    )}`;

  const images = [
    { id: 'first', src: svgImage('Conference hall', '#0f766e'), filename: 'conference-hall.svg' },
    { id: 'second', src: svgImage('Project board', '#4338ca'), filename: 'project-board.svg' }
  ];

  let index = $state(0);
  let open = $state(false);
</script>

<Story name="Gallery" asChild>
  <div class="flex min-h-52 items-center justify-center rounded-lg bg-background p-6">
    <button type="button" class="btn-action" onclick={() => (open = true)}>Open gallery</button>
    {#if open}
      <ImageModal items={images} bind:index onclose={() => (open = false)} />
    {/if}
  </div>
</Story>
