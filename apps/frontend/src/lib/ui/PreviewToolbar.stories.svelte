<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PreviewToolbar from './PreviewToolbar.svelte';
  import AttachmentActions from './AttachmentActions.svelte';
  import PreviewAction from './PreviewAction.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import SocialPostEmbed from '$lib/components/SocialPostEmbed.svelte';
  import { VideoProcessingStatus } from '@chatto/client/timeline/messageAttachments';
  import VideoPlayer from '$lib/components/chat/VideoPlayer.svelte';
  import FullscreenVideoOverlay from '$lib/components/chat/FullscreenVideoOverlay.svelte';
  import clip from '$lib/test-utils/fixtures/wide-preview.mp4?url';
  import coast from '../../../e2e/fixtures/brighton.jpg?url';

  const { Story } = defineMeta({ title: 'UI/PreviewToolbar', component: PreviewToolbar });
</script>

{#snippet previews()}
  <div class="group/preview relative w-64 max-w-full preview-frame">
    <img src={coast} alt="Brighton coast" class="h-48 w-full preview-content object-cover" />
    <PreviewToolbar>
      <AttachmentActions editLabel="Edit description" onEdit={() => {}} onDelete={() => {}} />
    </PreviewToolbar>
  </div>
  <LinkPreviewCard
    preview={{
      url: 'https://example.test',
      title: 'Chatto Preview Next',
      siteName: 'Chatto Preview Next',
      description: 'Come join our community!',
      imageUrl: null
    }}
    showDismiss={false}
    canDelete
  />
  <SocialPostEmbed
    url="https://example.test/post"
    post={{
      provider: 'bluesky',
      url: 'https://example.test/post',
      author: { displayName: 'Coastal notes', handle: 'coast.example.test' },
      text: 'A quiet morning by the sea.',
      images: [{ url: coast, alt: 'Brighton coast' }]
    }}
    showDismiss={false}
    onDelete={() => {}}
  />
{/snippet}

<Story name="Preview family" asChild>
  <div class="flex max-w-md flex-col items-start gap-4">
    {@render previews()}
  </div>
</Story>

<Story name="Narrow RTL" asChild>
  <div dir="rtl" class="flex w-64 max-w-full flex-col items-start gap-4">
    {@render previews()}
  </div>
</Story>

<Story name="Inline row" asChild>
  <PreviewToolbar layout="row">
    {#snippet viewing()}<PreviewAction label="View file" icon="icon-[uil--expand-alt]" />{/snippet}
    <AttachmentActions editLabel="Edit description" onEdit={() => {}} onDelete={() => {}} />
  </PreviewToolbar>
</Story>

<Story name="Animation controls" asChild>
  <div class="w-64 max-w-full preview-frame">
    <img src={coast} alt="Brighton coast" class="h-48 w-full preview-content object-cover" />
    <PreviewToolbar>
      {#snippet viewing()}<PreviewAction
          label="Pause animation"
          icon="icon-[uil--pause]"
          aria-pressed={false}
        />{/snippet}
    </PreviewToolbar>
  </div>
</Story>

<Story name="Video controls" asChild>
  <FullscreenVideoOverlay />
  <div class="flex max-w-full flex-wrap items-start gap-6">
    {#each [480, 240, 180] as width (width)}
      <div style:width={`${width}px`} class="max-w-full">
        <VideoPlayer
          status={VideoProcessingStatus.Completed}
          filename="Coast.mp4"
          fallbackUrl={clip}
          width={1280}
          height={720}
        >
          {#snippet actions()}
            <AttachmentActions editLabel="Edit description" onEdit={() => {}} onDelete={() => {}} />
          {/snippet}
        </VideoPlayer>
      </div>
    {/each}
  </div>
</Story>

<Story name="GIF preview" asChild>
  <VideoPlayer
    status={VideoProcessingStatus.Completed}
    filename="Coast.gif"
    autoLoop
    variants={[{ url: clip, width: 640, height: 360, quality: '360p', size: 1024 }]}
    onOpen={() => {}}
  />
</Story>
