<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AttachmentModal from './AttachmentModal.svelte';
  import AttachmentPreview from './AttachmentPreview.svelte';
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
      downloadUrl={images[index].url}
      {index}
      count={images.length}
      onnavigate={(direction) => (index = (index + direction + images.length) % images.length)}
      ondownload={() => {}}
      onclose={() => (open = false)}
    >
      <AttachmentPreview
        item={images[index]}
        serverId="story"
        url={images[index].url}
        busy={false}
        onpreview={() => {}}
        onerror={async () => null}
      />
    </AttachmentModal>
  {/if}
</Story>
<Story name="Download without preview" asChild>
  <button class="btn-action" onclick={() => (documentOpen = true)}>Open file</button>
  {#if documentOpen}
    <AttachmentModal
      filename="Project archive.zip"
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
