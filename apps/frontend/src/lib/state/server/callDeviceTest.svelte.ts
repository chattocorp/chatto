/** An explicitly started, page-owned microphone test. Audio stays in memory. */
export class CallDeviceTest {
  active = $state(false);
  pending = $state(false);
  recording = $state(false);
  level = $state(0);
  clipURL = $state('');
  error = $state(false);
  #generation = 0;
  #stream: MediaStream | null = null;
  #context: AudioContext | null = null;
  #recorder: MediaRecorder | null = null;
  #frame = 0;
  #limit: ReturnType<typeof setTimeout> | undefined;

  async start(deviceId: string): Promise<void> {
    this.stop();
    const generation = this.#generation;
    this.pending = true;
    this.error = false;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          ...(deviceId ? { deviceId: { ideal: deviceId } } : {}),
          channelCount: { ideal: 1 },
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: true
        },
        video: false
      });
      if (generation !== this.#generation) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      this.#stream = stream;
      stream.getAudioTracks().forEach((track) =>
        track.addEventListener(
          'ended',
          () => {
            if (generation === this.#generation) {
              this.stop();
              this.error = true;
            }
          },
          { once: true }
        )
      );
      const context = new AudioContext();
      this.#context = context;
      await context.resume();
      if (generation !== this.#generation) return;
      const analyser = context.createAnalyser();
      analyser.fftSize = 1024;
      context.createMediaStreamSource(stream).connect(analyser);
      const samples = new Float32Array(analyser.fftSize);
      const sample = () => {
        if (generation !== this.#generation) return;
        analyser.getFloatTimeDomainData(samples);
        this.level = Math.min(
          1,
          Math.sqrt(samples.reduce((sum, value) => sum + value * value, 0) / samples.length) * 4
        );
        this.#frame = requestAnimationFrame(sample);
      };
      this.active = true;
      sample();
    } catch {
      if (generation === this.#generation) {
        this.stop();
        this.error = true;
      }
    } finally {
      if (generation === this.#generation) this.pending = false;
    }
  }

  /** Record at most ten seconds; completion releases microphone capture. */
  record(): void {
    if (!this.#stream || this.recording || typeof MediaRecorder === 'undefined') return;
    this.clearClip();
    const generation = this.#generation;
    try {
      const recorder = new MediaRecorder(this.#stream);
      this.#recorder = recorder;
      const chunks: Blob[] = [];
      recorder.ondataavailable = (event) => {
        if (generation === this.#generation && event.data.size) chunks.push(event.data);
      };
      recorder.onstop = () => {
        if (generation !== this.#generation) return;
        const clip = new Blob(chunks, { type: recorder.mimeType });
        this.stop();
        if (clip.size) this.clipURL = URL.createObjectURL(clip);
      };
      recorder.onerror = () => {
        if (generation === this.#generation) {
          this.stop();
          this.error = true;
        }
      };
      recorder.start();
      this.recording = true;
      this.#limit = setTimeout(() => this.finishRecording(), 10_000);
    } catch {
      this.stop();
      this.error = true;
    }
  }

  finishRecording(): void {
    if (this.#recorder?.state === 'recording') this.#recorder.stop();
    clearTimeout(this.#limit);
  }

  clearClip(): void {
    if (this.clipURL) URL.revokeObjectURL(this.clipURL);
    this.clipURL = '';
  }

  /** Invalidate pending capture and discard recordings on navigation or cancel. */
  stop(): void {
    this.#generation++;
    clearTimeout(this.#limit);
    cancelAnimationFrame(this.#frame);
    if (this.#recorder?.state === 'recording') this.#recorder.stop();
    this.#recorder = null;
    this.#stream?.getTracks().forEach((track) => track.stop());
    this.#stream = null;
    void this.#context?.close().catch(() => {});
    this.#context = null;
    this.active = false;
    this.pending = false;
    this.recording = false;
    this.level = 0;
    this.clearClip();
  }
}
