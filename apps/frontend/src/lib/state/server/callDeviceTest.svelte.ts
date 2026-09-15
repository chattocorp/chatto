import { MicrophoneProcessor } from '$lib/audio/microphoneProcessor';
import { microphoneMeter } from '$lib/audio/noiseGate';
import type { Track } from 'livekit-client';
/** An explicitly started, page-owned microphone test. Audio is monitored locally without recording. */
export class CallDeviceTest {
  active = $state(false);
  pending = $state(false);
  level = $state(0);
  error = $state(false);
  gateUnavailable = $state(false);
  #processor: MicrophoneProcessor | null = null;
  #generation = 0;
  #stream: MediaStream | null = null;
  #context: AudioContext | null = null;
  #audio: HTMLAudioElement | null = null;
  #frame = 0;

  async start(
    deviceId: string,
    speakerId = '',
    threshold: () => number = () => -60
  ): Promise<void> {
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
      const processor = new MicrophoneProcessor(threshold());
      this.#processor = processor;
      await processor.init({
        track: stream.getAudioTracks()[0],
        audioContext: context,
        kind: 'audio' as Track.Kind.Audio
      });
      if (generation !== this.#generation) return;
      this.gateUnavailable = processor.unavailable;
      const audio = new Audio();
      this.#audio = audio;
      audio.srcObject = new MediaStream([processor.processedTrack ?? stream.getAudioTracks()[0]]);
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
      const analyser = context.createAnalyser();
      analyser.fftSize = 1024;
      if (!processor.active) context.createMediaStreamSource(stream).connect(analyser);
      const samples = new Float32Array(analyser.fftSize);
      const sample = () => {
        if (generation !== this.#generation) return;
        processor.setThreshold(threshold());
        this.gateUnavailable = processor.unavailable;
        if (processor.active) this.level = microphoneMeter(processor.level);
        else {
          analyser.getFloatTimeDomainData(samples);
          this.level = microphoneMeter(
            Math.sqrt(samples.reduce((sum, value) => sum + value * value, 0) / samples.length)
          );
        }
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
    this.#processor?.dispose();
    this.#processor = null;
    this.gateUnavailable = false;
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
