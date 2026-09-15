/** Browser-local microphone effects. Gains are dB; compressor amount is 0–100. */
export interface MicrophoneEffects {
  lowCut: boolean;
  equalizer: boolean;
  bass: number;
  mid: number;
  treble: number;
  compressor: boolean;
  amount: number;
}

/** Bound inputs before they reach AudioParams; omitted values produce fresh defaults. */
export function normalizeMicrophoneEffects(value?: unknown): MicrophoneEffects {
  const raw = value && typeof value === 'object' ? (value as Partial<MicrophoneEffects>) : {};
  const bounded = (value: unknown, min: number, max: number, fallback: number) =>
    typeof value === 'number' && Number.isFinite(value)
      ? Math.round(Math.max(min, Math.min(max, value)))
      : fallback;
  return {
    lowCut: raw.lowCut === true,
    equalizer: raw.equalizer === true,
    bass: bounded(raw.bass, -6, 6, 0),
    mid: bounded(raw.mid, -6, 6, 0),
    treble: bounded(raw.treble, -6, 6, 0),
    compressor: raw.compressor === true,
    amount: bounded(raw.amount, 0, 100, 50)
  };
}
