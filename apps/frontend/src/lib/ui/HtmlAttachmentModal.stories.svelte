<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import HtmlAttachmentModal from './HtmlAttachmentModal.svelte';

  const { Story } = defineMeta({
    title: 'UI/HTML attachment modal',
    component: HtmlAttachmentModal,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let open = $state(false);
  let previewUrl = $state<string | null>(null);
  const documentUrl =
    'data:text/html,' +
    encodeURIComponent(
      '<!doctype html><html lang="en"><title>Project report</title><body style="font:18px system-ui;padding:3rem"><h1>Project report</h1><p>This shared document is displayed in a sandbox.</p></body></html>'
    );
</script>

<Story name="Preview consent" asChild>
  <button
    type="button"
    class="btn-action"
    onclick={() => {
      previewUrl = null;
      open = true;
    }}>Open HTML attachment</button
  >
  {#if open}
    <HtmlAttachmentModal
      filename="project-report.html"
      contentType="text/html"
      size={24576}
      downloadUrl={documentUrl}
      {previewUrl}
      onpreview={() => (previewUrl = documentUrl)}
      ondownload={() => {}}
      onclose={() => (open = false)}
    />
  {/if}
</Story>
