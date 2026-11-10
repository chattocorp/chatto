/** Options for the local video compression experiment. No bytes are uploaded here. */
export type VideoCompressionOptions = {
  maxBytes: number;
  signal: AbortSignal;
  onProgress: (fraction: number) => void;
};

/** Conservative PoC bounds: avoid retaining very large outputs or decoding huge frames. */
const MAX_OUTPUT_BYTES = 100 * 1024 * 1024;

/**
 * Make a smaller MP4 or WebM with browser codecs. Unsupported tracks fail the
 * operation rather than silently removing audio. The caller retains the original.
 */
export async function compressVideo(file: File, options: VideoCompressionOptions): Promise<File> {
  const { signal, onProgress } = options;
  signal.throwIfAborted();
  const {
    Input,
    BlobSource,
    ALL_FORMATS,
    Output,
    BufferTarget,
    Conversion,
    Mp4OutputFormat,
    WebMOutputFormat,
    canEncodeVideo,
    canEncodeAudio
  } = await import('mediabunny');
  signal.throwIfAborted();
  const input = new Input({ source: new BlobSource(file), formats: ALL_FORMATS });
  let conversion: Awaited<ReturnType<typeof Conversion.init>> | undefined;
  let cancellation: Promise<void> | undefined;
  const cancel = () => {
    if (conversion && conversion.state !== 'done' && conversion.state !== 'canceled') {
      cancellation ??= conversion.cancel();
      // The operation reports failure through execute(); cleanup still waits below.
      void cancellation.catch(() => {});
    }
  };
  signal.addEventListener('abort', cancel, { once: true });
  try {
    const video = await input.getPrimaryVideoTrack();
    const audio = await input.getPrimaryAudioTrack();
    const duration = await input.computeDuration();
    signal.throwIfAborted();
    if (!video || !(await video.canDecode()) || (audio && !(await audio.canDecode()))) {
      throw new Error('Unsupported source codec');
    }
    if (
      video.displayWidth * video.displayHeight > 3840 * 2160 ||
      (await video.hasHighDynamicRange())
    ) {
      throw new Error('HDR and sources above 4K are outside this experiment');
    }
    const maxBytes = Math.min(options.maxBytes, MAX_OUTPUT_BYTES, file.size - 1);
    const audioBitrate = audio ? 96_000 : 0;
    // Leave 15% for rate-control variance and the container; verify actual size below.
    const bitrate = Math.min(
      3_000_000,
      Math.floor((maxBytes * 8 * 0.85) / duration) - audioBitrate
    );
    if (!Number.isFinite(duration) || duration <= 0 || bitrate < 150_000) {
      throw new Error('The size budget is too small for this duration');
    }
    const scale = Math.min(1, 1920 / Math.max(video.displayWidth, video.displayHeight));
    const width = Math.max(2, Math.floor((video.displayWidth * scale) / 2) * 2);
    const height = Math.max(2, Math.floor((video.displayHeight * scale) / 2) * 2);
    // Pair codecs with their containers and require audio support before choosing.
    const candidates = [
      { video: 'avc', audio: 'aac', webm: false },
      { video: 'vp9', audio: 'opus', webm: true },
      { video: 'vp8', audio: 'opus', webm: true }
    ] as const;
    let selected: (typeof candidates)[number] | undefined;
    for (const candidate of candidates) {
      signal.throwIfAborted();
      if (
        (await canEncodeVideo(candidate.video, { width, height, bitrate })) &&
        (!audio ||
          (await canEncodeAudio(candidate.audio, {
            numberOfChannels: 2,
            sampleRate: 48_000,
            bitrate: audioBitrate
          })))
      ) {
        selected = candidate;
        break;
      }
    }
    if (!selected) throw new Error('No supported output codecs');
    const target = new BufferTarget();
    const output = new Output({
      target,
      format: selected.webm
        ? new WebMOutputFormat()
        : new Mp4OutputFormat({ fastStart: 'fragmented' })
    });
    conversion = await Conversion.init({
      input,
      output,
      tracks: 'primary',
      video: {
        codec: selected.video,
        bitrate,
        width,
        height,
        fit: 'contain',
        forceTranscode: true
      },
      audio: audio
        ? {
            codec: selected.audio,
            bitrate: audioBitrate,
            numberOfChannels: 2,
            sampleRate: 48_000,
            forceTranscode: true
          }
        : undefined
    });
    signal.throwIfAborted();
    if (!conversion.isValid || conversion.discardedTracks.length > 0) {
      throw new Error('Cannot preserve the selected tracks');
    }
    let exceededLimit = false;
    target.on('write', ({ end }) => {
      if (end > maxBytes && !exceededLimit) {
        exceededLimit = true;
        cancel();
      }
    });
    conversion.onProgress = (fraction) => {
      if (!signal.aborted) onProgress(Math.max(0, Math.min(1, fraction)));
    };
    await conversion.execute();
    signal.throwIfAborted();
    if (exceededLimit || !target.buffer || target.buffer.byteLength > maxBytes) {
      throw new Error('Compressed video exceeds the size budget');
    }
    const extension = selected.webm ? 'webm' : 'mp4';
    return new File(
      [target.buffer],
      `${file.name.replace(/\.[^.]+$/, '')}-compressed.${extension}`,
      {
        type: selected.webm ? 'video/webm' : 'video/mp4'
      }
    );
  } finally {
    signal.removeEventListener('abort', cancel);
    cancel();
    await cancellation?.catch(() => {});
    input.dispose();
  }
}
