import { beforeAll, describe, expect, it } from 'vitest';
import {
  Input,
  BlobSource,
  ALL_FORMATS,
  Output,
  BufferTarget,
  WebMOutputFormat,
  CanvasSource,
  AudioBufferSource
} from 'mediabunny';
import { compressVideo } from './compressVideo';

/** Build a local audio/video fixture; WebM padding exercises the upload-size budget. */
async function makeVideo(width = 320, height = 180): Promise<File> {
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext('2d')!;
  const target = new BufferTarget();
  const output = new Output({ target, format: new WebMOutputFormat() });
  const video = new CanvasSource(canvas, { codec: 'vp8', bitrate: 1_000_000 });
  const audio = new AudioBufferSource({ codec: 'opus', bitrate: 96_000 });
  output.addVideoTrack(video);
  output.addAudioTrack(audio);
  try {
    await output.start();
    for (let frame = 0; frame < 30; frame++) {
      context.fillStyle = `hsl(${frame * 9},80%,45%)`;
      context.fillRect(0, 0, width, height);
      await video.add(frame / 30, 1 / 30);
    }
    const buffer = new AudioBuffer({ length: 48_000, sampleRate: 48_000, numberOfChannels: 1 });
    const data = buffer.getChannelData(0);
    for (let i = 0; i < data.length; i++)
      data[i] = Math.sin((i * 2 * Math.PI * 440) / 48_000) * 0.1;
    await audio.add(buffer);
    await output.finalize();
    // An EBML Void element holds 1 MiB of padding without changing the media.
    const padding = new Uint8Array(4 + 1024 * 1024);
    padding.set([0xec, 0x30, 0x00, 0x00]);
    return new File([target.buffer!, padding], 'source.webm', { type: 'video/webm' });
  } finally {
    if (output.state !== 'finalized') await output.cancel();
  }
}

describe('local browser video compression', () => {
  let source: File;
  beforeAll(async () => {
    source = await makeVideo();
  });

  it('encodes within the byte budget and retains decodable video and audio', async () => {
    const progress: number[] = [];
    const file = await compressVideo(source, {
      maxBytes: 128 * 1024,
      signal: new AbortController().signal,
      onProgress: (fraction) => progress.push(fraction)
    });
    expect(file.size).toBeLessThanOrEqual(128 * 1024);
    expect(file.size).toBeLessThan(source.size);
    expect(['video/mp4', 'video/webm']).toContain(file.type);
    const input = new Input({ source: new BlobSource(file), formats: ALL_FORMATS });
    try {
      expect(await (await input.getPrimaryVideoTrack())?.canDecode()).toBe(true);
      expect(await (await input.getPrimaryAudioTrack())?.canDecode()).toBe(true);
      expect(await input.computeDuration()).toBeGreaterThan(0.8);
      expect(progress.length).toBeGreaterThan(0);
    } finally {
      input.dispose();
    }
  });

  it.each([
    [640, 360, 426, 238],
    [360, 640, 238, 426]
  ])(
    'reduces %ix%i input at a low bitrate budget to %ix%i',
    async (width, height, expectedWidth, expectedHeight) => {
      const file = await compressVideo(await makeVideo(width, height), {
        maxBytes: 40 * 1024,
        signal: new AbortController().signal,
        onProgress: () => {}
      });
      expect(file.size).toBeLessThanOrEqual(40 * 1024);
      const input = new Input({ source: new BlobSource(file), formats: ALL_FORMATS });
      try {
        const video = await input.getPrimaryVideoTrack();
        expect(video?.displayWidth).toBe(expectedWidth);
        expect(video?.displayHeight).toBe(expectedHeight);
        expect(await (await input.getPrimaryAudioTrack())?.canDecode()).toBe(true);
      } finally {
        input.dispose();
      }
    }
  );

  it('does not return a result after cancellation during conversion', async () => {
    const controller = new AbortController();
    await expect(
      compressVideo(source, {
        maxBytes: 128 * 1024,
        signal: controller.signal,
        onProgress: () => controller.abort()
      })
    ).rejects.toThrow();
    expect(controller.signal.aborted).toBe(true);
  });

  it('rejects a byte budget too small for the duration', async () => {
    await expect(
      compressVideo(source, {
        maxBytes: 100,
        signal: new AbortController().signal,
        onProgress: () => {}
      })
    ).rejects.toThrow('size budget');
  });
});
