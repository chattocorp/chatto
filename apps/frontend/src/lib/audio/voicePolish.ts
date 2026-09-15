/** Bound the derived Voice Quality amount before it reaches audio-thread state. */
export function normalizePolish(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? Math.max(0, Math.min(1, value)) : 0;
}

/**
 * Automatic high-band de-essing followed by a linked sample-peak limiter.
 * No look-ahead or sample delay. The ceiling applies before encoding, not to
 * reconstructed inter-sample peaks. Instances belong to one audio worklet.
 */
export class VoicePolish {
  #target = 0;
  #amount = 0;
  #low = new Float64Array(0);
  #high = new Float64Array(0);
  #energy = 0;
  #highEnergy = 0;
  #deEssGain = 1;
  #limitGain = 1;
  readonly #filter: number;
  readonly #attack: number;
  readonly #release: number;
  readonly #smooth: number;

  constructor(sampleRate: number, amount = 0) {
    this.#filter = 1 - Math.exp((-2 * Math.PI * 4000) / sampleRate);
    this.#attack = 1 - Math.exp(-1 / (sampleRate * 0.001));
    this.#release = 1 - Math.exp(-1 / (sampleRate * 0.08));
    this.#smooth = 1 - Math.exp(-1 / (sampleRate * 0.015));
    this.#amount = this.#target = normalizePolish(amount);
  }

  /** Smooth live changes; construction applies the saved amount immediately. */
  set amount(value: number) {
    this.#target = normalizePolish(value);
  }

  process(input: Float32Array[], output: Float32Array[]): void {
    const channels = output.length;
    if (this.#low.length !== channels) {
      this.#low = new Float64Array(channels);
      this.#high = new Float64Array(channels);
    }
    for (let i = 0; i < (output[0]?.length ?? 0); i++) {
      this.#amount += (this.#target - this.#amount) * this.#smooth;
      if (Math.abs(this.#amount - this.#target) < 0.00001) this.#amount = this.#target;
      let energy = 0;
      let highEnergy = 0;
      for (let c = 0; c < channels; c++) {
        const sample = input[c]?.[i] ?? 0;
        this.#low[c] += this.#filter * (sample - this.#low[c]);
        this.#high[c] = sample - this.#low[c];
        energy += sample * sample;
        highEnergy += this.#high[c] ** 2;
      }
      energy /= Math.max(1, channels);
      highEnergy /= Math.max(1, channels);
      this.#energy += (energy - this.#energy) * this.#attack;
      this.#highEnergy += (highEnergy - this.#highEnergy) * this.#attack;
      // Require both a high-band-heavy signal and audible high-band energy.
      // Quiet hiss and ordinary low/mid speech must not trigger a blanket EQ cut.
      const prominence = Math.max(
        0,
        Math.min(1, (this.#highEnergy / Math.max(this.#energy, 1e-12) - 0.25) / 0.35)
      );
      const audible = Math.max(0, Math.min(1, (Math.sqrt(this.#highEnergy) - 0.015) / 0.06));
      const target = 10 ** ((-2 * prominence * audible) / 20);
      this.#deEssGain +=
        (target - this.#deEssGain) * (target < this.#deEssGain ? this.#attack : this.#release);
      let peak = 0;
      for (let c = 0; c < channels; c++) {
        // Complementary split sums to the original signal when reduction is off.
        const sample =
          this.#amount === 0
            ? (input[c]?.[i] ?? 0)
            : this.#low[c] + this.#high[c] * (1 + (this.#deEssGain - 1) * this.#amount);
        output[c][i] = sample;
        peak = Math.max(peak, Math.abs(output[c][i]));
      }
      if (this.#amount === 0) {
        this.#limitGain = 1;
        this.#deEssGain = 1;
        continue;
      }
      const ceiling = 0.99;
      const limit = Math.min(1, ceiling / Math.max(peak, 1e-12));
      // Immediate linked attack catches even the first sample of a transient.
      this.#limitGain = Math.min(limit, this.#limitGain + (1 - this.#limitGain) * this.#release);
      // Fade held attenuation with the control, while still enforcing the
      // current sample ceiling for an active limiter.
      const appliedGain = Math.min(limit, 1 + (this.#limitGain - 1) * this.#amount);
      for (const channel of output) channel[i] *= appliedGain;
    }
  }
}
