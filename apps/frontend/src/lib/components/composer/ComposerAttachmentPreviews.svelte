<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import {
    formatFileSize,
    type AttachmentsState,
    type VideoPreparation
  } from './attachments.svelte';
  import { uploadPercentage, type AttachmentSubmissionStatus } from './submission.svelte';

  let {
    attachments,
    disabled,
    getSubmissionStatus,
    onremove,
    ondescription
  }: {
    attachments: AttachmentsState;
    disabled: boolean;
    getSubmissionStatus: (file: File) => AttachmentSubmissionStatus | null;
    onremove: (index: number) => void;
    ondescription: (index: number) => void;
  } = $props();

  function uploadStatusLabel(status: AttachmentSubmissionStatus): string {
    if (status.phase === 'preparing') return m('composer.upload.preparing');
    if (status.phase === 'failed') return m('composer.upload.failed');
    if (status.phase === 'uploaded') return m('composer.upload.uploaded');
    return m('composer.upload.uploading', { percentage: uploadPercentage(status) ?? 0 });
  }
  /** Preparation takes precedence until the tile is ready for upload. */
  function tileStatus(
    preparation: VideoPreparation | undefined,
    upload: AttachmentSubmissionStatus | null
  ) {
    if (preparation) {
      const percentage =
        preparation.phase === 'compressing' ? Math.round(preparation.progress * 100) : null;
      let statusLabel = m('composer.upload.preparing');
      if (preparation.phase === 'failed') statusLabel = m('composer.upload.prepare_failed');
      if (preparation.phase === 'compressing')
        statusLabel = m('composer.upload.compressing', { percentage: percentage ?? 0 });
      return { percentage, statusLabel, failed: preparation.phase === 'failed', hasProgress: true };
    }
    return {
      percentage: upload ? uploadPercentage(upload) : null,
      statusLabel: upload ? uploadStatusLabel(upload) : null,
      failed: upload?.phase === 'failed',
      hasProgress: upload !== null
    };
  }
</script>

{#if attachments.filesWithUrls.length > 0}
  <div class="flex flex-wrap gap-2">
    {#each attachments.filesWithUrls as { file, url, description, preparation }, index (url)}
      {@const { percentage, failed, statusLabel, hasProgress } = tileStatus(
        preparation,
        getSubmissionStatus(file)
      )}
      {@const descriptionActionLabel = description
        ? m('room.attachment.edit_description')
        : m('room.attachment.add_description')}
      <div class="w-72 max-w-full preview-frame text-sm" data-testid="composer-attachment-preview">
        <div class="flex min-w-0 items-center gap-2 preview-content px-2 py-1.5">
          <div class="relative h-12 w-12 shrink-0 preview-thumbnail">
            {#if file.type.startsWith('image/')}
              <img src={url} alt={description || file.name} class="h-full w-full object-cover" />
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
                class="flex h-full w-full items-center justify-center bg-surface-emphasized"
              >
                <span aria-hidden="true" class="iconify icon-[uil--music] text-lg text-muted"
                ></span>
              </div>
            {:else}
              <div
                data-testid="file-attachment-preview"
                class="flex h-full w-full items-center justify-center bg-surface-emphasized"
              >
                <span class="text-xs text-muted">{file.name.split('.').pop()}</span>
              </div>
            {/if}
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-1">
              <span class="min-w-0 flex-1 truncate font-medium text-text" title={file.name}
                >{file.name}</span
              >
              <div class="flex shrink-0 items-center gap-0.5">
                <button
                  type="button"
                  onclick={() => ondescription(index)}
                  {disabled}
                  class={[
                    'mini-icon-action h-5 w-5 items-center justify-center disabled:cursor-not-allowed disabled:opacity-50',
                    description && 'text-action hover:text-action'
                  ]}
                  aria-label={descriptionActionLabel}
                  title={descriptionActionLabel}
                >
                  <span class="iconify icon-[uil--file-edit-alt] text-sm" aria-hidden="true"></span>
                </button>
                <button
                  type="button"
                  onclick={() => onremove(index)}
                  {disabled}
                  class="mini-icon-action h-5 w-5 items-center justify-center enabled:hover:text-danger disabled:cursor-not-allowed disabled:opacity-50"
                  aria-label={m('composer.upload.remove', { filename: file.name })}
                  title={m('composer.upload.remove', { filename: file.name })}
                >
                  <span class="iconify icon-[uil--times] text-sm" aria-hidden="true"></span>
                </button>
              </div>
            </div>
            <div class={['mt-0.5 truncate text-xs', failed ? 'text-danger' : 'text-muted']}>
              {statusLabel ?? formatFileSize(file.size)}
            </div>
            <div
              data-testid="attachment-upload-progress"
              class={[
                'mt-1 h-1.5 overflow-hidden rounded-full bg-surface-emphasized',
                !hasProgress && 'invisible'
              ]}
              role={hasProgress ? 'progressbar' : undefined}
              aria-hidden={hasProgress ? undefined : 'true'}
              aria-label={hasProgress ? file.name : undefined}
              aria-valuemin={hasProgress ? 0 : undefined}
              aria-valuemax={hasProgress ? 100 : undefined}
              aria-valuenow={percentage ?? undefined}
              aria-valuetext={statusLabel ?? undefined}
            >
              {#if percentage !== null}
                <div
                  class={['h-full rounded-full', failed ? 'bg-danger' : 'bg-action']}
                  style:width={`${percentage}%`}
                ></div>
              {/if}
            </div>
          </div>
        </div>
      </div>
    {/each}
  </div>
{/if}
