/** Browser-local microphone effects. Gains are dB; compressor amount is 0–100. */
export interface MicrophoneEffects {
  lowCut: boolean;
  equalizer: boolean;
  bass: number;
  mid: number;
  treble: number;
  compressor: boolean;
  amount: number;
  /** Ramp filter cutoff and compression from neutral; omitted means full strength. */
  strength?: number;
}

/** Bound inputs before they reach AudioParams; omitted values produce fresh defaults. */
export function normalizeMicrophoneEffects(value?: unknown): MicrophoneEffects {
  const raw = value && typeof value === 'object' ? (value as Partial<MicrophoneEffects>) : {};
  const bounded = (value: unknown, min: number, max: number, fallback: number) =>
    typeof value === 'number' && Number.isFinite(value)
      ? Math.max(min, Math.min(max, value))
      : fallback;
  return {
    lowCut: raw.lowCut === true,
    equalizer: raw.equalizer === true,
    bass: bounded(raw.bass, -6, 6, 0),
    mid: bounded(raw.mid, -6, 6, 0),
    treble: bounded(raw.treble, -6, 6, 0),
    compressor: raw.compressor === true,
    amount: bounded(raw.amount, 0, 100, 50),
    strength: bounded(raw.strength, 0, 1, 1)
  };
}

/** Clamp the browser-local voice control; invalid storage restores Normal. */
export function normalizeVoiceAmount(value: unknown): number {
  return Number.isFinite(value) ? Math.max(0, Math.min(100, value as number)) : 0;
}

/** Interpolate normalized 0–100 input across Normal → Pretty cool → AWESOME without rounding. */
export function microphoneEffectsForAmount(amount: number): MicrophoneEffects {
  const lower = Math.min(amount / 50, 1);
  const upper = Math.max(amount / 50 - 1, 0);
  return {
    lowCut: amount > 0,
    equalizer: amount > 0,
    bass: 0 - lower - 2 * upper,
    mid: lower + upper,
    treble: lower + 2 * upper,
    compressor: amount > 0,
    amount: 15 + 40 * upper,
    strength: lower
  };
}
