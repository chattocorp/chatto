import { NoiseGate, normalizeGateThreshold } from './noiseGate';

// AudioWorklet globals are separate from both DOM and WorkerGlobalScope.
declare const sampleRate: number;
declare class AudioWorkletProcessor {
  readonly port: MessagePort;
}
declare function registerProcessor(
  name: string,
  processor: new (options: AudioWorkletNodeOptions) => AudioWorkletProcessor
): void;

/** Audio-thread owner: sample-based timing continues in background tabs. */
class MicrophoneGateWorklet extends AudioWorkletProcessor {
  #gate = new NoiseGate(sampleRate);
  #frames = 0;
  #stopped = false;
  constructor(options: AudioWorkletNodeOptions) {
    super();
    this.#gate.threshold = normalizeGateThreshold(options.processorOptions?.threshold);
    this.port.onmessage = ({ data }) => {
      if (data.stop) this.#stopped = true;
      else this.#gate.threshold = normalizeGateThreshold(data.threshold);
    };
  }
  process(inputs: Float32Array[][], outputs: Float32Array[][]): boolean {
    if (this.#stopped) return false;
    this.#gate.process(inputs[0] ?? [], outputs[0] ?? []);
    this.#frames += outputs[0]?.[0]?.length ?? 0;
    if (this.#frames >= sampleRate * 0.06) {
      this.port.postMessage(this.#gate.level);
      this.#frames = 0;
    }
    return true;
  }
}
registerProcessor('chatto-microphone-gate', MicrophoneGateWorklet);
