import { MicrophoneEffectsGraph } from './microphoneEffectsGraph';
import type { AudioProcessorOptions, Track, TrackProcessor } from 'livekit-client';
import workletURL from './noiseGate.worklet?worker&url';
import { GATE_OFF } from './noiseGate';
import { normalizeMicrophoneEffects, type MicrophoneEffects } from './microphoneEffects';

const modules = new WeakMap<AudioContext, Promise<void>>();

/** Owns the microphone processing graph, but never owns the source track or supplied context. */
export class MicrophoneProcessor implements TrackProcessor<
  Track.Kind.Audio,
  AudioProcessorOptions
> {
  readonly name = 'chatto-microphone-gate';
  processedTrack?: MediaStreamTrack;
  #level = 0;
  #analyser?: AnalyserNode;
  #samples?: Float32Array<ArrayBuffer>;
  active = false;
  unavailable = false;
  #generation = 0;
  #disposed = false;
  #source?: MediaStreamAudioSourceNode;
  #node?: AudioWorkletNode;
  #polish?: AudioWorkletNode;
  #onError?: () => void;
  #destination?: MediaStreamAudioDestinationNode;
  #threshold = GATE_OFF;
  #effects: MicrophoneEffects = normalizeMicrophoneEffects();
  #graph?: MicrophoneEffectsGraph;

  constructor(threshold = GATE_OFF) {
    this.#threshold = threshold;
  }

  /** Input RMS comes from the worklet, or its analyser fallback after failure. */
  get level(): number {
    if (!this.#analyser || !this.#samples) return this.#level;
    this.#analyser.getFloatTimeDomainData(this.#samples);
    return Math.sqrt(
      this.#samples.reduce((sum, value) => sum + value * value, 0) / this.#samples.length
    );
  }

  private setupFallbackMeter(context: AudioContext): void {
    this.#analyser = context.createAnalyser();
    this.#analyser.fftSize = 1024;
    this.#samples = new Float32Array(this.#analyser.fftSize);
    this.#source?.connect(this.#analyser);
  }

  setThreshold(value: number): void {
    if (value === this.#threshold) return;
    this.#threshold = value;
    this.#node?.port.postMessage({ threshold: value });
  }

  /** Retain settings across SDK restarts and apply them to the current graph. */
  setEffects(value: MicrophoneEffects): void {
    const previous = this.#effects.polish;
    this.#effects = normalizeMicrophoneEffects(value);
    if (previous !== this.#effects.polish) {
      this.#node?.port.postMessage({ polish: this.#effects.polish });
      this.#polish?.port.postMessage({ polish: this.#effects.polish });
    }
    this.#graph?.update(this.#effects);
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
        processorOptions: { threshold: this.#threshold, polish: this.#effects.polish }
      });
      this.#node = node;
      const destination = audioContext.createMediaStreamDestination();
      this.#destination = destination;
      const polish = new AudioWorkletNode(audioContext, 'chatto-voice-polish', {
        processorOptions: { polish: this.#effects.polish }
      });
      this.#polish = polish;
      polish.connect(destination);
      node.port.onmessage = ({ data }) => {
        this.#level = data;
      };
      node.port.postMessage({ threshold: this.#threshold });
      this.#onError = () => {
        if (generation !== this.#generation) return;
        source.disconnect();
        this.#graph?.destroy();
        this.#graph = undefined;
        node.disconnect();
        polish.disconnect();
        node.removeEventListener('processorerror', this.#onError!);
        polish.removeEventListener('processorerror', this.#onError!);
        node.port.onmessage = null;
        node.port.postMessage({ stop: true });
        polish.port.postMessage({ stop: true });
        node.port.close();
        polish.port.close();
        source.connect(destination);
        this.setupFallbackMeter(audioContext);
        this.active = false;
        this.unavailable = true;
      };
      node.addEventListener('processorerror', this.#onError, { once: true });
      polish.addEventListener('processorerror', this.#onError, { once: true });
      this.#graph = new MicrophoneEffectsGraph(audioContext, node, polish);
      this.#graph.update(this.#effects, true);
      source.connect(this.#graph.input);
      this.processedTrack = destination.stream.getAudioTracks()[0];
      this.active = true;
    } catch {
      if (generation !== this.#generation) return;
      void this.destroy();
      this.processedTrack = track;
      this.unavailable = true;
      try {
        this.#source = audioContext.createMediaStreamSource(new MediaStream([track]));
        this.setupFallbackMeter(audioContext);
      } catch {
        // Meter support is optional too; the original track remains usable.
      }
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
    if (this.#polish) {
      this.#polish.port.postMessage({ stop: true });
      if (this.#onError) this.#polish.removeEventListener('processorerror', this.#onError);
      this.#polish.port.close();
      this.#polish.disconnect();
      this.#polish = undefined;
    }
    this.#graph?.destroy();
    this.#graph = undefined;
    this.#source?.disconnect();
    this.#node?.disconnect();
    this.#destination?.stream.getTracks().forEach((track) => track.stop());
    this.#source = undefined;
    this.#node = undefined;
    this.#onError = undefined;
    this.#destination = undefined;
    this.processedTrack = undefined;
    this.active = false;
    this.#level = 0;
    this.#analyser?.disconnect();
    this.#analyser = undefined;
    this.#samples = undefined;
  }
}
