<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AttachmentModal from './AttachmentModal.svelte';
  import AttachmentPreview from './AttachmentPreview.svelte';
  import { renderMarkdown } from '$lib/markdown';
  const { Story } = defineMeta({
    title: 'UI/Attachment viewer',
    component: AttachmentModal,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let open = $state(false);
  let index = $state(0);
  const images = ['#408cab', '#be764f'].map((color, i) => ({
    id: String(i),
    filename: `Landscape ${i + 1}.svg`,
    contentType: 'image/svg+xml',
    width: 800,
    height: 500,
    url:
      'data:image/svg+xml,' +
      encodeURIComponent(
        `<svg xmlns="http://www.w3.org/2000/svg" width="800" height="500"><rect width="800" height="500" fill="${color}"/><circle cx="400" cy="250" r="100" fill="white"/></svg>`
      )
  }));
  let documentOpen = $state(false);
  const archiveDescription =
    'Project files and reference material for the next release.\n\nIncludes source files, illustrations, and notes from the design review.';
  let markdownOpen = $state(false);
  const markdownSource =
    '# Project review\n\nA **rendered Markdown** attachment.\n\n' +
    '- Read the report\n- Download the original\n\n' +
    '| Feature | Status |\n| --- | --- |\n| Preview | Ready |\n\n' +
    '```ts\nconst preview = true;\n```\n\n' +
    Array.from(
      { length: 12 },
      (_, i) => `## Section ${i + 1}\n\nDocument content stays inside the scrolling preview.\n\n`
    ).join('');
  const markdownDocument = renderMarkdown(markdownSource);
</script>

<Story name="Image gallery" asChild>
  <button
    class="btn-action"
    onclick={() => {
      index = 0;
      open = true;
    }}>Open gallery</button
  >
  {#if open}
    <AttachmentModal
      filename={images[index].filename}
      contentType={images[index].contentType}
      description="A white circle on a coloured background."
      size={24000}
      imageViewer
      downloadUrl={images[index].url}
      {index}
      count={images.length}
      onnavigate={(direction) => (index = (index + direction + images.length) % images.length)}
      ondownload={() => {}}
      onclose={() => (open = false)}
    >
      {#key images[index].id}
        <AttachmentPreview
          item={images[index]}
          serverId="story"
          url={images[index].url}
          busy={false}
          zoomable
          onpreview={() => {}}
          onerror={async () => null}
        />
      {/key}
    </AttachmentModal>
  {/if}
</Story>
<Story name="Markdown document" asChild>
  <button class="btn-action" onclick={() => (markdownOpen = true)}>Open Markdown</button>
  {#if markdownOpen}
    <AttachmentModal
      filename="Project review.md"
      contentType="text/markdown"
      description="Notes from the project review."
      size={markdownSource.length}
      downloadUrl={'data:text/markdown,' + encodeURIComponent(markdownSource)}
      ondownload={() => {}}
      onclose={() => (markdownOpen = false)}
    >
      {#await markdownDocument then markdownHtml}
        <AttachmentPreview
          item={{
            id: 'markdown',
            filename: 'Project review.md',
            contentType: 'text/markdown',
            width: 0,
            height: 0
          }}
          serverId="story"
          url={null}
          busy={false}
          {markdownHtml}
          onpreview={() => {}}
          onerror={async () => null}
        />
      {/await}
    </AttachmentModal>
  {/if}
</Story>
<Story name="Download without preview" asChild>
  <button class="btn-action" onclick={() => (documentOpen = true)}>Open file</button>
  {#if documentOpen}
    <AttachmentModal
      filename="Project archive.zip"
      description={archiveDescription}
      contentType="application/zip"
      size={2048000}
      downloadUrl="data:application/zip,"
      ondownload={() => {}}
      onclose={() => (documentOpen = false)}
    >
      <AttachmentPreview
        item={{
          id: 'archive',
          filename: 'Project archive.zip',
          contentType: 'application/zip',
          width: 0,
          height: 0
        }}
        serverId="story"
        url={null}
        busy={false}
        onpreview={() => {}}
        onerror={async () => null}
      />
    </AttachmentModal>
  {/if}
</Story>
