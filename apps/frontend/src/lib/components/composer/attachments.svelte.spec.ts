import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AttachmentsState, type AttachmentLimits } from './attachments.svelte';
import { getToasts, toast } from '$lib/ui/toast';

const prepareFilesMock = vi.hoisted(() => vi.fn());

vi.mock('$lib/attachments/prepareFiles', () => ({
  prepareFiles: prepareFilesMock
}));

function imageFile(name = 'image.png', bytes = 3): File {
  return new File([new Uint8Array(bytes)], name, { type: 'image/png' });
}

function videoFile(name = 'clip.mp4', bytes = 3): File {
  return new File([new Uint8Array(bytes)], name, { type: 'video/mp4' });
}

function documentFile(name = 'document.pdf', bytes = 3): File {
  return new File([new Uint8Array(bytes)], name, { type: 'application/pdf' });
}

describe('AttachmentsState', () => {
  let limits: AttachmentLimits;
  let state: AttachmentsState;

  beforeEach(() => {
    limits = {
      videoProcessingEnabled: false,
      maxUploadSize: 25 * 1024 * 1024,
      maxVideoUploadSize: 25 * 1024 * 1024
    };
    state = new AttachmentsState(() => limits);
    toast.clear();
    prepareFilesMock.mockReset();
    prepareFilesMock.mockImplementation(async (files: File[]) => files);
    Object.defineProperty(URL, 'createObjectURL', {
      value: vi.fn((file: File) => `blob:${file.name}`),
      configurable: true
    });
    Object.defineProperty(URL, 'revokeObjectURL', {
      value: vi.fn(),
      configurable: true
    });
  });

  it('accepts non-media files', async () => {
    const file = documentFile();

    await state.stageFiles([file]);

    expect(state.selectedFiles).toEqual([file]);
  });

  it('stages prepared files and appends subsequent files', async () => {
    const first = imageFile('first.png');
    const second = imageFile('second.png');

    await state.stageFiles([first]);
    await state.stageFiles([second]);

    expect(state.filesWithUrls.map(({ file }) => file.name)).toEqual(['first.png', 'second.png']);
    expect(state.filesWithUrls.map(({ url }) => url)).toEqual(['blob:first.png', 'blob:second.png']);
  });

  it('rejects video files when video processing is disabled', async () => {
    await state.stageFiles([videoFile()]);

    expect(getToasts().map((t) => t.message)).toContain(
      'Video uploads are disabled on this server.'
    );
    expect(state.filesWithUrls).toEqual([]);
    expect(prepareFilesMock).not.toHaveBeenCalled();
  });

  it('accepts video files when video processing is enabled', async () => {
    limits.videoProcessingEnabled = true;
    const file = videoFile();

    await state.stageFiles([file]);

    expect(state.selectedFiles).toEqual([file]);
  });

  it('rejects files over the matching upload size limit', async () => {
    limits.maxUploadSize = 1;

    await state.stageFiles([imageFile('too-large.png', 2)]);

    expect(getToasts().map((t) => t.message).join('\n')).toContain('too-large.png is too large');
    expect(state.filesWithUrls).toEqual([]);
    expect(prepareFilesMock).not.toHaveBeenCalled();
  });

  it('uses the video-specific upload limit for videos', async () => {
    limits.videoProcessingEnabled = true;
    limits.maxUploadSize = 10;
    limits.maxVideoUploadSize = 1;

    await state.stageFiles([videoFile('too-large.mp4', 2)]);

    expect(getToasts().map((t) => t.message).join('\n')).toContain('too-large.mp4 is too large');
    expect(state.filesWithUrls).toEqual([]);
    expect(prepareFilesMock).not.toHaveBeenCalled();
  });

  it('clears staged object URLs', async () => {
    await state.stageFiles([imageFile('clear.png')]);

    state.clear();

    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:clear.png');
    expect(state.filesWithUrls).toEqual([]);
  });
});
