<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { formatFileSize, type AttachmentsState } from './attachments.svelte';
  import { uploadPercentage, type AttachmentSubmissionStatus } from './submission.svelte';

  let {
    attachments,
    disabled,
    getSubmissionStatus,
    onremove
  }: {
    attachments: AttachmentsState;
    disabled: boolean;
    getSubmissionStatus: (file: File) => AttachmentSubmissionStatus | null;
    onremove: (index: number) => void;
  } = $props();

  function uploadStatusLabel(status: AttachmentSubmissionStatus): string {
    if (status.phase === 'preparing') return m('composer.upload.preparing');
    if (status.phase === 'failed') return m('composer.upload.failed');
    if (status.phase === 'uploaded') return m('composer.upload.uploaded');
    return m('composer.upload.uploading', { percentage: uploadPercentage(status) ?? 0 });
  }
</script>

{#if attachments.filesWithUrls.length > 0}
  <div class="flex flex-wrap gap-2">
    {#each attachments.filesWithUrls as { file, url }, index (url)}
      {@const submissionStatus = getSubmissionStatus(file)}
      {@const percentage = submissionStatus ? uploadPercentage(submissionStatus) : null}
      <div
        class="flex w-72 max-w-full items-center gap-2 rounded-md bg-surface p-2 text-sm"
        data-testid="composer-attachment-preview"
      >
        <div class="relative h-12 w-12 shrink-0 overflow-hidden rounded-md">
          {#if file.type.startsWith('image/')}
            <img src={url} alt={file.name} class="h-full w-full object-cover" />
          {:else if file.type.startsWith('video/')}
            <!-- Browser renders the first frame as a thumbnail from the object URL. -->
            <video
              data-testid="video-attachment-preview"
              src="{url}#t=0.1"
              preload="metadata"
              muted
              class="h-full w-full object-cover"
            ></video>
          {:else if file.type.startsWith('audio/')}
            <div
              data-testid="audio-attachment-preview"
              class="flex h-full w-full items-center justify-center bg-surface-strong"
            >
              <span class="iconify icon-[uil--music] text-lg text-muted"></span>
            </div>
          {:else}
            <div
              data-testid="file-attachment-preview"
              class="flex h-full w-full items-center justify-center bg-surface-strong"
            >
              <span class="text-xs text-muted">{file.name.split('.').pop()}</span>
            </div>
          {/if}
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center justify-between gap-2">
            <span class="truncate font-medium text-text" title={file.name}>{file.name}</span>
            <button
              type="button"
              onclick={() => onremove(index)}
              {disabled}
              class="flex h-5 w-5 shrink-0 cursor-pointer items-center justify-center rounded-full text-muted transition-[background-color,color] enabled:hover:bg-surface-strong enabled:hover:text-danger disabled:cursor-not-allowed disabled:opacity-50"
              aria-label={m('composer.upload.remove', { filename: file.name })}
              title={m('composer.upload.remove', { filename: file.name })}
            >
              <span class="iconify icon-[uil--times]"></span>
            </button>
          </div>
          <div
            class={[
              'mt-0.5 truncate text-xs',
              submissionStatus?.phase === 'failed' ? 'text-danger' : 'text-muted'
            ]}
          >
            {submissionStatus ? uploadStatusLabel(submissionStatus) : formatFileSize(file.size)}
          </div>
          <div
            data-testid="attachment-upload-progress"
            class={[
              'mt-1 h-1.5 overflow-hidden rounded-full bg-surface-strong',
              !submissionStatus && 'invisible'
            ]}
            role={submissionStatus ? 'progressbar' : undefined}
            aria-hidden={submissionStatus ? undefined : 'true'}
            aria-label={submissionStatus ? file.name : undefined}
            aria-valuemin={submissionStatus ? 0 : undefined}
            aria-valuemax={submissionStatus ? 100 : undefined}
            aria-valuenow={percentage ?? undefined}
            aria-valuetext={submissionStatus ? uploadStatusLabel(submissionStatus) : undefined}
          >
            {#if percentage !== null}
              <div
                class={[
                  'h-full rounded-full',
                  submissionStatus?.phase === 'failed' ? 'bg-danger' : 'bg-action'
                ]}
                style:width={`${percentage}%`}
              ></div>
            {/if}
          </div>
        </div>
      </div>
    {/each}
  </div>
{/if}
