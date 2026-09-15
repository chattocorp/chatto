import {
  defaultMicrophoneEffects,
  normalizeMicrophoneEffects,
  type MicrophoneEffects
} from './microphoneEffects';

/** Owns native DSP nodes around the gate; never owns the context or gate. */
export class MicrophoneEffectsGraph {
  readonly input: BiquadFilterNode;
  readonly #bass: BiquadFilterNode;
  readonly #mid: BiquadFilterNode;
  readonly #treble: BiquadFilterNode;
  readonly #headroom: GainNode;
  readonly #compressor: DynamicsCompressorNode;
  readonly #dry: GainNode;
  readonly #wet: GainNode;
  readonly #nodes: AudioNode[];
  #settings = '';

  constructor(
    private context: BaseAudioContext,
    gate: AudioNode,
    output: AudioNode
  ) {
    this.input = context.createBiquadFilter();
    this.input.type = 'highpass';
    // High-pass Q is expressed in dB; this gives a flat Butterworth response.
    this.input.Q.value = 20 * Math.log10(Math.SQRT1_2);
    this.#bass = context.createBiquadFilter();
    this.#bass.type = 'lowshelf';
    this.#bass.frequency.value = 200;
    this.#mid = context.createBiquadFilter();
    this.#mid.type = 'peaking';
    this.#mid.frequency.value = 1200;
    this.#mid.Q.value = 0.7;
    this.#treble = context.createBiquadFilter();
    this.#treble.type = 'highshelf';
    this.#treble.frequency.value = 4000;
    this.#headroom = context.createGain();
    this.#compressor = context.createDynamicsCompressor();
    this.#compressor.knee.value = 12;
    this.#compressor.attack.value = 0.006;
    this.#compressor.release.value = 0.15;
    this.#dry = context.createGain();
    this.#wet = context.createGain();
    this.input.connect(gate);
    gate.connect(this.#headroom).connect(this.#bass).connect(this.#mid).connect(this.#treble);
    this.#treble.connect(this.#dry).connect(output);
    this.#treble.connect(this.#compressor).connect(this.#wet).connect(output);
    this.#nodes = [
      this.input,
      this.#headroom,
      this.#bass,
      this.#mid,
      this.#treble,
      this.#compressor,
      this.#dry,
      this.#wet
    ];
    this.update(defaultMicrophoneEffects, true);
  }

  /** Smooth updates avoid clicks; dry bypass avoids compressor latency when off. */
  update(settings: MicrophoneEffects, immediate = false): void {
    const value = normalizeMicrophoneEffects(settings);
    const key = JSON.stringify(value);
    if (key === this.#settings) return;
    this.#settings = key;
    const set = (param: AudioParam, target: number) => {
      param.cancelScheduledValues(this.context.currentTime);
      if (immediate) param.value = target;
      else param.setTargetAtTime(target, this.context.currentTime, 0.015);
    };
    set(this.input.frequency, value.lowCut ? 80 : 0);
    const gains = value.equalizer ? [value.bass, value.mid, value.treble] : [0, 0, 0];
    set(this.#bass.gain, gains[0]);
    set(this.#mid.gain, gains[1]);
    set(this.#treble.gain, gains[2]);
    // Reserve the sum of positive boosts before EQ to avoid output clipping.
    set(this.#headroom.gain, 10 ** (-gains.reduce((sum, gain) => sum + Math.max(0, gain), 0) / 20));
    set(this.#compressor.threshold, -12 - value.amount * 0.24);
    set(this.#compressor.ratio, 2 + value.amount * 0.06);
    set(this.#dry.gain, value.compressor ? 0 : 1);
    set(this.#wet.gain, value.compressor ? 1 : 0);
  }

  destroy(): void {
    this.#nodes.forEach((node) => node.disconnect());
  }
}
