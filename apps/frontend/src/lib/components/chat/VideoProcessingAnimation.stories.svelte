<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import VideoProcessingAnimation from './VideoProcessingAnimation.svelte';

  const { Story } = defineMeta({
    title: 'Chat/Video processing animation',
    component: VideoProcessingAnimation,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Animated WebGL picture shown while a video is queued or being prepared. Pass progress from 0 to 1 to resolve the simulated TV signal; omit it for a one-way ambient refinement.'
        }
      }
    }
  });
</script>

<Story name="Processing" asChild>
  <div class="aspect-video w-full max-w-[480px] overflow-hidden rounded-lg border border-border">
    <VideoProcessingAnimation label="Processing video…" />
  </div>
</Story>

<Story name="Queued" asChild>
  <div class="aspect-video w-full max-w-[480px] overflow-hidden rounded-lg border border-border">
    <VideoProcessingAnimation label="Video queued for processing…" />
  </div>
</Story>

<Story name="Narrow" asChild>
  <div class="aspect-video w-64 overflow-hidden rounded-lg border border-border">
    <VideoProcessingAnimation label="Processing video…" />
  </div>
</Story>

<Story name="Signal fidelity" asChild>
  <div class="flex max-w-[720px] flex-col gap-3 sm:flex-row">
    <div class="aspect-video min-w-0 flex-1 overflow-hidden rounded-lg border border-border">
      <VideoProcessingAnimation label="Starting…" progress={0} />
    </div>
    <div class="aspect-video min-w-0 flex-1 overflow-hidden rounded-lg border border-border">
      <VideoProcessingAnimation label="Halfway…" progress={0.5} />
    </div>
    <div class="aspect-video min-w-0 flex-1 overflow-hidden rounded-lg border border-border">
      <VideoProcessingAnimation label="Finishing…" progress={1} />
    </div>
  </div>
</Story>
