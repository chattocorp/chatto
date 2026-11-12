<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ImageZoomControls from './ImageZoomControls.svelte';

  const { Story } = defineMeta({
    title: 'UI/ImageZoomControls',
    component: ImageZoomControls,
    tags: ['autodocs']
  });
</script>

<Story name="Waiting for image" />
