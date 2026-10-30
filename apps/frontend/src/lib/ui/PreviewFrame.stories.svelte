<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import { expect, waitFor } from 'storybook/test';
  import MessageContent from '$lib/components/MessageContent.svelte';
  import TipTapEditor from '$lib/components/composer/TipTapEditor.svelte';
  import LinkPreviewCard from '$lib/components/LinkPreviewCard.svelte';
  import ComposerLinkPreviewCard from '$lib/components/composer/ComposerLinkPreviewCard.svelte';
  import ComposerAttachmentPreviews from '$lib/components/composer/ComposerAttachmentPreviews.svelte';
  import { AttachmentsState } from '$lib/components/composer/attachments.svelte';
  import LinkHoverContent from '$lib/components/messages/LinkHoverContent.svelte';
  import VideoPlayer from '$lib/components/chat/VideoPlayer.svelte';
  import { VideoProcessingStatus } from '@chatto/client/timeline/messageAttachments';
  import LoadingFog from './LoadingFog.svelte';
  import coast from '../../../e2e/fixtures/brighton.jpg?url';

  const { Story } = defineMeta({
    title: 'UI/Preview frames',
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Shared preview-frame, preview-content, and preview-thumbnail primitives. Keep native links, buttons, and media controls; the primitives supply appearance only. All images in these examples are local fixtures.'
        }
      }
    }
  });
  const preview = {
    url: 'https://example.test/coast',
    siteName: 'Field notes',
    title: 'A weekend by the sea',
    description: 'Coastal walks and a fresh perspective on a familiar place.',
    imageUrl: coast
  };
  const codeExample = '```ruby\nIO.puts("moo")\n```';
</script>

<script lang="ts">
  // Local fixtures avoid uploads, object URL lifetimes, and third-party requests.
  const attachments = new AttachmentsState(() => ({
    maxUploadSize: 10_000_000,
    videoProcessingEnabled: true
  }));
  attachments.restore([
    {
      file: new File(['image'], 'Coast.jpg', { type: 'image/jpeg' }),
      url: coast,
      description: 'Waves along the seafront'
    },
    {
      file: new File(['document'], 'Design notes.pdf', { type: 'application/pdf' }),
      url: 'https://example.test/design.pdf',
      description: ''
    }
  ]);
</script>

