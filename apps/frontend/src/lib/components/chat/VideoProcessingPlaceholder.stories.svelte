<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import VideoProcessingPlaceholder from './VideoProcessingPlaceholder.svelte';

  const { Story } = defineMeta({
    title: 'Chat/Video processing placeholder',
    component: VideoProcessingPlaceholder,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Shared loading fog fills the reserved video frame while a video is queued or being prepared. The visible label identifies the status.'
        }
      }
    }
  });
</script>

<Story name="Processing" asChild>
  <div class="aspect-video w-full max-w-[480px] overflow-hidden rounded-lg border border-border">
    <VideoProcessingPlaceholder label="Processing video…" />
  </div>
</Story>

<Story name="Queued" asChild>
  <div class="aspect-video w-full max-w-[480px] overflow-hidden rounded-lg border border-border">
    <VideoProcessingPlaceholder label="Video queued for processing…" />
  </div>
</Story>

<Story name="Narrow" asChild>
  <div class="aspect-video w-64 overflow-hidden rounded-lg border border-border">
    <VideoProcessingPlaceholder label="Processing video…" />
  </div>
</Story>
