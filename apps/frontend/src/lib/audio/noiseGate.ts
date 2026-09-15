/** Lowest slider position disables gating. Other values are dBFS thresholds. */
export const GATE_OFF = -60;

/** Normalize browser-local values without enabling gating for invalid storage. */
export function normalizeGateThreshold(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= GATE_OFF && value <= 0
    ? Math.round(value)
    : GATE_OFF;
}

/** Map dBFS onto the shared meter and threshold slider scale. */
export function microphoneMeter(rms: number): number {
  return Math.max(0, Math.min(1, (20 * Math.log10(Math.max(rms, 0.000001)) + 60) / 60));
}

/** Sample-clock gate envelope. It never changes track enabled or mute state. */
export class NoiseGate {
  threshold = GATE_OFF;
  level = 0;
  #gain = 0;
  #open = false;
  #hold = 0;

  constructor(readonly sampleRate: number) {}

  /** Process all channels with one envelope to preserve channel balance. */
  process(input: Float32Array[], output: Float32Array[]): void {
    const frames = output[0]?.length ?? 0;
    let sum = 0;
    for (const channel of input) for (const sample of channel) sum += sample * sample;
    this.level = Math.sqrt(sum / Math.max(1, frames * input.length));
    const disabled = this.threshold <= GATE_OFF;
    const opening = 10 ** (this.threshold / 20);
    const closing = opening * 0.5;
    if (disabled || this.level >= (this.#open ? closing : opening)) {
      this.#open = true;
      this.#hold = this.sampleRate * 0.15;
    } else {
      this.#hold = Math.max(0, this.#hold - frames);
      if (this.#hold === 0) this.#open = false;
    }
    const target = this.#open ? 1 : 0;
    const step = 1 / (this.sampleRate * (target ? 0.005 : 0.08));
    for (let i = 0; i < frames; i++) {
      this.#gain = disabled
        ? 1
        : target
          ? Math.min(1, this.#gain + step)
          : Math.max(0, this.#gain - step);
      for (let c = 0; c < output.length; c++) output[c][i] = (input[c]?.[i] ?? 0) * this.#gain;
    }
  }
}
