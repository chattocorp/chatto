import { normalizePolish } from './voicePolish';

const clamp = (value: number) => Math.max(0, Math.min(1, value));

/**
 * Linked, automatic low-frequency cuts before EQ and compression.
 * A fast 120 Hz burst detector limits plosives; a slower 250 Hz detector
 * controls sustained boom. Neither adds look-ahead or delays the dry signal.
 * This heuristic cannot distinguish every low-pitched vowel from a plosive,
 * so cuts are bounded and require both bass dominance and audible energy.
 */
export class LowFrequencyControl {
  #amount: number;
  #target: number;
  #sub = new Float64Array(0);
  #bass = new Float64Array(0);
  #fastEnergy = 0;
  #fastSub = 0;
  #slowSub = 0;
  #slowEnergy = 0;
  #slowBass = 0;
  #plosiveGain = 1;
  #bassGain = 1;
  readonly #subFilter: number;
  readonly #bassFilter: number;
  readonly #fast: number;
  readonly #slow: number;
  readonly #attack: number;
  readonly #release: number;
  readonly #bassRelease: number;
  readonly #smooth: number;

  constructor(sampleRate: number, amount = 0) {
    const coefficient = (seconds: number) => 1 - Math.exp(-1 / (sampleRate * seconds));
    this.#subFilter = 1 - Math.exp((-2 * Math.PI * 120) / sampleRate);
    this.#bassFilter = 1 - Math.exp((-2 * Math.PI * 250) / sampleRate);
    this.#fast = coefficient(0.002);
    this.#slow = coefficient(0.15);
    this.#attack = coefficient(0.001);
    this.#release = coefficient(0.08);
    this.#bassRelease = coefficient(0.5);
    this.#smooth = coefficient(0.015);
    this.#amount = this.#target = normalizePolish(amount);
  }

  /** Match the processing amount, with a 15 ms transition. */
  set amount(value: number) {
    this.#target = normalizePolish(value);
  }

  /** Input and output may alias; each sample is read before it is replaced. */
  process(input: Float32Array[], output: Float32Array[]): void {
    const channels = output.length;
    if (this.#sub.length !== channels) {
      this.#sub = new Float64Array(channels);
      this.#bass = new Float64Array(channels);
    }
    for (let i = 0; i < (output[0]?.length ?? 0); i++) {
      this.#amount += (this.#target - this.#amount) * this.#smooth;
      if (Math.abs(this.#target - this.#amount) < 0.00001) this.#amount = this.#target;
      let energy = 0;
      let subEnergy = 0;
      for (let c = 0; c < channels; c++) {
        const sample = input[c]?.[i] ?? 0;
        this.#sub[c] += this.#subFilter * (sample - this.#sub[c]);
        energy += sample * sample;
        subEnergy += this.#sub[c] ** 2;
      }
      energy /= Math.max(1, channels);
      subEnergy /= Math.max(1, channels);
      this.#fastEnergy += (energy - this.#fastEnergy) * this.#fast;
      this.#fastSub += (subEnergy - this.#fastSub) * this.#fast;
      this.#slowSub += (subEnergy - this.#slowSub) * this.#slow;
      const burst = clamp((this.#fastSub / Math.max(this.#slowSub, 1e-9) - 2) / 4);
      const lowDominance = clamp((this.#fastSub / Math.max(this.#fastEnergy, 1e-9) - 0.55) / 0.3);
      const audible = clamp((Math.sqrt(this.#fastSub) - 0.035) / 0.1);
      const plosiveTarget = 10 ** ((-3 * burst * lowDominance * audible) / 20);
      this.#plosiveGain +=
        (plosiveTarget - this.#plosiveGain) *
        (plosiveTarget < this.#plosiveGain ? this.#attack : this.#release);

      // The slow detector sees the corrected signal, so brief plosives do not
      // ask the bass controller to reduce otherwise healthy speech afterwards.
      let correctedEnergy = 0;
      let bassEnergy = 0;
      for (let c = 0; c < channels; c++) {
        const sample = (input[c]?.[i] ?? 0) + this.#sub[c] * (this.#plosiveGain - 1) * this.#amount;
        this.#bass[c] += this.#bassFilter * (sample - this.#bass[c]);
        correctedEnergy += sample * sample;
        bassEnergy += this.#bass[c] ** 2;
      }
      this.#slowEnergy += (correctedEnergy / Math.max(1, channels) - this.#slowEnergy) * this.#slow;
      this.#slowBass += (bassEnergy / Math.max(1, channels) - this.#slowBass) * this.#slow;
      const boom = clamp((this.#slowBass / Math.max(this.#slowEnergy, 1e-9) - 0.65) / 0.25);
      const bassAudible = clamp((Math.sqrt(this.#slowBass) - 0.05) / 0.15);
      const bassTarget = 10 ** ((-1.5 * boom * bassAudible) / 20);
      this.#bassGain +=
        (bassTarget - this.#bassGain) *
        (bassTarget < this.#bassGain ? this.#slow : this.#bassRelease);
      // Blend correction with the smoothed amount directly: slow release must
      // not leave a held cut that jumps away when Normal reaches exact bypass.
      for (let c = 0; c < channels; c++) {
        const sample = input[c]?.[i] ?? 0;
        output[c][i] =
          this.#amount === 0
            ? sample
            : sample +
              this.#sub[c] * (this.#plosiveGain - 1) * this.#amount +
              this.#bass[c] * (this.#bassGain - 1) * this.#amount;
      }
      if (this.#amount === 0) this.#plosiveGain = this.#bassGain = 1;
    }
  }
}
