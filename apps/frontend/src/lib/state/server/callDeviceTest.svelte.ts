import { normalizeMicrophoneEffects, type MicrophoneEffects } from '$lib/audio/microphoneEffects';
import { MicrophoneProcessor } from '$lib/audio/microphoneProcessor';
import { microphoneMeter } from '$lib/audio/noiseGate';
import type { Track } from 'livekit-client';
/** Optional Web Audio output selection, absent from older browsers and DOM types. */
export type OutputAudioContext = AudioContext & {
  setSinkId?: (deviceId: string) => Promise<void>;
};

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
  #context: OutputAudioContext | null = null;
  #audio: HTMLAudioElement | null = null;
  #monitor: MediaStreamAudioSourceNode | null = null;
  #frame = 0;
  #outputChange: Promise<unknown> = Promise.resolve();

  async start(
    deviceId: string,
    speakerId = '',
    threshold: () => number = () => -60,
    effects: () => MicrophoneEffects = normalizeMicrophoneEffects
  ): Promise<void> {
    this.stop();
    const generation = this.#generation;
    this.pending = true;
    this.error = false;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: {
          ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
          channelCount: { ideal: 1 },
          echoCancellation: true,
          noiseSuppression: true,
          autoGainControl: false
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
      const context: OutputAudioContext = new AudioContext();
      this.#context = context;
      // Use one audio graph and clock for capture processing and monitoring.
      // Older browsers retain the media-element output path below.
      const selectOutput = context.setSinkId?.bind(context);
      if (selectOutput) await selectOutput(speakerId);
      if (generation !== this.#generation) return;
      await context.resume();
      if (generation !== this.#generation) return;
      const processor = new MicrophoneProcessor(threshold());
      this.#processor = processor;
      processor.setEffects(effects());
      await processor.init({
        track: stream.getAudioTracks()[0],
        audioContext: context,
        kind: 'audio' as Track.Kind.Audio
      });
      if (generation !== this.#generation) return;
      this.gateUnavailable = processor.unavailable;
      if (selectOutput) {
        if (!processor.connectMonitor(context.destination)) {
          this.#monitor = context.createMediaStreamSource(stream);
          this.#monitor.connect(context.destination);
        }
      } else {
        const audio = new Audio();
        this.#audio = audio;
        audio.srcObject = new MediaStream([processor.processedTrack ?? stream.getAudioTracks()[0]]);
        if (speakerId) {
          if (!('setSinkId' in audio)) throw new Error('Output selection unavailable');
          await audio.setSinkId(speakerId);
        }
        if (generation !== this.#generation) return;
        await audio.play();
        if (generation !== this.#generation) {
          audio.pause();
          return;
        }
      }
      const sample = () => {
        if (generation !== this.#generation) return;
        processor.setThreshold(threshold());
        processor.setEffects(effects());
        this.gateUnavailable = processor.unavailable;
        this.level = microphoneMeter(processor.level);
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

  /** Switch only playback, serializing requests without reopening the microphone. */
  setSpeaker(speakerId: string): Promise<boolean> {
    const generation = this.#generation;
    const change = this.#outputChange.then(async () => {
      if (generation !== this.#generation || !this.active) return false;
      try {
        if (this.#context && typeof this.#context.setSinkId === 'function') {
          await this.#context.setSinkId(speakerId);
        } else if (this.#audio && typeof this.#audio.setSinkId === 'function') {
          await this.#audio.setSinkId(speakerId);
        } else {
          throw new Error('Output selection unavailable');
        }
        return generation === this.#generation;
      } catch {
        if (generation === this.#generation) {
          this.stop();
          this.error = true;
        }
        return false;
      }
    });
    this.#outputChange = change;
    return change;
  }

  /** Invalidate pending capture and stop monitoring on navigation or cancel. */
  stop(): void {
    this.#generation++;
    this.#outputChange = Promise.resolve();
    this.#processor?.dispose();
    this.#processor = null;
    this.gateUnavailable = false;
    cancelAnimationFrame(this.#frame);
    this.#monitor?.disconnect();
    this.#monitor = null;
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
