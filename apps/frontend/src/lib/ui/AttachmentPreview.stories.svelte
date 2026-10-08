<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AttachmentPreview from './AttachmentPreview.svelte';
  import pdfUrl from '$lib/test-utils/fixtures/attachment.pdf?url';

  const { Story } = defineMeta({
    title: 'UI/Attachment preview',
    component: AttachmentPreview,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  const image = {
    id: 'image',
    filename: 'Landscape.svg',
    contentType: 'image/svg+xml',
    width: 800,
    height: 500,
    url:
      'data:image/svg+xml,' +
      encodeURIComponent(
        '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="500"><rect width="800" height="500" fill="#408cab"/><circle cx="400" cy="250" r="100" fill="white"/></svg>'
      )
  };
  const page = {
    id: 'page',
    filename: 'Report.html',
    contentType: 'text/html',
    width: 0,
    height: 0,
    url: null
  };
  const pdf = {
    id: 'pdf',
    filename: 'Report.pdf',
    contentType: 'application/pdf',
    width: 0,
    height: 0
  };
</script>

<Story name="Image" asChild>
  <div class="h-80">
    <AttachmentPreview
      item={image}
      serverId="story"
      url={image.url}
      busy={false}
      zoomable
      onpreview={() => {}}
      onerror={async () => null}
    />
  </div>
</Story>

<Story name="Native PDF" asChild>
  <div class="h-[70vh]">
    <AttachmentPreview
      item={pdf}
      serverId="story"
      url={pdfUrl}
      busy={false}
      onpreview={() => {}}
      onerror={async () => null}
    />
  </div>
</Story>

<Story name="PDF without native support" asChild>
  <div class="h-80">
    <AttachmentPreview
      item={pdf}
      serverId="story"
      url={pdfUrl}
      busy={false}
      pdfViewerEnabled={false}
      onpreview={() => {}}
      onerror={async () => null}
    />
  </div>
</Story>

<Story name="HTML consent" asChild>
  <div class="h-80">
    <AttachmentPreview
      item={page}
      serverId="story"
      url={null}
      busy={false}
      onpreview={() => {}}
      onerror={async () => null}
    />
  </div>
</Story>