{#snippet imagePreview(shape = 'aspect-video')}
  <button type="button" class="block max-w-full preview-frame" aria-label="View coastal photograph">
    <img
      src={coast}
      alt="Waves along the Brighton seafront"
      class={['w-full preview-content object-cover', shape]}
    />
  </button>
{/snippet}

{#snippet filePreview(filename = 'Design notes.pdf')}
  <div class="flex min-w-0 items-center gap-3 preview-frame p-3">
    <div class="flex size-10 shrink-0 items-center justify-center preview-thumbnail">
      <span class="iconify icon-[uil--file-alt] text-xl text-muted" aria-hidden="true"></span>
    </div>
    <div class="min-w-0 flex-1 text-sm">
      <div class="truncate font-medium">{filename}</div>
      <div class="text-muted">PDF · 240 KB</div>
    </div>
    <button type="button" class="btn-secondary attachment-action-button" aria-label="View document">
      <span class="iconify icon-[uil--expand-alt]" aria-hidden="true"></span>
    </button>
  </div>
{/snippet}

<Story
  name="Preview family"
  asChild
  play={async ({ canvasElement }) => {
    const frame = canvasElement.querySelector('.preview-frame');
    await expect(frame).not.toBeNull();
    await expect(getComputedStyle(frame!).borderTopWidth).toBe('1px');
    await expect(getComputedStyle(frame!).paddingTop).toBe('4px');
  }}
>
  <div class="grid max-w-4xl grid-cols-1 items-start gap-6 p-4 sm:grid-cols-2">
    <LinkPreviewCard {preview} showDismiss={false} />
    <div class="flex min-w-0 flex-col gap-4">
      {@render imagePreview()}
      {@render filePreview()}
      <div class="preview-surface max-w-full">
        <LinkHoverContent
          href="https://chat.example.test/chat/-/development"
          local={{
            name: 'Development',
            kind: 'room',
            server: 'Chatto community',
            description: 'Build together. Share ideas and work in progress.'
          }}
        />
      </div>
    </div>
  </div>
</Story>

<Story
  name="Code in messages and composer"
  asChild
  play={async ({ canvasElement }) => {
    await waitFor(() => expect(canvasElement.querySelectorAll('pre')).toHaveLength(2));
    const frames = [...canvasElement.querySelectorAll('pre')];
    for (const frame of frames) {
      await expect(frame.textContent).toContain('IO.puts("moo")');
      await expect(getComputedStyle(frame).paddingTop).toBe('4px');
      await expect(getComputedStyle(frame).borderTopWidth).toBe('1px');
    }
    await expect(getComputedStyle(frames[0]).backgroundColor).toBe(
      getComputedStyle(frames[1]).backgroundColor
    );
    await expect(getComputedStyle(frames[0].querySelector('code')!).backgroundColor).toBe(
      getComputedStyle(frames[1].querySelector('code')!).backgroundColor
    );
  }}
>
  <div class="flex max-w-xl flex-col gap-5 p-4">
    <div>
      <p class="mb-2 text-sm text-muted">Posted code</p>
      <MessageContent body={codeExample} />
    </div>
    <div>
      <p class="mb-2 text-sm text-muted">Visual composer</p>
      <TipTapEditor onReady={(editor) => editor.setContent(codeExample)} />
    </div>
    <div>
      <p class="mb-2 text-sm text-muted">Link attachment in composer</p>
      <ComposerLinkPreviewCard {preview} onDismiss={() => {}} />
    </div>
  </div>
</Story>

<Story name="Image shapes and thumbnails" asChild>
  <div class="flex flex-wrap items-start gap-4 p-4">
    <div class="w-72 max-w-full">{@render imagePreview()}</div>
    <div class="w-40 max-w-full">{@render imagePreview('aspect-[3/4]')}</div>
    <div class="w-40 max-w-full">{@render imagePreview('aspect-square')}</div>
    <div class="flex gap-2">
      {#each [10, 12, 16] as size (size)}
        <img
          src={coast}
          alt="Coast thumbnail"
          class="preview-thumbnail object-cover"
          style:width={`${size / 4}rem`}
          style:height={`${size / 4}rem`}
        />
      {/each}
    </div>
  </div>
</Story>

<Story name="Composer rows" asChild>
  <div class="flex max-w-md flex-col gap-3 p-4">
    <ComposerLinkPreviewCard {preview} onDismiss={() => {}} />
    <ComposerLinkPreviewCard preview={{ ...preview, imageUrl: null }} onDismiss={() => {}} />
    <ComposerAttachmentPreviews
      {attachments}
      disabled={false}
      getSubmissionStatus={() => null}
      onremove={() => {}}
      ondescription={() => {}}
    />
    {@render filePreview('A very long filename that must stay inside the composer.pdf')}
  </div>
</Story>

<Story name="Loading unavailable and video states" asChild>
  <div class="flex flex-wrap items-start gap-4 p-4">
    <div class="preview-surface max-w-full"><LinkHoverContent href={preview.url} loading /></div>
    <div class="preview-surface max-w-full"><LinkHoverContent href={preview.url} /></div>
    <div class="w-64 max-w-full preview-frame">
      <LoadingFog class="h-36 w-full preview-content" />
    </div>
    <VideoPlayer
      status={VideoProcessingStatus.Processing}
      variants={[]}
      filename="Coast.mp4"
      width={320}
      height={180}
    />
    <VideoPlayer
      status={VideoProcessingStatus.Failed}
      variants={[]}
      filename="Coast.mp4"
      width={320}
      height={180}
    />
  </div>
</Story>

<Story name="Depth settings" asChild>
  <div class="flex flex-wrap items-start gap-5 p-4">
    {#each [{ name: 'Flat', strength: 0, width: 1 }, { name: 'Kinda 3D', strength: 0.75, width: 1 }, { name: 'Very 3D', strength: 1.75, width: 1.5 }] as mode (mode.name)}
      <div
        class="flex w-64 max-w-full flex-col gap-3"
        style:--depth-strength={mode.strength}
        style:--depth-width={mode.width}
      >
        <p class="text-sm font-medium">{mode.name}</p>
        {@render imagePreview()}
        {@render filePreview()}
        <LinkPreviewCard {preview} showDismiss={false} />
      </div>
    {/each}
  </div>
</Story>

<Story name="Narrow and right to left" asChild>
  <div class="flex w-64 max-w-full flex-col gap-3 p-3" dir="rtl">
    <LinkPreviewCard {preview} showDismiss={false} />
    <ComposerLinkPreviewCard {preview} onDismiss={() => {}} />
    {@render imagePreview('aspect-square')}
    {@render filePreview('A very long filename that must not widen this card.pdf')}
  </div>
</Story>
