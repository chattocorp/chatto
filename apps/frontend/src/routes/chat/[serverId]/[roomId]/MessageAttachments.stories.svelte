<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import MessageAttachments from './MessageAttachments.svelte';
  import type { MessageAttachmentView } from '@chatto/client/timeline/messageAttachments';
  import { VideoProcessingStatus } from '@chatto/client/timeline/messageAttachments';
  import wideClip from '$lib/test-utils/fixtures/wide-preview.mp4?url';
  import tallClip from '$lib/test-utils/fixtures/tall-preview.mp4?url';

  const { Story } = defineMeta({
    title: 'Messages/Image attachments',
    component: MessageAttachments,
    tags: ['autodocs']
  });

  /** Local images make empty space and cropping visible without an asset server. */
  function image(id: string, width: number, height: number): MessageAttachmentView {
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}"><rect width="100%" height="100%" fill="#276d81"/><path d="M0 0L${width} ${height}M${width} 0L0 ${height}" stroke="#7fc3cb" stroke-width="4"/><circle cx="${width / 2}" cy="${height / 2}" r="${Math.min(width, height) / 4}" fill="#edc77c"/></svg>`;
    return {
      id,
      filename: `${id}.svg`,
      contentType: 'image/svg+xml',
      width,
      height,
      assetUrl: {
        url: `data:image/svg+xml,${encodeURIComponent(svg)}`,
        expiresAt: '2099-01-01T00:00:00Z'
      },
      thumbnailAssetUrl: null,
      videoProcessing: null
    };
  }

  const landscape = image('landscape', 1600, 900);
  const wide = image('wide', 2000, 400);
  const portrait = image('portrait', 320, 1600);
  const square = image('square', 600, 600);

  /** Local clips exercise the real player, including intrinsic metadata sizing. */
  function clip(id: string, autoLoop: boolean, tall = false): MessageAttachmentView {
    const width = tall ? 120 : 600;
    const height = tall ? 600 : 200;
    const assetUrl = {
      url: tall ? tallClip : wideClip,
      expiresAt: '2099-01-01T00:00:00Z'
    };
    return {
      ...image(id, width, height),
      filename: `${id}.${autoLoop ? 'gif' : 'mp4'}`,
      contentType: autoLoop ? 'image/gif' : 'video/mp4',
      assetUrl,
      videoProcessing: {
        status: VideoProcessingStatus.Completed,
        width,
        height,
        durationMs: 1000,
        thumbnailAssetUrl: null,
        hlsMasterPlaylistUrl: null,
        sourceAvailable: true,
        reasonCode: null,
        variants: [{ quality: 'original', width, height, size: 1024, assetUrl }]
      }
    };
  }

  const wideImage = image('wide-image', 600, 200);
  const wideLoop = clip('wide-loop', true);
  const wideVideo = clip('wide-video', false);
  const tallLoop = clip('tall-loop', true, true);
  const rawWideVideo = { ...wideVideo, id: 'raw-video', videoProcessing: null };
</script>

<script lang="ts">
  import { provideServerScope } from '$lib/state/server/scope.svelte';
  import type { ServerConnection } from '@chatto/client/server/serverConnection';
  import type { ServerStateStore } from '@chatto/client/server/store';
  import type { ConnectAPIConfig } from '@chatto/client/api/connect';

  // Data URLs need no server requests; the scope supplies the component's API context.
  provideServerScope({
    serverId: 'storybook',
    connection: {
      getAPI: <T,>(factory: (config: ConnectAPIConfig) => T): T =>
        factory({
          serverId: 'storybook',
          queryScope: 'storybook',
          baseUrl: 'https://example.test/api/connect',
          bearerToken: null
        })
    } as ServerConnection,
    store: {} as ServerStateStore,
    isCurrent: () => true
  });
</script>

{#snippet attachment(attachments: MessageAttachmentView[])}
  <MessageAttachments
    {attachments}
    serverId="storybook"
    roomId="images"
    eventId={attachments[0].id}
  />
{/snippet}

<Story name="Single image proportions" asChild>
  <div class="flex max-w-xl flex-col items-start gap-4 p-4">
    {@render attachment([wide])}
    {@render attachment([landscape])}
    <div class="flex gap-4">
      {@render attachment([portrait])}
      {@render attachment([square])}
    </div>
  </div>
</Story>

<Story name="Narrow message" asChild>
  <div class="w-40 max-w-full p-4">
    {@render attachment([wide])}
    {@render attachment([landscape])}
    {@render attachment([square])}
  </div>
</Story>

<Story name="Gallery" asChild>
  <div class="max-w-xl p-4">
    {@render attachment([landscape, wide, portrait, square])}
  </div>
</Story>

<Story name="Wide image GIF and video" asChild>
  <div class="flex max-w-xl flex-col gap-4 p-4">
    <div>
      <p class="mb-2 text-muted">Image · 3:1</p>
      {@render attachment([wideImage])}
    </div>
    <div>
      <p class="mb-2 text-muted">Animated GIF · 3:1</p>
      {@render attachment([wideLoop])}
    </div>
    <div>
      <p class="mb-2 text-muted">Video · 3:1</p>
      {@render attachment([wideVideo])}
    </div>
    <div>
      <p class="mb-2 text-muted">Original video · 3:1</p>
      {@render attachment([rawWideVideo])}
    </div>
  </div>
</Story>

<Story name="Tall GIF with actions" asChild>
  <div class="w-64 max-w-full p-4">
    <MessageAttachments
      attachments={[tallLoop]}
      serverId="storybook"
      roomId="images"
      eventId="tall-loop"
      canDeleteAttachment
      canEditAttachmentDescription
    />
  </div>
</Story>
