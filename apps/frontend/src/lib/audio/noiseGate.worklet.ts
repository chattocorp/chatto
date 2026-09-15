import { NoiseGate, normalizeGateThreshold } from './noiseGate';
import { LowFrequencyControl } from './lowFrequencyControl';
import { VoicePolish, normalizePolish } from './voicePolish';

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
  #bass: LowFrequencyControl;
  #frames = 0;
  #stopped = false;
  constructor(options: AudioWorkletNodeOptions) {
    super();
    this.#bass = new LowFrequencyControl(sampleRate, options.processorOptions?.polish);
    this.#gate.threshold = normalizeGateThreshold(options.processorOptions?.threshold);
    this.#gate.softness = normalizePolish(options.processorOptions?.polish);
    this.port.onmessage = ({ data }) => {
      if (data.stop) this.#stopped = true;
      else {
        if ('threshold' in data) this.#gate.threshold = normalizeGateThreshold(data.threshold);
        if ('polish' in data) {
          this.#gate.softness = normalizePolish(data.polish);
          this.#bass.amount = data.polish;
        }
      }
    };
  }
  process(inputs: Float32Array[][], outputs: Float32Array[][]): boolean {
    if (this.#stopped) return false;
    const output = outputs[0] ?? [];
    this.#gate.process(inputs[0] ?? [], output);
    this.#bass.process(output, output);
    this.#frames += outputs[0]?.[0]?.length ?? 0;
    if (this.#frames >= sampleRate * 0.06) {
      this.port.postMessage(this.#gate.level);
      this.#frames = 0;
    }
    return true;
  }
}
registerProcessor('chatto-microphone-gate', MicrophoneGateWorklet);

/** Final audio-thread stage, after the native EQ/compressor graph. */
class VoicePolishWorklet extends AudioWorkletProcessor {
  #polish: VoicePolish;
  #stopped = false;
  constructor(options: AudioWorkletNodeOptions) {
    super();
    this.#polish = new VoicePolish(sampleRate, options.processorOptions?.polish);
    this.port.onmessage = ({ data }) => {
      if (data.stop) this.#stopped = true;
      else this.#polish.amount = data.polish;
    };
  }
  process(inputs: Float32Array[][], outputs: Float32Array[][]): boolean {
    if (this.#stopped) return false;
    this.#polish.process(inputs[0] ?? [], outputs[0] ?? []);
    return true;
  }
}
registerProcessor('chatto-voice-polish', VoicePolishWorklet);
