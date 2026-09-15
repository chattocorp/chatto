import type { AudioProcessorOptions, Track, TrackProcessor } from 'livekit-client';
import workletURL from './noiseGate.worklet?worker&url';
import { GATE_OFF } from './noiseGate';

const modules = new WeakMap<AudioContext, Promise<void>>();

/** Owns the gate graph, but never owns the source track or supplied context. */
export class MicrophoneProcessor implements TrackProcessor<
  Track.Kind.Audio,
  AudioProcessorOptions
> {
  readonly name = 'chatto-microphone-gate';
  processedTrack?: MediaStreamTrack;
  level = 0;
  active = false;
  unavailable = false;
  #generation = 0;
  #disposed = false;
  #source?: MediaStreamAudioSourceNode;
  #node?: AudioWorkletNode;
  #onError?: () => void;
  #destination?: MediaStreamAudioDestinationNode;
  #threshold = GATE_OFF;

  constructor(threshold = GATE_OFF) {
    this.#threshold = threshold;
  }

  setThreshold(value: number): void {
    if (value === this.#threshold) return;
    this.#threshold = value;
    this.#node?.port.postMessage({ threshold: value });
  }

  async init({ track, audioContext }: AudioProcessorOptions): Promise<void> {
    if (this.#disposed) return;
    void this.destroy();
    const generation = this.#generation;
    this.unavailable = false;
    // A failed optional processor must leave the microphone usable.
    this.processedTrack = track;
    try {
      if (!audioContext.audioWorklet) throw new Error('Audio worklets unavailable');
      let loaded = modules.get(audioContext);
      if (!loaded) {
        loaded = audioContext.audioWorklet.addModule(workletURL);
        modules.set(audioContext, loaded);
        void loaded.catch(() => modules.delete(audioContext));
      }
      await loaded;
      if (generation !== this.#generation) return;
      const source = audioContext.createMediaStreamSource(new MediaStream([track]));
      this.#source = source;
      const node = new AudioWorkletNode(audioContext, 'chatto-microphone-gate', {
        processorOptions: { threshold: this.#threshold }
      });
      this.#node = node;
      const destination = audioContext.createMediaStreamDestination();
      this.#destination = destination;
      node.port.onmessage = ({ data }) => {
        this.level = data;
      };
      node.port.postMessage({ threshold: this.#threshold });
      this.#onError = () => {
        if (generation !== this.#generation) return;
        source.disconnect();
        node.disconnect();
        source.connect(destination);
        this.active = false;
        this.unavailable = true;
      };
      node.addEventListener('processorerror', this.#onError, { once: true });
      source.connect(node).connect(destination);
      this.processedTrack = destination.stream.getAudioTracks()[0];
      this.active = true;
    } catch {
      if (generation !== this.#generation) return;
      void this.destroy();
      this.processedTrack = track;
      this.unavailable = true;
    }
  }

  async restart(options: AudioProcessorOptions): Promise<void> {
    await this.init(options);
  }

  /** Permanently close a call/test owner, including queued SDK initialization. */
  dispose(): void {
    this.#disposed = true;
    void this.destroy();
  }

  async destroy(): Promise<void> {
    this.#generation++;
    this.#node?.port.postMessage({ stop: true });
    if (this.#node) {
      this.#node.port.onmessage = null;
      if (this.#onError) this.#node.removeEventListener('processorerror', this.#onError);
      this.#node.port.close();
    }
    this.#source?.disconnect();
    this.#node?.disconnect();
    this.#destination?.stream.getTracks().forEach((track) => track.stop());
    this.#source = undefined;
    this.#node = undefined;
    this.#onError = undefined;
    this.#destination = undefined;
    this.processedTrack = undefined;
    this.active = false;
    this.level = 0;
  }
}
