/** An explicitly started, page-owned microphone test. Audio is monitored locally without recording. */
export class CallDeviceTest {
  active = $state(false);
  pending = $state(false);
  level = $state(0);
  error = $state(false);
  #generation = 0;
  #stream: MediaStream | null = null;
  #context: AudioContext | null = null;
  #audio: HTMLAudioElement | null = null;
  #frame = 0;

  async start(deviceId: string, speakerId = ''): Promise<void> {
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
      const audio = new Audio();
      this.#audio = audio;
      audio.srcObject = stream;
      if ('setSinkId' in audio && speakerId) {
        try {
          await audio.setSinkId(speakerId);
        } catch {
          await audio.setSinkId('');
        }
      }
      if (generation !== this.#generation) return;
      await audio.play();
      if (generation !== this.#generation) {
        audio.pause();
        return;
      }
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

  /** Invalidate pending capture and stop monitoring on navigation or cancel. */
  stop(): void {
    this.#generation++;
    cancelAnimationFrame(this.#frame);
    this.#audio?.pause();
    if (this.#audio) this.#audio.srcObject = null;
    this.#audio = null;
    this.#stream?.getTracks().forEach((track) => track.stop());
    this.#stream = null;
    void this.#context?.close().catch(() => {});
    this.#context = null;
    this.active = false;
    this.pending = false;
    this.level = 0;
  }
}
