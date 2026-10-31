<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ZoomableImage from './ZoomableImage.svelte';

  const { Story } = defineMeta({
    title: 'UI/ZoomableImage',
    component: ZoomableImage,
    tags: ['autodocs']
  });
  const src = `data:image/svg+xml,${encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="400" height="1200">' +
      Array.from(
        { length: 8 },
        (_, i) =>
          `<rect x="40" y="${i * 150 + 20}" width="320" height="100" rx="12" fill="#243b53" stroke="#66c2df"/><text x="200" y="${i * 150 + 80}" text-anchor="middle" fill="white" font-size="24">Checkpoint ${i + 1}</text>`
      ).join('') +
      '</svg>'
  )}`;
</script>

<Story name="Image viewer" asChild>
  <div class="h-120 w-full max-w-2xl">
    <ZoomableImage {src} alt="Eight checkpoints" onerror={() => {}} />
  </div>
</Story>
