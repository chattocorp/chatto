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
      maxUploadSize: 25 * 1024 * 1024
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
    expect(state.filesWithUrls.map(({ url }) => url)).toEqual([
      'blob:first.png',
      'blob:second.png'
    ]);
  });

  it('keeps trimmed descriptions with their selected files', async () => {
    const first = imageFile('first.png');
    const second = imageFile('second.png');

    await state.stageFiles([first, second]);
    state.setDescription(1, '  A chart\nwith two lines.  ');

    expect(state.descriptions).toEqual([{ file: second, description: 'A chart\nwith two lines.' }]);
    expect(state.filesWithUrls[0].description).toBe('');
    expect(state.filesWithUrls[1].description).toBe('A chart\nwith two lines.');
  });

  it('caps a batch at ten files and explains why extra files were not added', async () => {
    const files = Array.from({ length: 12 }, (_, i) => imageFile(`${i}.png`));

    await state.stageFiles(files);

    expect(state.selectedFiles).toEqual(files.slice(0, 10));
    expect(prepareFilesMock).toHaveBeenCalledWith(files.slice(0, 10));
    expect(getToasts().map((t) => t.message)).toEqual([
      'You can attach up to 10 files per message. Extra files were not added.'
    ]);
  });

  it('counts earlier selections and frees a slot when a file is removed', async () => {
    await state.stageFiles(Array.from({ length: 10 }, (_, i) => imageFile(`${i}.png`)));
    expect(getToasts()).toHaveLength(0);
    prepareFilesMock.mockClear();

    await state.stageFiles([imageFile('extra.png')]);
    expect(prepareFilesMock).not.toHaveBeenCalled();
    state.removeFile(0);
    await state.stageFiles([imageFile('replacement.png'), imageFile('extra.png')]);

    expect(state.selectedFiles).toHaveLength(10);
    expect(state.selectedFiles.at(-1)?.name).toBe('replacement.png');
  });

  it('reserves slots while another selection is still being prepared', async () => {
    const first = Array.from({ length: 9 }, (_, i) => imageFile(`${i}.png`));
    const preparation = Promise.withResolvers<File[]>();
    prepareFilesMock.mockImplementation((files: File[]) =>
      files[0] === first[0] ? preparation.promise : Promise.resolve(files)
    );

    const pending = state.stageFiles(first);
    await state.stageFiles([imageFile('last.png'), imageFile('extra.png')]);
    expect(state.pendingCount).toBe(9);
    preparation.resolve(first);
    await pending;

    expect(state.selectedFiles).toHaveLength(10);
    expect(state.selectedFiles.some((file) => file.name === 'extra.png')).toBe(false);
    expect(state.pendingCount).toBe(0);
  });

  it('releases slots after preparation fails', async () => {
    prepareFilesMock.mockRejectedValueOnce(new Error('conversion failed'));
    await state.stageFiles(Array.from({ length: 10 }, () => imageFile()));
    expect(state.pendingCount).toBe(0);

    await state.stageFiles([imageFile('retry.png')]);
    expect(state.selectedFiles.map((file) => file.name)).toEqual(['retry.png']);
  });

  it.each(['clear', 'restore'] as const)('discards pending files after %s', async (operation) => {
    const preparation = Promise.withResolvers<File[]>();
    prepareFilesMock.mockReturnValueOnce(preparation.promise);
    const pending = state.stageFiles([imageFile('old.png')]);

    if (operation === 'clear') state.clear();
    else state.restore([]);
    await state.stageFiles(Array.from({ length: 10 }, () => imageFile('new.png')));
    preparation.resolve([imageFile('old.png')]);
    await pending;

    expect(state.selectedFiles).toHaveLength(10);
    expect(state.selectedFiles.every((file) => file.name === 'new.png')).toBe(true);
    expect(state.pendingCount).toBe(0);
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

    expect(
      getToasts()
        .map((t) => t.message)
        .join('\n')
    ).toContain('too-large.png is too large');
    expect(state.filesWithUrls).toEqual([]);
    expect(prepareFilesMock).not.toHaveBeenCalled();
  });

  it('uses the general upload limit for videos', async () => {
    limits.videoProcessingEnabled = true;
    limits.maxUploadSize = 1;

    await state.stageFiles([videoFile('too-large.mp4', 2)]);

    expect(
      getToasts()
        .map((t) => t.message)
        .join('\n')
    ).toContain('too-large.mp4 is too large');
    expect(state.filesWithUrls).toEqual([]);
    expect(prepareFilesMock).not.toHaveBeenCalled();
  });

  it('accepts GIFs and videos at the same limit and rejects both above it', () => {
    limits.videoProcessingEnabled = true;
    limits.maxUploadSize = 10;
    const gif = new File([new Uint8Array(10)], 'animation.gif', { type: 'image/gif' });
    const video = videoFile('clip.mp4', 10);
    const largeGif = new File([new Uint8Array(11)], 'large.gif', { type: 'image/gif' });
    const largeVideo = videoFile('large.mp4', 11);

    expect(state.validateFiles([gif, video, largeGif, largeVideo])).toEqual([gif, video]);
    expect(getToasts()).toHaveLength(2);
  });

  it('clears staged object URLs', async () => {
    await state.stageFiles([imageFile('clear.png')]);

    state.clear();

    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:clear.png');
    expect(state.filesWithUrls).toEqual([]);
  });
});
