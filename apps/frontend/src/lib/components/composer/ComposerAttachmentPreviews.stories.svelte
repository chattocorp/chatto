<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ComposerAttachmentPreviews from './ComposerAttachmentPreviews.svelte';
  import { AttachmentsState } from './attachments.svelte';

  const { Story } = defineMeta({
    title: 'Composer/Attachment preparation',
    component: ComposerAttachmentPreviews
  });
</script>

<script lang="ts">
  const attachments = new AttachmentsState(() => ({
    maxUploadSize: 25 * 1024 * 1024,
    videoProcessingEnabled: true
  }));
  attachments.filesWithUrls = [
    {
      file: new File(['fixture'], 'Holiday video.mp4', { type: 'video/mp4' }),
      url: '',
      description: '',
      preparation: { phase: 'compressing', progress: 0.42, controller: new AbortController() }
    }
  ];
</script>

<Story name="Compressing" asChild>
  <ComposerAttachmentPreviews
    {attachments}
    disabled={false}
    getSubmissionStatus={() => null}
    onremove={(index) => attachments.removeFile(index)}
    ondescription={() => {}}
  />
</Story>
