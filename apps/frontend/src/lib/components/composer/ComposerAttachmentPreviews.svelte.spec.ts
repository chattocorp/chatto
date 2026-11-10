import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { AttachmentsState } from './attachments.svelte';
import { ComposerSubmissionState } from './submission.svelte';
import ComposerAttachmentPreviews from './ComposerAttachmentPreviews.svelte';

const compress = vi.hoisted(() => vi.fn());
vi.mock('$lib/attachments/compressVideo', () => ({ compressVideo: compress }));

describe('attachment preparation progress', () => {
  it('keeps the tile and progress element when compression transitions to upload', async () => {
    const deferred = Promise.withResolvers<File>();
    compress.mockReturnValueOnce(deferred.promise);
    const attachments = new AttachmentsState(() => ({
      maxUploadSize: 1,
      videoProcessingEnabled: true
    }));
    const submission = new ComposerSubmissionState({
      getAPI: vi.fn(),
      getMentionRoleStatus: () => 'ready',
      loadMentionRoles: async () => true,
      getMentionRoleNames: () => [],
      onPostSuccess: vi.fn()
    });
    const { container, getByRole } = render(ComposerAttachmentPreviews, {
      props: {
        attachments,
        disabled: false,
        getSubmissionStatus: (file) => submission.attachmentStatus(file),
        onremove: (index) => attachments.removeFile(index),
        ondescription: vi.fn()
      }
    });
    const work = attachments.stageFiles([new File(['xx'], 'source.mp4', { type: 'video/mp4' })]);
    await vi.waitFor(() => expect(compress).toHaveBeenCalled());
    compress.mock.calls.at(-1)![1].onProgress(0.42);
    await expect.element(getByRole('progressbar')).toHaveAttribute('aria-valuenow', '42');
    await expect
      .element(getByRole('progressbar'))
      .toHaveAttribute('aria-valuetext', 'Compressing video… 42%');
    const tile = container.querySelector('[data-testid="composer-attachment-preview"]');
    const progress = container.querySelector('[data-testid="attachment-upload-progress"]');
    const output = new File(['x'], 'compressed.mp4', { type: 'video/mp4' });
    deferred.resolve(output);
    await work;
    submission.updateAttachmentStatus({
      file: output,
      phase: 'uploading',
      committedBytes: 1,
      totalBytes: 2
    });
    await expect
      .element(getByRole('progressbar'))
      .toHaveAttribute('aria-valuetext', '50% uploaded');
    expect(container.querySelector('[data-testid="composer-attachment-preview"]')).toBe(tile);
    expect(container.querySelector('[data-testid="attachment-upload-progress"]')).toBe(progress);
    attachments.clear();
    await tick();
  });

  it('uses the existing remove action to cancel preparation', async () => {
    const deferred = Promise.withResolvers<File>();
    compress.mockReturnValueOnce(deferred.promise);
    const attachments = new AttachmentsState(() => ({
      maxUploadSize: 1,
      videoProcessingEnabled: true
    }));
    const { getByRole } = render(ComposerAttachmentPreviews, {
      props: {
        attachments,
        disabled: false,
        getSubmissionStatus: () => null,
        onremove: (index) => attachments.removeFile(index),
        ondescription: vi.fn()
      }
    });
    const work = attachments.stageFiles([new File(['xx'], 'source.mp4', { type: 'video/mp4' })]);
    const signal = attachments.filesWithUrls[0].preparation!.controller.signal;
    await getByRole('button', { name: 'Remove source.mp4' }).click();
    expect(signal.aborted).toBe(true);
    deferred.resolve(new File(['x'], 'compressed.mp4', { type: 'video/mp4' }));
    await work;
    expect(attachments.filesWithUrls).toHaveLength(0);
  });
});
